package payments

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/ratelimit"
)

var ruleCreate = ratelimit.Rule{Name: "payment_create", Limit: 10, Window: time.Minute}

type Handlers struct {
	DB      *pgxpool.Pool
	Cfg     config.Config
	Limiter *ratelimit.Limiter
	Click   *Click
	Payme   *Payme
}

// New собирает обработчики из конфигурации. Ошибка — неверный список IP.
func New(pool *pgxpool.Pool, cfg config.Config, limiter *ratelimit.Limiter) (*Handlers, error) {
	clickIPs, err := parseAllow(cfg.ClickAllowedIPs)
	if err != nil {
		return nil, err
	}
	paymeIPs, err := parseAllow(cfg.PaymeAllowedIPs)
	if err != nil {
		return nil, err
	}
	h := &Handlers{DB: pool, Cfg: cfg, Limiter: limiter}
	h.Click = &Click{DB: pool, Allowed: clickIPs}
	if cfg.ClickEnabled() {
		h.Click.ServiceID, h.Click.SecretKey = cfg.ClickServiceID, cfg.ClickSecretKey
	}
	h.Payme = &Payme{DB: pool, AccountKey: cfg.PaymeAccountKey, Allowed: paymeIPs,
		IKPU: cfg.PaymeIKPU, PackageCode: cfg.PaymePackageCode, VATPercent: cfg.PaymeVATPercent}
	if h.Payme.AccountKey == "" {
		h.Payme.AccountKey = "payment_id"
	}
	if cfg.PaymeEnabled() {
		h.Payme.Key = cfg.PaymeKey
	}
	return h, nil
}

// WebhookRoutes — вызываются провайдерами, без пользовательского токена (подпись/ключ кассы).
func (h *Handlers) WebhookRoutes(r chi.Router) {
	r.Post("/payments/click/prepare", h.Click.Prepare)
	r.Post("/payments/click/complete", h.Click.Complete)
	r.Post("/payments/payme", h.Payme.ServeHTTP)
}

func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Get("/tariffs", h.publicTariffs)
}

// ClientRoutes монтируются за RequireRole(client).
func (h *Handlers) ClientRoutes(r chi.Router) {
	r.Post("/payments", h.create)
	r.Get("/payments", h.listMine)
	r.Get("/payments/{id}", h.getMine)
}

// AdminRoutes монтируются под /v1/admin.
func (h *Handlers) AdminRoutes(r chi.Router) {
	r.Get("/tariffs", h.adminTariffs)
	r.Post("/tariffs", h.createTariff)
	r.Patch("/tariffs/{id}", h.updateTariff)
	r.Get("/payments", h.adminPayments)
}

// ---------- тарифы ----------

