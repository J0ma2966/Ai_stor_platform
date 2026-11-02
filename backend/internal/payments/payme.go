package payments

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
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

// Payme Merchant API (JSON-RPC 2.0). Вызывает нас сам Payme, авторизация —
// Basic "Paycom:<KEY>". Ответ всегда HTTP 200 с result или error.
const (
	paymeTimeout = 12 * time.Hour // транзакция в состоянии 1 дольше — отменяется (reason 4)
	paymeMaxBody = 64 << 10

	rpcParse        = -32700
	rpcInvalid      = -32600
	rpcMethod       = -32601
	rpcSystem       = -32400
	rpcAuth         = -32504
	paymeBadAmount  = -31001
	paymeTxnMissing = -31003
	paymeCantCancel = -31007
	paymeCantDo     = -31008
	paymeNoOrder    = -31050
	paymeOrderDone  = -31051
	paymeOrderBusy  = -31099

	reasonTimeout = 4
)

var paymeMessages = map[int][3]string{ // ru, uz, en
	rpcParse:        {"Ошибка разбора запроса", "So'rovni o'qishda xato", "Parse error"},
	rpcInvalid:      {"Неверный запрос", "Noto'g'ri so'rov", "Invalid request"},
	rpcMethod:       {"Метод не найден", "Metod topilmadi", "Method not found"},
	rpcSystem:       {"Системная ошибка", "Tizim xatosi", "System error"},
	rpcAuth:         {"Недостаточно привилегий", "Huquqlar yetarli emas", "Insufficient privileges"},
	paymeBadAmount:  {"Неверная сумма", "Noto'g'ri summa", "Invalid amount"},
	paymeTxnMissing: {"Транзакция не найдена", "Tranzaksiya topilmadi", "Transaction not found"},
	paymeCantCancel: {"Токены уже зачислены, отмена невозможна", "Tokenlar hisobga o'tkazilgan, bekor qilib bo'lmaydi", "Order completed, cannot cancel"},
	paymeCantDo:     {"Невозможно выполнить операцию", "Amalni bajarib bo'lmaydi", "Unable to perform operation"},
	paymeNoOrder:    {"Платёж не найден", "To'lov topilmadi", "Payment not found"},
	paymeOrderDone:  {"Платёж уже оплачен или отменён", "To'lov allaqachon to'langan yoki bekor qilingan", "Payment already paid or cancelled"},
	paymeOrderBusy:  {"Платёж уже ожидает оплаты в другой транзакции", "To'lov boshqa tranzaksiyada kutilmoqda", "Payment is busy with another transaction"},
}

type rpcError struct {
	Code    int               `json:"code"`
	Message map[string]string `json:"message"`
	Data    any               `json:"data,omitempty"`
}

func rpcErr(code int, data any) *rpcError {
	m := paymeMessages[code]
	return &rpcError{Code: code, Message: map[string]string{"ru": m[0], "uz": m[1], "en": m[2]}, Data: data}
}

type Payme struct {
	DB         *pgxpool.Pool
	Key        string // ключ кассы; пустой — все запросы отклоняются
	AccountKey string // поле account (по умолчанию payment_id)
	Allowed    ipAllow
	Now        func() time.Time
	// Фискализация: если IKPU задан, CheckPerformTransaction возвращает detail для чека.
	IKPU        string
	PackageCode string
	VATPercent  int
}

func (p *Payme) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func ms(t time.Time) int64 { return t.UnixMilli() }

func msPtr(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.UnixMilli()
}

type rpcRequest struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type paymeParams struct {
	ID      string                     `json:"id"`
	Time    int64                      `json:"time"`
	Amount  int64                      `json:"amount"`
	Account map[string]json.RawMessage `json:"account"`
	Reason  *int                       `json:"reason"`
	From    int64                      `json:"from"`
	To      int64                      `json:"to"`
}

func (p *Payme) authorized(r *http.Request) bool {
	if p.Key == "" || !p.Allowed.allows(httpx.ClientIP(r.Context())) {
		return false
	}
	h, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Basic ")
	if !ok {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h))
	if err != nil {
		return false
	}
	login, key, _ := strings.Cut(string(raw), ":")
	okLogin := subtle.ConstantTimeCompare([]byte(login), []byte("Paycom"))
	okKey := subtle.ConstantTimeCompare([]byte(key), []byte(p.Key))
	return okLogin&okKey == 1
}

