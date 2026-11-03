package payments

import (
	"context"
	"crypto/md5" // #nosec G501 -- алгоритм подписи задан протоколом Click SHOP API
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
)

// Коды ошибок Click SHOP API.
const (
	clickOK             = 0
	clickSignFailed     = -1
	clickBadAmount      = -2
	clickActionNotFound = -3
	clickAlreadyPaid    = -4
	clickOrderNotFound  = -5
	clickTxnNotFound    = -6
	clickUpdateFailed   = -7
	clickBadRequest     = -8
	clickTxnCancelled   = -9
	clickActionPrepare  = "0"
	clickActionComplete = "1"
	clickMaxBody        = 16 << 10
)

var clickNotes = map[int]string{
	clickOK:             "Success",
	clickSignFailed:     "SIGN CHECK FAILED!",
	clickBadAmount:      "Incorrect parameter amount",
	clickActionNotFound: "Action not found",
	clickAlreadyPaid:    "Already paid",
	clickOrderNotFound:  "User does not exist",
	clickTxnNotFound:    "Transaction does not exist",
	clickUpdateFailed:   "Failed to update user",
	clickBadRequest:     "Error in request from click",
	clickTxnCancelled:   "Transaction cancelled",
}

// Click — обработчик Prepare/Complete (SHOP API). Подпись:
// prepare:  md5(click_trans_id + service_id + SECRET + merchant_trans_id + amount + action + sign_time)
// complete: md5(click_trans_id + service_id + SECRET + merchant_trans_id + merchant_prepare_id + amount + action + sign_time)
type Click struct {
	DB        *pgxpool.Pool
	ServiceID string
	SecretKey string
	Allowed   ipAllow
	Now       func() time.Time
}

type clickResp struct {
	ClickTransID      int64  `json:"click_trans_id"`
	MerchantTransID   string `json:"merchant_trans_id"`
	MerchantPrepareID *int64 `json:"merchant_prepare_id,omitempty"`
	MerchantConfirmID *int64 `json:"merchant_confirm_id,omitempty"`
	Error             int    `json:"error"`
	ErrorNote         string `json:"error_note"`
}

func (c *Click) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Click) Prepare(w http.ResponseWriter, r *http.Request)  { c.handle(w, r, false) }
func (c *Click) Complete(w http.ResponseWriter, r *http.Request) { c.handle(w, r, true) }

func clickSign(parts ...string) string {
	// Алгоритм подписи задан Click SHOP API и не выбирается нами; секрет участвует в хэше.
	// nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-md5
	sum := md5.Sum([]byte(strings.Join(parts, ""))) // #nosec G401 -- протокол Click
	return hex.EncodeToString(sum[:])
}