type Tariff struct {
	ID          uuid.UUID `json:"id"`
	NameRU      string    `json:"name_ru"`
	NameUZ      string    `json:"name_uz"`
	Description string    `json:"description"`
	Tokens      int64     `json:"tokens"`
	Price       int64     `json:"price"`
	Highlighted bool      `json:"highlighted"`
	IsPublished bool      `json:"is_published"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const tariffCols = `id, name_ru, name_uz, description, tokens::bigint, price, highlighted, is_published, created_at, updated_at`

func (h *Handlers) listTariffs(w http.ResponseWriter, r *http.Request, onlyPublished bool) {
	rows, err := h.DB.Query(r.Context(), `SELECT `+tariffCols+` FROM tariffs
		WHERE is_published OR NOT $1 ORDER BY price, id`, onlyPublished)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Tariff])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.List(w, items, "")
}

func (h *Handlers) publicTariffs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=60")
	h.listTariffs(w, r, true)
}

func (h *Handlers) adminTariffs(w http.ResponseWriter, r *http.Request) { h.listTariffs(w, r, false) }

type tariffReq struct {
	NameRU      *string `json:"name_ru" validate:"omitempty,min=1,max=100"`
	NameUZ      *string `json:"name_uz" validate:"omitempty,min=1,max=100"`
	Description *string `json:"description" validate:"omitempty,max=500"`
	Tokens      *int64  `json:"tokens" validate:"omitempty,min=1,max=100000"`
	Price       *int64  `json:"price" validate:"omitempty,min=100,max=100000000000"` // тийины: от 1 сума
	Highlighted *bool   `json:"highlighted"`
	IsPublished *bool   `json:"is_published"`
}

func (h *Handlers) createTariff(w http.ResponseWriter, r *http.Request) {
	var req tariffReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	missing := map[string]string{}
	if req.NameRU == nil {
		missing["name_ru"] = "required"
	}
	if req.NameUZ == nil {
		missing["name_uz"] = "required"
	}
	if req.Tokens == nil {
		missing["tokens"] = "required"
	}
	if req.Price == nil {
		missing["price"] = "required"
	}
	if len(missing) > 0 {
		httpx.Fail(w, r, &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "request validation failed", Fields: missing})
		return
	}
	var t Tariff
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		row := tx.QueryRow(r.Context(), `
			INSERT INTO tariffs (name_ru, name_uz, description, tokens, price, highlighted, is_published)
			VALUES ($1, $2, COALESCE($3, ''), $4, $5, COALESCE($6, false), COALESCE($7, false))
			RETURNING `+tariffCols, req.NameRU, req.NameUZ, req.Description, req.Tokens, req.Price, req.Highlighted, req.IsPublished)
		if err := row.Scan(&t.ID, &t.NameRU, &t.NameUZ, &t.Description, &t.Tokens, &t.Price, &t.Highlighted, &t.IsPublished, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return err
		}
		return audit.Log(r.Context(), tx, "tariff.create", "tariff", t.ID.String(), map[string]any{"tokens": t.Tokens, "price": t.Price})
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

// updateTariff меняет тариф. Уже созданные платежи не затрагиваются: сумма и токены в них — снимок.
func (h *Handlers) updateTariff(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req tariffReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var t Tariff
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		row := tx.QueryRow(r.Context(), `
			UPDATE tariffs SET
			  name_ru = COALESCE($2, name_ru), name_uz = COALESCE($3, name_uz),
			  description = COALESCE($4, description), tokens = COALESCE($5, tokens),
			  price = COALESCE($6, price), highlighted = COALESCE($7, highlighted),
			  is_published = COALESCE($8, is_published)
			WHERE id = $1 RETURNING `+tariffCols,
			id, req.NameRU, req.NameUZ, req.Description, req.Tokens, req.Price, req.Highlighted, req.IsPublished)
		if err := row.Scan(&t.ID, &t.NameRU, &t.NameUZ, &t.Description, &t.Tokens, &t.Price, &t.Highlighted, &t.IsPublished, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return db.NotFound(err)
		}
		return audit.Log(r.Context(), tx, "tariff.update", "tariff", id.String(), map[string]any{
			"tokens": t.Tokens, "price": t.Price, "is_published": t.IsPublished})
	})
	if errors.Is(err, db.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

// ---------- платежи клиента ----------

type Payment struct {
	ID          uuid.UUID  `json:"id"`
	Number      int64      `json:"number"`
	UserID      uuid.UUID  `json:"user_id"`
	TariffID    uuid.UUID  `json:"tariff_id"`
	Provider    string     `json:"provider"`
	Amount      int64      `json:"amount"`
	Tokens      int64      `json:"tokens"`
	Status      string     `json:"status"`
	PaidAt      *time.Time `json:"paid_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
	CreatedAt   time.Time  `json:"created_at"`
	CheckoutURL string     `json:"checkout_url,omitempty"`
}

const paymentCols = `id, number, user_id, tariff_id, provider::text, amount, tokens::bigint, status::text, paid_at, cancelled_at, created_at`

func scanPayment(row pgx.Row) (*Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.Number, &p.UserID, &p.TariffID, &p.Provider, &p.Amount, &p.Tokens, &p.Status, &p.PaidAt, &p.CancelledAt, &p.CreatedAt)
	if err != nil {
		return nil, db.NotFound(err)
	}
	return &p, nil
}

func (h *Handlers) enabled(provider string) bool {
	switch provider {
	case ProviderClick:
		return h.Cfg.ClickEnabled()
	case ProviderPayme:
		return h.Cfg.PaymeEnabled()
	}
	return false
}

func (h *Handlers) returnURL(p *Payment) string {
	u, err := url.Parse(h.Cfg.PaymentReturnURL)
	if err != nil || h.Cfg.PaymentReturnURL == "" {
		return ""
	}
	q := u.Query()
	q.Set("payment", p.ID.String())
	u.RawQuery = q.Encode()
	return u.String()
}

// CheckoutURL — ссылка на платёжную страницу провайдера (только для pending).
func (h *Handlers) CheckoutURL(p *Payment) string {
	if p.Status != StatusPending {
		return ""
	}
	switch p.Provider {
	case ProviderClick:
		q := url.Values{
			"service_id":        {h.Cfg.ClickServiceID},
			"merchant_id":       {h.Cfg.ClickMerchantID},
			"amount":            {formatSums(p.Amount)},
			"transaction_param": {p.ID.String()},
		}
		if ru := h.returnURL(p); ru != "" {
			q.Set("return_url", ru)
		}
		return "https://my.click.uz/services/pay?" + q.Encode()
	case ProviderPayme:
		base := "https://checkout.paycom.uz/"
		if h.Cfg.PaymeTest {
			base = "https://checkout.test.paycom.uz/"
		}
		parts := []string{"m=" + h.Cfg.PaymeMerchantID, "ac." + h.Payme.AccountKey + "=" + p.ID.String(),
			"a=" + strconv.FormatInt(p.Amount, 10)}
		if ru := h.returnURL(p); ru != "" && !strings.ContainsAny(ru, ";") {
			parts = append(parts, "c="+ru)
		}
		return base + base64.StdEncoding.EncodeToString([]byte(strings.Join(parts, ";")))
	}
	return ""
}

