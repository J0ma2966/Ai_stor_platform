// Package promotions — реклама товаров магазинов (docs/PROMOTIONS.md).
//
// Правила:
//   - магазин подаёт заявку на продвижение своего опубликованного товара и просит срок (дней);
//   - суперадмин одобряет заявку (можно поменять даты), отклоняет с причиной или
//     останавливает идущую рекламу; может и сам запустить рекламу товара без заявки;
//   - у товара не больше одной «открытой» рекламы: заявка на рассмотрении или
//     одобренная, срок которой ещё не закончился (проверка под блокировкой строки товара);
//   - на главной (GET /v1/promotions, пакет products) показываются товары с идущей
//     рекламой: approved, starts_at <= now() < ends_at, товар опубликован, магазин активен.
//
// Оплата рекламы пока вне системы: администратор включает рекламу после договорённости.
package promotions

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/notify"
	"github.com/boutique-market/backend/internal/platform/db"
)

// Границы. MinDays/MaxDays продублированы в CHECK миграции 00050.
const (
	MinDays    = 1
	MaxDays    = 90
	MaxNote    = 500
	MaxPeriod  = 365 * 24 * time.Hour // самая длинная реклама, которую может задать администратор
	maxBackdue = 24 * time.Hour       // начало в прошлом — не раньше суток назад (опечатки в дате)
)

// Состояния для ответа и фильтра. scheduled/active/ended — это status='approved'
// в зависимости от текущего времени.
const (
	StatePending   = "pending"
	StateScheduled = "scheduled"
	StateActive    = "active"
	StateEnded     = "ended"
	StateRejected  = "rejected"
	StateStopped   = "stopped"
)

var stateSQL = map[string]string{
	StatePending:   "pr.status = 'pending'",
	StateScheduled: "pr.status = 'approved' AND now() < pr.starts_at",
	StateActive:    "pr.status = 'approved' AND pr.starts_at <= now() AND now() < pr.ends_at",
	StateEnded:     "pr.status = 'approved' AND pr.ends_at <= now()",
	StateRejected:  "pr.status = 'rejected'",
	StateStopped:   "pr.status = 'stopped'",
}

const stateExpr = `CASE WHEN pr.status = 'approved' AND now() < pr.starts_at THEN 'scheduled'
	WHEN pr.status = 'approved' AND pr.ends_at <= now() THEN 'ended'
	WHEN pr.status = 'approved' THEN 'active'
	ELSE pr.status::text END`

type Handlers struct {
	DB         *pgxpool.Pool
	PublicBase string
}

// StoreRoutes монтируются под /v1/store/promotions (роль store).
func (h *Handlers) StoreRoutes(r chi.Router) {
	r.Get("/", h.storeList)
	r.Post("/", h.storeRequest)
	r.Delete("/{id}", h.storeCancel)
}

// AdminRoutes монтируются под /v1/admin/promotions (роль admin).
func (h *Handlers) AdminRoutes(r chi.Router) {
	r.Get("/", h.adminList)
	r.Post("/", h.adminCreate)
	r.Post("/{id}/approve", h.adminApprove)
	r.Post("/{id}/reject", h.adminReject)
	r.Post("/{id}/stop", h.adminStop)
}