func (p *Payme) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	log := httpx.LoggerFrom(r.Context()).With(slog.String("provider", "payme"))
	r.Body = http.MaxBytesReader(w, r.Body, paymeMaxBody)
	decodeErr := json.NewDecoder(r.Body).Decode(&req)
	if !p.authorized(r) { // id возвращаем и в ошибке авторизации — так требует Payme
		log.Warn("payme auth failed", slog.String("ip", httpx.ClientIP(r.Context())))
		writeRPC(w, req.ID, nil, rpcErr(rpcAuth, nil))
		return
	}
	if decodeErr != nil {
		writeRPC(w, nil, nil, rpcErr(rpcParse, nil))
		return
	}
	var prm paymeParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &prm); err != nil {
			writeRPC(w, req.ID, nil, rpcErr(rpcInvalid, nil))
			return
		}
	}
	log = log.With(slog.String("method", req.Method), slog.String("payme_id", prm.ID))

	var (
		result any
		rerr   *rpcError
	)
	run := func(fn func(ctx context.Context, tx pgx.Tx) (any, *rpcError, error)) {
		// rpc-ошибка не откатывает транзакцию: отмена по таймауту должна сохраниться
		err := db.Tx(r.Context(), p.DB, func(tx pgx.Tx) error {
			var err error
			result, rerr, err = fn(r.Context(), tx)
			return err
		})
		if err != nil {
			log.Error("payme failed", slog.String("err", err.Error()))
			result, rerr = nil, rpcErr(rpcSystem, nil)
		}
	}
	needID := func() bool {
		if prm.ID == "" || len(prm.ID) > 64 {
			rerr = rpcErr(rpcInvalid, "id")
			return false
		}
		return true
	}
	switch req.Method {
	case "CheckPerformTransaction":
		run(func(ctx context.Context, tx pgx.Tx) (any, *rpcError, error) { return p.checkPerform(ctx, tx, &prm) })
	case "CreateTransaction":
		if needID() {
			run(func(ctx context.Context, tx pgx.Tx) (any, *rpcError, error) { return p.create(ctx, tx, &prm) })
		}
	case "PerformTransaction":
		if needID() {
			run(func(ctx context.Context, tx pgx.Tx) (any, *rpcError, error) { return p.perform(ctx, tx, prm.ID) })
		}
	case "CancelTransaction":
		if needID() {
			run(func(ctx context.Context, tx pgx.Tx) (any, *rpcError, error) {
				return p.cancel(ctx, tx, prm.ID, prm.Reason)
			})
		}
	case "CheckTransaction":
		if needID() {
			run(func(ctx context.Context, tx pgx.Tx) (any, *rpcError, error) { return p.check(ctx, tx, prm.ID) })
		}
	case "GetStatement":
		run(func(ctx context.Context, tx pgx.Tx) (any, *rpcError, error) {
			return p.statement(ctx, tx, prm.From, prm.To)
		})
	default:
		rerr = rpcErr(rpcMethod, req.Method)
	}
	if rerr != nil {
		log.Info("payme error", slog.Int("code", rerr.Code))
	} else {
		log.Info("payme ok")
	}
	writeRPC(w, req.ID, result, rerr)
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, e *rpcError) {
	body := map[string]any{"jsonrpc": "2.0", "id": nil}
	if len(id) > 0 {
		body["id"] = id
	}
	if e != nil {
		body["error"] = e
	} else {
		body["result"] = result
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

// accountPayment достаёт id платежа из account (строка или число в JSON).
func (p *Payme) accountPayment(acc map[string]json.RawMessage) (uuid.UUID, bool) {
	raw, ok := acc[p.AccountKey]
	if !ok {
		return uuid.Nil, false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	id, err := uuid.Parse(strings.TrimSpace(s))
	return id, err == nil
}

// orderFor находит и блокирует платёж из account и проверяет, что его можно оплатить суммой amount.
func (p *Payme) orderFor(ctx context.Context, tx pgx.Tx, prm *paymeParams) (*payment, *rpcError, error) {
	id, ok := p.accountPayment(prm.Account)
	if !ok {
		return nil, rpcErr(paymeNoOrder, p.AccountKey), nil
	}
	pay, err := lockPayment(ctx, tx, id)
	if errors.Is(err, db.ErrNotFound) || (err == nil && pay.Provider != ProviderPayme) {
		return nil, rpcErr(paymeNoOrder, p.AccountKey), nil
	}
	if err != nil {
		return nil, nil, err
	}
	if pay.Status != StatusPending {
		return nil, rpcErr(paymeOrderDone, p.AccountKey), nil
	}
	if prm.Amount != pay.Amount {
		return nil, rpcErr(paymeBadAmount, "amount"), nil
	}
	return pay, nil, nil
}

func (p *Payme) checkPerform(ctx context.Context, tx pgx.Tx, prm *paymeParams) (any, *rpcError, error) {
	pay, rerr, err := p.orderFor(ctx, tx, prm)
	if pay == nil {
		return nil, rerr, err
	}
	res := map[string]any{"allow": true}
	if p.IKPU != "" {
		var title string
		if err := tx.QueryRow(ctx, `SELECT t.name_ru FROM payments p JOIN tariffs t ON t.id = p.tariff_id WHERE p.id = $1`,
			pay.ID).Scan(&title); err != nil {
			return nil, nil, err
		}
		item := map[string]any{
			"title": title + " (" + strconv.FormatInt(pay.Tokens, 10) + " AI-токенов)", "price": pay.Amount, "count": 1,
			"code": p.IKPU, "vat_percent": p.VATPercent,
		}
		if p.PackageCode != "" {
			item["package_code"] = p.PackageCode
		}
		res["detail"] = map[string]any{"receipt_type": 0, "items": []any{item}}
	}
	return res, nil, nil
}

// lockTxn: блокирует платёж транзакции, затем саму транзакцию (порядок блокировок общий для всех вебхуков).
func lockTxn(ctx context.Context, tx pgx.Tx, externalID string) (*payment, *txn, error) {
	var pid uuid.UUID
	err := tx.QueryRow(ctx, `SELECT payment_id FROM payment_transactions WHERE provider = 'payme' AND external_id = $1`,
		externalID).Scan(&pid)
	if err != nil {
		return nil, nil, db.NotFound(err)
	}
	pay, err := lockPayment(ctx, tx, pid)
	if err != nil {
		return nil, nil, err
	}
	t, err := scanTxn(tx.QueryRow(ctx, `SELECT `+txnCols+` FROM payment_transactions
		WHERE provider = 'payme' AND external_id = $1 FOR UPDATE`, externalID))
	return pay, t, err
}

func (p *Payme) expired(t *txn) bool {
	start := t.CreatedAt
	if t.ProviderTime > 0 {
		start = time.UnixMilli(t.ProviderTime)
	}
	return p.now().Sub(start) > paymeTimeout
}

func cancelTxn(ctx context.Context, tx pgx.Tx, pay *payment, t *txn, state, reason int, at time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE payment_transactions SET state = $2, cancelled_at = $3, cancel_reason = $4 WHERE id = $1`,
		t.ID, state, at, reason); err != nil {
		return err
	}
	t.State, t.CancelledAt, t.CancelReason = state, &at, &reason
	return markFailed(ctx, tx, pay, StatusCancelled)
}

func (p *Payme) create(ctx context.Context, tx pgx.Tx, prm *paymeParams) (any, *rpcError, error) {
	pay, t, err := lockTxn(ctx, tx, prm.ID)
	if errors.Is(err, db.ErrNotFound) {
		// новая транзакция: блокируем платёж из account и проверяем снова (параллельный дубль)
		var rerr *rpcError
		if pay, rerr, err = p.orderFor(ctx, tx, prm); pay == nil {
			return nil, rerr, err
		}
		t, err = scanTxn(tx.QueryRow(ctx, `SELECT `+txnCols+` FROM payment_transactions
			WHERE provider = 'payme' AND external_id = $1 FOR UPDATE`, prm.ID))
		if errors.Is(err, db.ErrNotFound) {
			var busy bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM payment_transactions WHERE payment_id = $1 AND state IN (1, 2))`,
				pay.ID).Scan(&busy); err != nil {
				return nil, nil, err
			}
			if busy {
				return nil, rpcErr(paymeOrderBusy, p.AccountKey), nil
			}
			t, err = scanTxn(tx.QueryRow(ctx, `INSERT INTO payment_transactions (payment_id, provider, external_id, state, amount, provider_time, created_at)
				VALUES ($1, 'payme', $2, 1, $3, $4, $5) RETURNING `+txnCols, pay.ID, prm.ID, prm.Amount, prm.Time, p.now()))
			if err != nil {
				return nil, nil, err
			}
			return map[string]any{"create_time": ms(t.CreatedAt), "transaction": strconv.FormatInt(t.ID, 10), "state": t.State}, nil, nil
		}
	}
	if err != nil {
		return nil, nil, err
	}
	// повтор CreateTransaction с тем же id
	if t.State != TxnCreated {
		return nil, rpcErr(paymeCantDo, nil), nil
	}
	if p.expired(t) {
		if err := cancelTxn(ctx, tx, pay, t, TxnCancelled, reasonTimeout, p.now()); err != nil {
			return nil, nil, err
		}
		return nil, rpcErr(paymeCantDo, nil), nil
	}
	return map[string]any{"create_time": ms(t.CreatedAt), "transaction": strconv.FormatInt(t.ID, 10), "state": t.State}, nil, nil
}