type createReq struct {
	TariffID uuid.UUID `json:"tariff_id" validate:"required"`
	Provider string    `json:"provider" validate:"required,oneof=click payme"`
}

// create: POST /v1/payments. Заголовок Idempotency-Key необязателен; с ним повтор
// возвращает тот же платёж, а не создаёт второй.
func (h *Handlers) create(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	if h.Limiter != nil && !h.Limiter.Check(w, r, ruleCreate, uid.String()) {
		return
	}
	var req createReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.enabled(req.Provider) {
		httpx.Fail(w, r, httpx.NewError(http.StatusUnprocessableEntity, "provider_unavailable", "payment provider is not available"))
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key != "" && (len(key) < 8 || len(key) > 128) {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_idempotency_key", "Idempotency-Key must be 8..128 chars"))
		return
	}
	var p *Payment
	status := http.StatusCreated
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var err error
		p, err = scanPayment(tx.QueryRow(r.Context(), `
			INSERT INTO payments (user_id, tariff_id, provider, amount, tokens, idempotency_key)
			SELECT $1::uuid, t.id, $3::payment_provider, t.price, t.tokens, NULLIF($4::text, '') FROM tariffs t WHERE t.id = $2 AND t.is_published
			ON CONFLICT (user_id, idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
			RETURNING `+paymentCols, uid, req.TariffID, req.Provider, key))
		if !errors.Is(err, db.ErrNotFound) {
			return err
		}
		if key == "" {
			return httpx.NewError(http.StatusUnprocessableEntity, "tariff_not_found", "tariff not found or not published")
		}
		// повтор с тем же ключом (или тариф не найден)
		p, err = scanPayment(tx.QueryRow(r.Context(), `SELECT `+paymentCols+` FROM payments
			WHERE user_id = $1 AND idempotency_key = $2`, uid, key))
		if errors.Is(err, db.ErrNotFound) {
			return httpx.NewError(http.StatusUnprocessableEntity, "tariff_not_found", "tariff not found or not published")
		}
		if err != nil {
			return err
		}
		if p.TariffID != req.TariffID || p.Provider != req.Provider {
			return httpx.NewError(http.StatusUnprocessableEntity, "idempotency_key_reused", "Idempotency-Key was used with a different request")
		}
		status = http.StatusOK
		return nil
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p.CheckoutURL = h.CheckoutURL(p)
	httpx.JSON(w, status, p)
}

func (h *Handlers) getMine(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	p, err := scanPayment(h.DB.QueryRow(r.Context(), `SELECT `+paymentCols+` FROM payments WHERE id = $1 AND user_id = $2`, id, uid))
	if errors.Is(err, db.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p.CheckoutURL = h.CheckoutURL(p)
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handlers) listMine(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, auth.MustFrom(r.Context()).UserID.String(), "", "")
}

func (h *Handlers) adminPayments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if s := q.Get("status"); s != "" && s != StatusPending && s != StatusPaid && s != StatusDeclined && s != StatusCancelled {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_filter", "unknown status"))
		return
	}
	if s := q.Get("provider"); s != "" && s != ProviderClick && s != ProviderPayme {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_filter", "unknown provider"))
		return
	}
	if s := q.Get("user_id"); s != "" {
		if _, err := uuid.Parse(s); err != nil {
			httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_filter", "invalid user_id"))
			return
		}
	}
	h.list(w, r, q.Get("user_id"), q.Get("status"), q.Get("provider"))
}

// list — keyset-пагинация по number (новые сверху).
func (h *Handlers) list(w http.ResponseWriter, r *http.Request, userID, status, provider string) {
	limit := httpx.Limit(r, 20, 100)
	before := int64(1 << 62)
	if c := r.URL.Query().Get("cursor"); c != "" {
		v, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_cursor", "invalid cursor"))
			return
		}
		before = v
	}
	rows, err := h.DB.Query(r.Context(), `SELECT `+paymentCols+` FROM payments
		WHERE number < $1
		  AND ($2::text = '' OR user_id = NULLIF($2::text, '')::uuid)
		  AND ($3::text = '' OR status = NULLIF($3::text, '')::payment_status)
		  AND ($4::text = '' OR provider = NULLIF($4::text, '')::payment_provider)
		ORDER BY number DESC LIMIT $5`, before, userID, status, provider, limit+1)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := []*Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.FormatInt(items[limit-1].Number, 10)
	}
	httpx.List(w, items, next)
}