func (c *Click) handle(w http.ResponseWriter, r *http.Request, complete bool) {
	r.Body = http.MaxBytesReader(w, r.Body, clickMaxBody)
	if err := r.ParseForm(); err != nil {
		writeClick(w, clickResp{Error: clickBadRequest})
		return
	}
	f := r.PostForm
	resp := clickResp{MerchantTransID: f.Get("merchant_trans_id")}
	resp.ClickTransID, _ = strconv.ParseInt(f.Get("click_trans_id"), 10, 64)
	log := httpx.LoggerFrom(r.Context()).With(slog.String("provider", "click"), slog.Bool("complete", complete),
		slog.Int64("click_trans_id", resp.ClickTransID))

	if c.SecretKey == "" || c.ServiceID == "" || !c.Allowed.allows(httpx.ClientIP(r.Context())) {
		log.Warn("click webhook rejected: provider disabled or ip not allowed", slog.String("ip", httpx.ClientIP(r.Context())))
		resp.Error = clickSignFailed
		writeClick(w, resp)
		return
	}
	parts := []string{f.Get("click_trans_id"), f.Get("service_id"), c.SecretKey, f.Get("merchant_trans_id")}
	if complete {
		parts = append(parts, f.Get("merchant_prepare_id"))
	}
	parts = append(parts, f.Get("amount"), f.Get("action"), f.Get("sign_time"))
	want := clickSign(parts...)
	got := strings.ToLower(f.Get("sign_string"))
	if subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		log.Warn("click sign check failed")
		resp.Error = clickSignFailed
		writeClick(w, resp)
		return
	}
	if subtle.ConstantTimeCompare([]byte(f.Get("service_id")), []byte(c.ServiceID)) != 1 || resp.ClickTransID <= 0 {
		resp.Error = clickBadRequest
		writeClick(w, resp)
		return
	}
	wantAction := clickActionPrepare
	if complete {
		wantAction = clickActionComplete
	}
	if f.Get("action") != wantAction {
		resp.Error = clickActionNotFound
		writeClick(w, resp)
		return
	}
	amount, ok := parseSums(f.Get("amount"))
	if !ok {
		resp.Error = clickBadAmount
		writeClick(w, resp)
		return
	}
	pid, err := uuid.Parse(f.Get("merchant_trans_id"))
	if err != nil {
		resp.Error = clickOrderNotFound
		writeClick(w, resp)
		return
	}
	var code int
	err = db.Tx(r.Context(), c.DB, func(tx pgx.Tx) error {
		var err error
		if complete {
			prepID, perr := strconv.ParseInt(f.Get("merchant_prepare_id"), 10, 64)
			if perr != nil {
				code = clickTxnNotFound
				return nil
			}
			clickErr, perr := strconv.Atoi(f.Get("error"))
			if perr != nil {
				clickErr = -1 // нечитаемое поле error считаем отказом списания, а не успехом
			}
			code, err = c.complete(r.Context(), tx, pid, prepID, f.Get("click_trans_id"), amount, clickErr, &resp)
		} else {
			code, err = c.prepare(r.Context(), tx, pid, f.Get("click_trans_id"), amount, &resp)
		}
		return err
	})
	if err != nil {
		log.Error("click webhook failed", slog.String("err", err.Error()))
		resp.Error = clickUpdateFailed
		resp.MerchantPrepareID, resp.MerchantConfirmID = nil, nil
		writeClick(w, resp)
		return
	}
	resp.Error = code
	log.Info("click webhook", slog.Int("error", code), slog.String("payment_id", pid.String()))
	writeClick(w, resp)
}