func (p *Payme) perform(ctx context.Context, tx pgx.Tx, id string) (any, *rpcError, error) {
	pay, t, err := lockTxn(ctx, tx, id)
	if errors.Is(err, db.ErrNotFound) {
		return nil, rpcErr(paymeTxnMissing, nil), nil
	}
	if err != nil {
		return nil, nil, err
	}
	switch t.State {
	case TxnPerformed:
		// повтор: тот же ответ, токены уже зачислены
	case TxnCreated:
		if p.expired(t) {
			if err := cancelTxn(ctx, tx, pay, t, TxnCancelled, reasonTimeout, p.now()); err != nil {
				return nil, nil, err
			}
			return nil, rpcErr(paymeCantDo, nil), nil
		}
		if pay.Status != StatusPending || t.Amount != pay.Amount {
			return nil, rpcErr(paymeCantDo, nil), nil
		}
		now := p.now()
		if _, err := tx.Exec(ctx, `UPDATE payment_transactions SET state = 2, performed_at = $2 WHERE id = $1`, t.ID, now); err != nil {
			return nil, nil, err
		}
		if err := markPaid(ctx, tx, pay, id, now); err != nil {
			return nil, nil, err
		}
		t.State, t.PerformedAt = TxnPerformed, &now
	default:
		return nil, rpcErr(paymeCantDo, nil), nil
	}
	return map[string]any{"transaction": strconv.FormatInt(t.ID, 10), "perform_time": msPtr(t.PerformedAt), "state": t.State}, nil, nil
}