// Promotion — заявка или реклама в кабинете магазина и в админке.
type Promotion struct {
	ID           uuid.UUID  `json:"id"`
	ProductID    uuid.UUID  `json:"product_id"`
	ProductTitle string     `json:"product_title"`
	ProductPrice int64      `json:"product_price"`
	ImageURL     string     `json:"image_url,omitempty"`
	StoreID      uuid.UUID  `json:"store_id"`
	StoreName    string     `json:"store_name"`
	Status       string     `json:"status"` // pending, approved, rejected, stopped
	State        string     `json:"state"`  // pending, scheduled, active, ended, rejected, stopped
	Days         int        `json:"days"`
	StoreNote    string     `json:"store_note"`
	AdminReason  *string    `json:"admin_reason"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	DecidedAt    *time.Time `json:"decided_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

const cols = `pr.id, pr.product_id, p.title, p.price, ` + cardImageSQL + `, pr.store_id, s.name, pr.status::text, ` + stateExpr + `,
	pr.days, pr.store_note, pr.admin_reason, pr.starts_at, pr.ends_at, pr.decided_at, pr.created_at`

// cardImageSQL — миниатюра фото «спереди» (см. products.CardImageSQL).
const cardImageSQL = `(SELECT CASE WHEN pi.variants = 'ready' THEN left(pi.key, -4) || '_800.jpg' ELSE pi.key END
		FROM product_images pi WHERE pi.product_id = p.id AND pi.angle = 'front')`

const from = ` FROM product_promotions pr JOIN products p ON p.id = pr.product_id JOIN stores s ON s.id = pr.store_id `

func (h *Handlers) scan(rows pgx.Rows) ([]Promotion, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Promotion, error) {
		var v Promotion
		var key *string
		err := row.Scan(&v.ID, &v.ProductID, &v.ProductTitle, &v.ProductPrice, &key, &v.StoreID, &v.StoreName, &v.Status, &v.State,
			&v.Days, &v.StoreNote, &v.AdminReason, &v.StartsAt, &v.EndsAt, &v.DecidedAt, &v.CreatedAt)
		if key != nil {
			v.ImageURL = media.PublicURL(h.PublicBase, *key)
		}
		return v, err
	})
}

func (h *Handlers) load(ctx context.Context, q pgx.Tx, id uuid.UUID) (Promotion, error) {
	rows, err := q.Query(ctx, `SELECT `+cols+from+`WHERE pr.id = $1`, id)
	if err != nil {
		return Promotion{}, err
	}
	items, err := h.scan(rows)
	if err != nil {
		return Promotion{}, err
	}
	if len(items) == 0 {
		return Promotion{}, httpx.ErrNotFound
	}
	return items[0], nil
}

func pathID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrNotFound
	}
	return id, nil
}

// list — общий список с фильтром по состоянию и курсором.
func (h *Handlers) list(w http.ResponseWriter, r *http.Request, where []string, args []any) {
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	q := r.URL.Query()
	if v := q.Get("state"); v != "" {
		cond, ok := stateSQL[v]
		if !ok {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "unknown state"))
			return
		}
		where = append(where, cond)
	}
	if c := q.Get("cursor"); c != "" {
		t, id, err := decodeCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_cursor", "invalid cursor"))
			return
		}
		where = append(where, "(pr.created_at, pr.id) < ("+arg(t)+", "+arg(id)+")")
	}
	limit := httpx.Limit(r, 20, 100)
	// nosemgrep: go.lang.security.injection.tainted-sql-string.tainted-sql-string -- только константы и плейсхолдеры $N
	sql := `SELECT ` + cols + from + `WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY pr.created_at DESC, pr.id DESC LIMIT ` + strconv.Itoa(limit+1)
	rows, err := h.DB.Query(r.Context(), sql, args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items, err := h.scan(rows)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = encodeCursor(items[limit-1].CreatedAt, items[limit-1].ID)
	}
	if items == nil {
		items = []Promotion{}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.List(w, items, next)
}