func (c *Click) prepare(ctx context.Context, tx pgx.Tx, pid uuid.UUID, clickTxn string, amount int64, resp *clickResp) (int, error) {
	p, err := lockPayment(ctx, tx, pid)
	if errors.Is(err, db.ErrNotFound) || (err == nil && p.Provider != ProviderClick) {
		return clickOrderNotFound, nil
	}
	if err != nil {
		return 0, err
	}
	t, err := scanTxn(tx.QueryRow(ctx, qClickTxnByExtLock, clickTxn))
	switch {
	case err == nil && t.PaymentID != p.ID:
		return clickTxnNotFound, nil
	case err == nil: // повтор Prepare
		resp.MerchantPrepareID = &t.ID
		switch t.State {
		case TxnCreated:
			return clickOK, nil
		case TxnPerformed:
			return clickAlreadyPaid, nil
		default:
			return clickTxnCancelled, nil
		}
	case !errors.Is(err, db.ErrNotFound):
		return 0, err
	}
	switch p.Status {
	case StatusPaid:
		return clickAlreadyPaid, nil
	case StatusPending:
	default:
		return clickTxnCancelled, nil
	}
	if amount != p.Amount {
		return clickBadAmount, nil
	}
	// Незавершённая подготовка с другим click_trans_id (клиент начал оплату заново) — отменяем её.
	if _, err := tx.Exec(ctx, `UPDATE payment_transactions SET state = -1, cancelled_at = now(), cancel_reason = 1
		WHERE payment_id = $1 AND state = 1`, p.ID); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO payment_transactions (payment_id, provider, external_id, state, amount)
		VALUES ($1, 'click', $2, 1, $3) RETURNING id`, p.ID, clickTxn, amount).Scan(&id); err != nil {
		return 0, err
	}
	resp.MerchantPrepareID = &id
	return clickOK, nil
}

func (c *Click) complete(ctx context.Context, tx pgx.Tx, pid uuid.UUID, prepID int64, clickTxn string, amount int64, clickErr int, resp *clickResp) (int, error) {
	p, err := lockPayment(ctx, tx, pid)
	if errors.Is(err, db.ErrNotFound) || (err == nil && p.Provider != ProviderClick) {
		return clickOrderNotFound, nil
	}
	if err != nil {
		return 0, err
	}
	t, err := scanTxn(tx.QueryRow(ctx, qClickTxnByIDLock, prepID))
	if errors.Is(err, db.ErrNotFound) || (err == nil && (t.PaymentID != p.ID || t.ExternalID != clickTxn)) {
		return clickTxnNotFound, nil
	}
	if err != nil {
		return 0, err
	}
	resp.MerchantConfirmID = &t.ID
	switch t.State {
	case TxnPerformed: // повтор Complete после успеха — тот же ответ, без повторного зачисления
		return clickOK, nil
	case TxnCancelled, TxnCancelledRefund:
		return clickTxnCancelled, nil
	}
	if amount != t.Amount || amount != p.Amount {
		return clickBadAmount, nil
	}
	if p.Status == StatusPaid {
		return clickAlreadyPaid, nil
	}
	if p.Status != StatusPending {
		return clickTxnCancelled, nil
	}
	if clickErr < 0 { // Click сообщает о неудаче списания — отменяем
		if _, err := tx.Exec(ctx, `UPDATE payment_transactions SET state = -1, cancelled_at = now(), cancel_reason = $2
			WHERE id = $1`, t.ID, clickErr); err != nil {
			return 0, err
		}
		if err := markFailed(ctx, tx, p, StatusDeclined); err != nil {
			return 0, err
		}
		return clickTxnCancelled, nil
	}
	now := c.now()
	if _, err := tx.Exec(ctx, `UPDATE payment_transactions SET state = 2, performed_at = $2 WHERE id = $1`, t.ID, now); err != nil {
		return 0, err
	}
	if err := markPaid(ctx, tx, p, clickTxn, now); err != nil {
		return 0, err
	}
	return clickOK, nil
}

func writeClick(w http.ResponseWriter, resp clickResp) {
	if resp.ErrorNote == "" {
		resp.ErrorNote = clickNotes[resp.Error]
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// parseSums переводит сумму Click в сумах («15000», «15000.0», «15000.00») в тийины.
func parseSums(s string) (int64, bool) {
	whole, frac, hasDot := strings.Cut(strings.TrimSpace(s), ".")
	if whole == "" || len(whole) > 15 || !digits(whole) {
		return 0, false
	}
	if hasDot {
		frac = strings.TrimRight(frac, "0")
		if len(frac) > 2 || !digits(frac) {
			return 0, false
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	n, err := strconv.ParseInt(whole+frac, 10, 64)
	return n, err == nil && n > 0
}

func digits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// formatSums — тийины в строку для ссылки Click (сумы с двумя знаками).
func formatSums(tiyin int64) string {
	return strconv.FormatInt(tiyin/100, 10) + "." + leftPad2(tiyin%100)
}

func leftPad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// SQL-запросы собраны из констант на этапе компиляции (пользовательский ввод — только плейсхолдеры).
const (
	qClickTxnByExtLock = `SELECT ` + txnCols + ` FROM payment_transactions
		WHERE provider = 'click' AND external_id = $1 FOR UPDATE`
	qClickTxnByIDLock = `SELECT ` + txnCols + ` FROM payment_transactions
		WHERE id = $1 AND provider = 'click' FOR UPDATE`
)