func (p *Payme) cancel(ctx context.Context, tx pgx.Tx, id string, reason *int) (any, *rpcError, error) {
	pay, t, err := lockTxn(ctx, tx, id)
	if errors.Is(err, db.ErrNotFound) {
		return nil, rpcErr(paymeTxnMissing, nil), nil
	}
	if err != nil {
		return nil, nil, err
	}
	switch t.State {
	case TxnCreated:
		rs := 0
		if reason != nil {
			rs = *reason
		}
		if err := cancelTxn(ctx, tx, pay, t, TxnCancelled, rs, p.now()); err != nil {
			return nil, nil, err
		}
	case TxnPerformed:
		// Токены зачислены сразу при проведении и могли быть потрачены — возврат
		// через Payme не поддерживаем, только через поддержку (admin_adjustment).
		return nil, rpcErr(paymeCantCancel, nil), nil
	}
	return map[string]any{"transaction": strconv.FormatInt(t.ID, 10), "cancel_time": msPtr(t.CancelledAt), "state": t.State}, nil, nil
}

func txnView(t *txn) map[string]any {
	var reason any
	if t.CancelReason != nil {
		reason = *t.CancelReason
	}
	return map[string]any{
		"create_time": ms(t.CreatedAt), "perform_time": msPtr(t.PerformedAt), "cancel_time": msPtr(t.CancelledAt),
		"transaction": strconv.FormatInt(t.ID, 10), "state": t.State, "reason": reason,
	}
}

func (p *Payme) check(ctx context.Context, tx pgx.Tx, id string) (any, *rpcError, error) {
	t, err := scanTxn(tx.QueryRow(ctx, `SELECT `+txnCols+` FROM payment_transactions WHERE provider = 'payme' AND external_id = $1`, id))
	if errors.Is(err, db.ErrNotFound) {
		return nil, rpcErr(paymeTxnMissing, nil), nil
	}
	if err != nil {
		return nil, nil, err
	}
	return txnView(t), nil, nil
}

func (p *Payme) statement(ctx context.Context, tx pgx.Tx, from, to int64) (any, *rpcError, error) {
	if from <= 0 || to < from {
		return nil, rpcErr(rpcInvalid, "from/to"), nil
	}
	rows, err := tx.Query(ctx, `SELECT `+txnCols+` FROM payment_transactions
		WHERE provider = 'payme' AND provider_time BETWEEN $1 AND $2 ORDER BY provider_time LIMIT 10000`, from, to)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		t, err := scanTxn(rows)
		if err != nil {
			return nil, nil, err
		}
		v := txnView(t)
		v["id"], v["time"], v["amount"] = t.ExternalID, t.ProviderTime, t.Amount
		v["account"] = map[string]string{p.AccountKey: t.PaymentID.String()}
		v["receivers"] = nil
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return map[string]any{"transactions": out}, nil, nil
}