// lockProduct блокирует строку товара (FOR NO KEY UPDATE — не мешает оформлению
// заказов, см. «Синхронизация» в ARCHITECTURE.md): две заявки (или заявка и одобрение) на один
// товар не проходят параллельно мимо проверки «одна открытая реклама».
func lockProduct(ctx context.Context, tx pgx.Tx, pid uuid.UUID) (storeID uuid.UUID, title, status string, err error) {
	err = tx.QueryRow(ctx, `SELECT store_id, title, status::text FROM products WHERE id = $1 FOR NO KEY UPDATE`, pid).Scan(&storeID, &title, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	return
}

// openExists — есть ли у товара другая заявка на рассмотрении или неистёкшая одобренная реклама.
func openExists(ctx context.Context, tx pgx.Tx, pid, except uuid.UUID) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM product_promotions
		WHERE product_id = $1 AND id <> $2 AND (status = 'pending' OR (status = 'approved' AND ends_at > now())))`, pid, except).Scan(&ok)
	return ok, err
}

var errExists = httpx.NewError(http.StatusConflict, "promotion_exists", "product already has a pending or running promotion")

// ---------- кабинет магазина ----------

// storeList: GET /v1/store/promotions?state=&cursor= — заявки и реклама своего магазина.
func (h *Handlers) storeList(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	h.list(w, r, []string{"pr.store_id = $1"}, []any{p.StoreID})
}

type requestReq struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Days      int       `json:"days"`
	Note      string    `json:"note" validate:"max=2000"` // байты; точная проверка — по символам ниже
}

// storeRequest: POST /v1/store/promotions — заявка на рекламу своего товара.
func (h *Handlers) storeRequest(w http.ResponseWriter, r *http.Request) {
	var req requestReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Note = cleanText(req.Note)
	fields := map[string]string{}
	if req.Days < MinDays || req.Days > MaxDays {
		fields["days"] = "range"
	}
	if len([]rune(req.Note)) > MaxNote {
		fields["note"] = "max"
	}
	if len(fields) > 0 {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "request validation failed", Fields: fields})
		return
	}
	p := auth.MustFrom(r.Context())
	var id uuid.UUID
	var out Promotion
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		storeID, _, status, err := lockProduct(r.Context(), tx, req.ProductID)
		if err != nil {
			return err
		}
		if storeID != p.StoreID {
			return httpx.ErrNotFound // чужой и несуществующий товар не различаются
		}
		if status != "published" {
			return httpx.NewError(http.StatusConflict, "product_not_published", "only published products can be promoted")
		}
		if ok, err := openExists(r.Context(), tx, req.ProductID, uuid.Nil); err != nil {
			return err
		} else if ok {
			return errExists
		}
		if err := tx.QueryRow(r.Context(), `INSERT INTO product_promotions (product_id, store_id, days, store_note)
			VALUES ($1, $2, $3, $4) RETURNING id`, req.ProductID, storeID, req.Days, req.Note).Scan(&id); err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "promotion.request", "promotion", id.String(),
			map[string]any{"product_id": req.ProductID, "days": req.Days}); err != nil {
			return err
		}
		out, err = h.load(r.Context(), tx, id)
		return err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

// storeCancel: DELETE /v1/store/promotions/{id} — отозвать заявку, пока её не рассмотрели.
func (h *Handlers) storeCancel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := auth.MustFrom(r.Context())
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(r.Context(), `SELECT status::text FROM product_promotions WHERE id = $1 AND store_id = $2 FOR UPDATE`, id, p.StoreID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if status != "pending" {
			return httpx.NewError(http.StatusConflict, "promotion_decided", "promotion was already reviewed by the platform")
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM product_promotions WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Log(r.Context(), tx, "promotion.cancel", "promotion", id.String(), nil)
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ---------- суперадмин ----------

// adminList: GET /v1/admin/promotions?state=&store=&cursor=
func (h *Handlers) adminList(w http.ResponseWriter, r *http.Request) {
	where := []string{"true"}
	var args []any
	if v := r.URL.Query().Get("store"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "store must be uuid"))
			return
		}
		args = append(args, id)
		where = append(where, "pr.store_id = $1")
	}
	h.list(w, r, where, args)
}

// txNow — время начала транзакции (now() в PostgreSQL). Срок по умолчанию считается
// от него, чтобы одобренная «сейчас» реклама сразу была active (state считается по now()).
func txNow(ctx context.Context, tx pgx.Tx) time.Time {
	var t time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&t); err != nil {
		return time.Now().UTC()
	}
	return t.UTC()
}

type periodReq struct {
	StartsAt *time.Time `json:"starts_at"` // по умолчанию — сейчас
	EndsAt   *time.Time `json:"ends_at"`   // по умолчанию — начало + срок заявки (дней)
}

// period проверяет и дополняет срок показа. days — срок по умолчанию.
func period(req periodReq, days int, now time.Time) (time.Time, time.Time, error) {
	start := now
	if req.StartsAt != nil {
		start = req.StartsAt.UTC()
	}
	end := start.Add(time.Duration(days) * 24 * time.Hour)
	if req.EndsAt != nil {
		end = req.EndsAt.UTC()
	}
	fields := map[string]string{}
	if start.Before(now.Add(-maxBackdue)) {
		fields["starts_at"] = "past"
	}
	if !end.After(start) || !end.After(now) {
		fields["ends_at"] = "before_start"
	} else if end.Sub(start) > MaxPeriod {
		fields["ends_at"] = "too_long"
	}
	if len(fields) > 0 {
		return start, end, &httpx.Error{Status: 422, Code: "validation_failed", Message: "invalid promotion period", Fields: fields}
	}
	return start, end, nil
}

type createReq struct {
	ProductID uuid.UUID  `json:"product_id" validate:"required"`
	Days      int        `json:"days"`
	StartsAt  *time.Time `json:"starts_at"`
	EndsAt    *time.Time `json:"ends_at"`
}

// adminCreate: POST /v1/admin/promotions — запустить рекламу товара без заявки магазина.
func (h *Handlers) adminCreate(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Days == 0 {
		req.Days = 7
	}
	if req.Days < MinDays || req.Days > MaxDays {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "request validation failed", Fields: map[string]string{"days": "range"}})
		return
	}
	var out Promotion
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		start, end, err := period(periodReq{StartsAt: req.StartsAt, EndsAt: req.EndsAt}, req.Days, txNow(r.Context(), tx))
		if err != nil {
			return err
		}
		storeID, title, status, err := lockProduct(r.Context(), tx, req.ProductID)
		if err != nil {
			return err
		}
		if status != "published" {
			return httpx.NewError(http.StatusConflict, "product_not_published", "only published products can be promoted")
		}
		if ok, err := openExists(r.Context(), tx, req.ProductID, uuid.Nil); err != nil {
			return err
		} else if ok {
			return errExists
		}
		var id uuid.UUID
		if err := tx.QueryRow(r.Context(), `INSERT INTO product_promotions (product_id, store_id, status, days, starts_at, ends_at, decided_at)
			VALUES ($1, $2, 'approved', $3, $4, $5, now()) RETURNING id`, req.ProductID, storeID, req.Days, start, end).Scan(&id); err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "promotion.create", "promotion", id.String(),
			map[string]any{"product_id": req.ProductID, "starts_at": start, "ends_at": end}); err != nil {
			return err
		}
		if err := notify.SendToStore(r.Context(), tx, storeID, notify.PromotionApproved,
			map[string]any{"product_id": req.ProductID, "title": title, "starts_at": start, "ends_at": end}); err != nil {
			return err
		}
		out, err = h.load(r.Context(), tx, id)
		return err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

// decide выполняет решение администратора по заявке. Порядок блокировок как у
// остальных путей: сначала строка товара, потом строка заявки.
func (h *Handlers) decide(w http.ResponseWriter, r *http.Request, fn func(tx pgx.Tx, pr Promotion, title string) error) {
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var out Promotion
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var pid uuid.UUID
		if err := tx.QueryRow(r.Context(), `SELECT product_id FROM product_promotions WHERE id = $1`, id).Scan(&pid); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.ErrNotFound
			}
			return err
		}
		_, title, _, err := lockProduct(r.Context(), tx, pid)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `SELECT 1 FROM product_promotions WHERE id = $1 FOR UPDATE`, id); err != nil {
			return err
		}
		pr, err := h.load(r.Context(), tx, id)
		if err != nil {
			return err
		}
		if err := fn(tx, pr, title); err != nil {
			return err
		}
		out, err = h.load(r.Context(), tx, id)
		return err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// adminApprove: POST /v1/admin/promotions/{id}/approve {starts_at?, ends_at?}
func (h *Handlers) adminApprove(w http.ResponseWriter, r *http.Request) {
	var req periodReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.decide(w, r, func(tx pgx.Tx, pr Promotion, title string) error {
		if pr.Status != "pending" {
			return httpx.NewError(http.StatusConflict, "promotion_decided", "only pending requests can be approved")
		}
		var status string
		if err := tx.QueryRow(r.Context(), `SELECT status::text FROM products WHERE id = $1`, pr.ProductID).Scan(&status); err != nil {
			return err
		}
		if status != "published" {
			return httpx.NewError(http.StatusConflict, "product_not_published", "only published products can be promoted")
		}
		if ok, err := openExists(r.Context(), tx, pr.ProductID, pr.ID); err != nil {
			return err
		} else if ok {
			return errExists
		}
		start, end, err := period(req, pr.Days, txNow(r.Context(), tx))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `UPDATE product_promotions SET status = 'approved', starts_at = $2, ends_at = $3,
			decided_at = now(), admin_reason = NULL WHERE id = $1`, pr.ID, start, end); err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "promotion.approve", "promotion", pr.ID.String(),
			map[string]any{"starts_at": start, "ends_at": end}); err != nil {
			return err
		}
		return notify.SendToStore(r.Context(), tx, pr.StoreID, notify.PromotionApproved,
			map[string]any{"product_id": pr.ProductID, "title": title, "starts_at": start, "ends_at": end})
	})
}

type reasonReq struct {
	Reason string `json:"reason" validate:"max=2000"`
}

func readReason(w http.ResponseWriter, r *http.Request, required bool) (string, bool) {
	var req reasonReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return "", false
	}
	req.Reason = cleanText(req.Reason)
	n := len([]rune(req.Reason))
	if (required && n < 3) || n > MaxNote {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "reason is required", Fields: map[string]string{"reason": "length"}})
		return "", false
	}
	return req.Reason, true
}

// adminReject: POST /v1/admin/promotions/{id}/reject {reason} — отклонить заявку.
func (h *Handlers) adminReject(w http.ResponseWriter, r *http.Request) {
	reason, ok := readReason(w, r, true)
	if !ok {
		return
	}
	h.decide(w, r, func(tx pgx.Tx, pr Promotion, title string) error {
		if pr.Status != "pending" {
			return httpx.NewError(http.StatusConflict, "promotion_decided", "only pending requests can be rejected")
		}
		if _, err := tx.Exec(r.Context(), `UPDATE product_promotions SET status = 'rejected', admin_reason = $2, decided_at = now()
			WHERE id = $1`, pr.ID, reason); err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "promotion.reject", "promotion", pr.ID.String(), map[string]any{"reason": reason}); err != nil {
			return err
		}
		return notify.SendToStore(r.Context(), tx, pr.StoreID, notify.PromotionRejected,
			map[string]any{"product_id": pr.ProductID, "title": title, "reason": reason})
	})
}

// adminStop: POST /v1/admin/promotions/{id}/stop {reason?} — снять идущую или запланированную рекламу.
func (h *Handlers) adminStop(w http.ResponseWriter, r *http.Request) {
	reason, ok := readReason(w, r, false)
	if !ok {
		return
	}
	h.decide(w, r, func(tx pgx.Tx, pr Promotion, title string) error {
		if pr.State != StateActive && pr.State != StateScheduled {
			return httpx.NewError(http.StatusConflict, "promotion_not_running", "only running or scheduled promotions can be stopped")
		}
		// ends_at обрезается до текущего момента, чтобы в истории был реальный срок показа
		// (для запланированной рекламы — равным началу нельзя по CHECK, поэтому +1 мкс).
		if _, err := tx.Exec(r.Context(), `UPDATE product_promotions SET status = 'stopped', admin_reason = NULLIF($2, ''),
			ends_at = GREATEST(starts_at + interval '1 microsecond', LEAST(ends_at, now())), decided_at = now() WHERE id = $1`, pr.ID, reason); err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "promotion.stop", "promotion", pr.ID.String(), map[string]any{"reason": reason}); err != nil {
			return err
		}
		return notify.SendToStore(r.Context(), tx, pr.StoreID, notify.PromotionStopped,
			map[string]any{"product_id": pr.ProductID, "title": title, "reason": reason})
	})
}

// cleanText убирает управляющие символы (кроме переводов строк) и пробелы по краям.
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' {
			return r
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// ---------- курсор ----------

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	s := string(b)
	i := strings.LastIndexByte(s, '|')
	if i < 0 {
		return time.Time{}, uuid.Nil, errors.New("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, s[:i])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(s[i+1:])
	return t, id, err
}
