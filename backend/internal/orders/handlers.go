package orders

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/ratelimit"
)

// PhotoURLTTL — сколько живёт подписанная ссылка на фото места доставки.
const PhotoURLTTL = 5 * time.Minute

const idemScope = "checkout"

type Handlers struct {
	DB      *pgxpool.Pool
	Media   media.Storage
	Limiter *ratelimit.Limiter
	// PublicBase — базовый URL CDN для публичных картинок товаров.
	PublicBase string
}

var ruleCheckout = ratelimit.Rule{Name: "checkout_user", Limit: 20, Window: time.Hour}

// ClientRoutes монтируются под /v1/orders (только роль client).
func (h *Handlers) ClientRoutes(r chi.Router) {
	r.Post("/", h.checkout)
	r.Get("/", h.listHandler)
	r.Get("/{id}", h.get)
}

// StoreRoutes монтируются под /v1/store/orders (только роль store).
func (h *Handlers) StoreRoutes(r chi.Router) {
	r.Get("/", h.listHandler)
	r.Get("/{id}", h.get)
	r.Post("/{id}/status", h.setStatus)
}

// AdminRoutes монтируются под /v1/admin/orders (только роль admin).
func (h *Handlers) AdminRoutes(r chi.Router) {
	r.Get("/", h.listHandler)
	r.Get("/{id}", h.get)
	r.Post("/{id}/status", h.setStatus)
}

// ===================== Оформление заказа =====================

type checkoutItem struct {
	ProductID uuid.UUID `json:"product_id"`
	Qty       int       `json:"qty"`
}

type deliveryReq struct {
	Street         string   `json:"street"`
	House          string   `json:"house"`
	Entrance       string   `json:"entrance"`
	Floor          string   `json:"floor"`
	Apartment      string   `json:"apartment"`
	Intercom       string   `json:"intercom"`
	AddressComment string   `json:"address_comment"`
	Lat            *float64 `json:"lat"`
	Lng            *float64 `json:"lng"`
	PhotoKey       string   `json:"photo_key"`
}

type checkoutReq struct {
	Items               []checkoutItem `json:"items"`
	CustomerName        string         `json:"customer_name"`
	CustomerPhone       string         `json:"customer_phone"` // необязательно; по умолчанию телефон аккаунта
	CustomerComment     string         `json:"customer_comment"`
	DeliveryTimeComment string         `json:"delivery_time_comment"`
	Delivery            deliveryReq    `json:"delivery"`
}

type createdOrder struct {
	ID         uuid.UUID `json:"id"`
	Number     int64     `json:"number"`
	StoreID    uuid.UUID `json:"store_id"`
	StoreName  string    `json:"store_name"`
	Status     Status    `json:"status"`
	ItemsTotal int64     `json:"items_total"`
}

type checkoutResp struct {
	CheckoutID uuid.UUID      `json:"checkout_id"`
	Orders     []createdOrder `json:"orders"`
}

type product struct {
	id, storeID         uuid.UUID
	storeName           string
	title, brand        string
	imageKey            *string
	price               int64
	inStock             bool
	status, storeStatus string
}

func validationErr(fields map[string]string) error {
	return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "request validation failed", Fields: fields}
}

// checkout: POST /v1/orders (заголовок Idempotency-Key обязателен).
// Корзина делится на заказы по магазинам; всё в одной транзакции.
func (h *Handlers) checkout(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idemKey) < 8 || len(idemKey) > 128 {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header (8..128 chars) is required"))
		return
	}
	if h.Limiter != nil && !h.Limiter.Check(w, r, ruleCheckout, p.UserID.String()) {
		return
	}
	var req checkoutReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if fields := req.Normalize(); fields != nil {
		httpx.Fail(w, r, validationErr(fields))
		return
	}
	if req.CustomerPhone != "" && !httpx.IsE164(req.CustomerPhone) {
		httpx.Fail(w, r, validationErr(map[string]string{"customer_phone": "e164"}))
		return
	}
	if req.Delivery.PhotoKey != "" && !media.OwnsKey(p.UserID, media.PurposeDeliveryPhoto, req.Delivery.PhotoKey) {
		httpx.Fail(w, r, validationErr(map[string]string{"delivery.photo_key": "invalid"}))
		return
	}
	canon, _ := json.Marshal(req) // хэш нормализованного запроса
	reqHash := sha256.Sum256(canon)

	var status int
	var body []byte
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		// Повторный запрос с тем же ключом получает сохранённый ответ.
		// Параллельный дубль блокируется на INSERT до коммита первого.
		tag, err := tx.Exec(r.Context(), `
			INSERT INTO idempotency_keys (user_id, scope, key, request_hash) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, p.UserID, idemScope, idemKey, reqHash[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var prevHash []byte
			var prevStatus *int
			if err := tx.QueryRow(r.Context(), `SELECT request_hash, status_code, response FROM idempotency_keys
				WHERE user_id = $1 AND scope = $2 AND key = $3`, p.UserID, idemScope, idemKey).Scan(&prevHash, &prevStatus, &body); err != nil {
				return err
			}
			if string(prevHash) != string(reqHash[:]) {
				return httpx.NewError(http.StatusUnprocessableEntity, "idempotency_key_reused", "Idempotency-Key was used with a different request")
			}
			if prevStatus == nil {
				return httpx.NewError(http.StatusConflict, "request_in_progress", "request with this Idempotency-Key is in progress")
			}
			status = *prevStatus
			return nil
		}

		resp, err := h.createOrders(r.Context(), tx, p, &req)
		if err != nil {
			return err
		}
		body, _ = json.Marshal(map[string]any{"data": resp})
		status = http.StatusCreated
		_, err = tx.Exec(r.Context(), `UPDATE idempotency_keys SET status_code = $4, response = $5
			WHERE user_id = $1 AND scope = $2 AND key = $3`, p.UserID, idemScope, idemKey, status, body)
		return err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (h *Handlers) createOrders(ctx context.Context, tx pgx.Tx, p auth.Principal, req *checkoutReq) (*checkoutResp, error) {
	if req.CustomerPhone == "" {
		var phone *string
		if err := tx.QueryRow(ctx, `SELECT phone FROM users WHERE id = $1`, p.UserID).Scan(&phone); err != nil {
			return nil, err
		}
		if phone == nil {
			return nil, validationErr(map[string]string{"customer_phone": "required"})
		}
		req.CustomerPhone = *phone
	}

	ids := make([]uuid.UUID, len(req.Items))
	for i, it := range req.Items {
		ids[i] = it.ProductID
	}
	// FOR SHARE: товар не снимут с публикации посреди оформления.
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.store_id, s.name, p.title, p.brand,
		       (SELECT key FROM product_images pi WHERE pi.product_id = p.id AND pi.angle = 'front'),
		       p.price, p.in_stock, p.status::text, s.status::text
		FROM products p JOIN stores s ON s.id = p.store_id
		WHERE p.id = ANY($1)
		FOR SHARE OF p, s`, ids)
	if err != nil {
		return nil, err
	}
	byID := map[uuid.UUID]product{}
	for rows.Next() {
		var pr product
		if err := rows.Scan(&pr.id, &pr.storeID, &pr.storeName, &pr.title, &pr.brand, &pr.imageKey,
			&pr.price, &pr.inStock, &pr.status, &pr.storeStatus); err != nil {
			rows.Close()
			return nil, err
		}
		byID[pr.id] = pr
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	unavailable := map[string]string{}
	for _, id := range ids {
		pr, ok := byID[id]
		switch {
		case !ok || pr.status != "published" || pr.storeStatus != "active":
			unavailable[id.String()] = "unavailable"
		case !pr.inStock:
			unavailable[id.String()] = "out_of_stock"
		}
	}
	if len(unavailable) > 0 {
		return nil, &httpx.Error{Status: http.StatusConflict, Code: "products_unavailable",
			Message: "some products cannot be ordered", Fields: unavailable}
	}

	// Группировка по магазинам в порядке появления в корзине.
	type group struct {
		storeID   uuid.UUID
		storeName string
		items     []checkoutItem
		total     int64
	}
	var groups []*group
	byStore := map[uuid.UUID]*group{}
	for _, it := range req.Items {
		pr := byID[it.ProductID]
		g := byStore[pr.storeID]
		if g == nil {
			g = &group{storeID: pr.storeID, storeName: pr.storeName}
			byStore[pr.storeID] = g
			groups = append(groups, g)
		}
		g.items = append(g.items, it)
		g.total += pr.price * int64(it.Qty)
	}

	d := req.Delivery
	resp := &checkoutResp{CheckoutID: uuid.New()}
	for _, g := range groups {
		o := createdOrder{StoreID: g.storeID, StoreName: g.storeName, Status: StatusPending, ItemsTotal: g.total}
		if err := tx.QueryRow(ctx, `
			INSERT INTO orders (checkout_id, user_id, store_id, items_total, customer_name, customer_phone,
			                    customer_comment, delivery_time_comment)
			VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''))
			RETURNING id, number`,
			resp.CheckoutID, p.UserID, g.storeID, g.total, req.CustomerName, req.CustomerPhone,
			req.CustomerComment, req.DeliveryTimeComment).Scan(&o.ID, &o.Number); err != nil {
			return nil, err
		}
		batch := &pgx.Batch{}
		for _, it := range g.items {
			pr := byID[it.ProductID]
			batch.Queue(`INSERT INTO order_items (order_id, product_id, title, brand, image_key, unit_price, qty)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`, o.ID, pr.id, pr.title, pr.brand, pr.imageKey, pr.price, it.Qty)
		}
		batch.Queue(`INSERT INTO order_deliveries (order_id, street, house, entrance, floor, apartment, intercom,
				address_comment, lat, lng, photo_key)
			VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9, $10, NULLIF($11, ''))`,
			o.ID, d.Street, d.House, d.Entrance, d.Floor, d.Apartment, d.Intercom, d.AddressComment, d.Lat, d.Lng, d.PhotoKey)
		batch.Queue(`INSERT INTO order_status_history (order_id, from_status, to_status, actor_id, actor_role)
			VALUES ($1, NULL, 'pending', $2, $3)`, o.ID, p.UserID, string(p.Role))
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return nil, err
		}
		resp.Orders = append(resp.Orders, o)
	}
	// Оформленные товары убираем из корзины.
	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE user_id = $1 AND product_id = ANY($2)`, p.UserID, ids); err != nil {
		return nil, err
	}
	return resp, nil
}

// ===================== Просмотр заказов =====================

type Item struct {
	ProductID *uuid.UUID `json:"product_id"`
	Title     string     `json:"title"`
	Brand     string     `json:"brand"`
	ImageURL  string     `json:"image_url,omitempty"`
	UnitPrice int64      `json:"unit_price"`
	Qty       int        `json:"qty"`
	imageKey  *string
}

type HistoryEntry struct {
	FromStatus *Status   `json:"from_status"`
	ToStatus   Status    `json:"to_status"`
	Reason     *string   `json:"reason,omitempty"`
	ActorRole  *string   `json:"actor_role,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type Summary struct {
	ID           uuid.UUID `json:"id"`
	Number       int64     `json:"number"`
	CheckoutID   uuid.UUID `json:"checkout_id"`
	StoreID      uuid.UUID `json:"store_id"`
	StoreName    string    `json:"store_name"`
	Status       Status    `json:"status"`
	Step         int       `json:"step"` // 0..4 для шкалы прогресса клиента
	ItemsTotal   int64     `json:"items_total"`
	ItemsCount   int       `json:"items_count"`
	CustomerName string    `json:"customer_name,omitempty"`
	ShortAddress string    `json:"short_address,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Detail struct {
	Summary
	CustomerPhone       string         `json:"customer_phone"`
	CustomerComment     *string        `json:"customer_comment"`
	DeliveryTimeComment *string        `json:"delivery_time_comment"`
	RejectionReason     *string        `json:"rejection_reason"`
	Delivery            DeliveryView   `json:"delivery"`
	Items               []Item         `json:"items"`
	History             []HistoryEntry `json:"history"`
	NextStatuses        []Status       `json:"next_statuses,omitempty"` // только магазину/админу
	CopyText            string         `json:"copy_text,omitempty"`     // только магазину/админу
}

// scope — фильтр видимости. Каждый запрос к заказам обязан идти через него.
func scope(p auth.Principal) (string, any) {
	switch p.Role {
	case auth.RoleClient:
		return "o.user_id = $1", p.UserID
	case auth.RoleStore:
		return "o.store_id = $1", p.StoreID
	case auth.RoleAdmin:
		return "$1::uuid IS NOT NULL", uuid.Nil // админ видит всё
	}
	return "false AND $1::uuid IS NULL", uuid.Nil
}

const summaryCols = `o.id, o.number, o.checkout_id, o.store_id, s.name, o.status::text, o.items_total,
	(SELECT COALESCE(sum(qty), 0) FROM order_items oi WHERE oi.order_id = o.id)::int,
	o.customer_name, d.street || ', ' || d.house, o.created_at, o.updated_at`

func scanSummary(row pgx.Row, s *Summary, extra ...any) error {
	var short *string
	dst := append([]any{&s.ID, &s.Number, &s.CheckoutID, &s.StoreID, &s.StoreName, &s.Status, &s.ItemsTotal,
		&s.ItemsCount, &s.CustomerName, &short, &s.CreatedAt, &s.UpdatedAt}, extra...)
	if err := row.Scan(dst...); err != nil {
		return err
	}
	if short != nil {
		s.ShortAddress = *short
	}
	s.Step = CustomerStep(s.Status)
	return nil
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, errors.New("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	u, err := uuid.Parse(id)
	return t, u, err
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request, p auth.Principal) {
	where, scopeArg := scope(p)
	args := []any{scopeArg}
	conds := []string{where}
	q := r.URL.Query()
	if st := q.Get("status"); st != "" {
		var statuses []string
		for _, s := range strings.Split(st, ",") {
			if !Status(s).Valid() {
				httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_filter", "unknown status"))
				return
			}
			statuses = append(statuses, s)
		}
		args = append(args, statuses)
		conds = append(conds, "o.status::text = ANY($2)")
	}
	if c := q.Get("cursor"); c != "" {
		t, id, err := decodeCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_cursor", "invalid cursor"))
			return
		}
		args = append(args, t, id)
		n := len(args)
		conds = append(conds, "(o.created_at, o.id) < ($"+strconv.Itoa(n-1)+", $"+strconv.Itoa(n)+")")
	}
	limit := httpx.Limit(r, 20, 50)
	rows, err := h.DB.Query(r.Context(), `
		SELECT `+summaryCols+`
		FROM orders o JOIN stores s ON s.id = o.store_id
		LEFT JOIN order_deliveries d ON d.order_id = o.id
		WHERE `+strings.Join(conds, " AND ")+`
		ORDER BY o.created_at DESC, o.id DESC LIMIT `+strconv.Itoa(limit+1), args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := []Summary{}
	for rows.Next() {
		var s Summary
		if err := scanSummary(rows, &s); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if p.Role == auth.RoleClient {
			s.CustomerName, s.ShortAddress = "", "" // клиенту в списке не нужно
		}
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	httpx.List(w, items, next)
}

func (h *Handlers) listHandler(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, auth.MustFrom(r.Context()))
}

// load читает заказ с учётом видимости. Чужой заказ => not_found (не 403),
// чтобы не раскрывать существование чужих id.
func (h *Handlers) load(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, p auth.Principal, id uuid.UUID) (*Detail, error) {
	where, scopeArg := scope(p)
	var o Detail
	var d DeliveryView
	var street, house, entrance, floor, apt, intercom, addrComment, photoKey *string
	row := q.QueryRow(ctx, `
		SELECT `+summaryCols+`, o.customer_phone, o.customer_comment, o.delivery_time_comment, o.rejection_reason,
		       d.street, d.house, d.entrance, d.floor, d.apartment, d.intercom, d.address_comment,
		       d.lat::float8, d.lng::float8, d.photo_key
		FROM orders o JOIN stores s ON s.id = o.store_id
		LEFT JOIN order_deliveries d ON d.order_id = o.id
		WHERE `+where+` AND o.id = $2`, scopeArg, id)
	if err := scanSummary(row, &o.Summary, &o.CustomerPhone, &o.CustomerComment, &o.DeliveryTimeComment, &o.RejectionReason,
		&street, &house, &entrance, &floor, &apt, &intercom, &addrComment, &d.Lat, &d.Lng, &photoKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, httpx.ErrNotFound
		}
		return nil, err
	}
	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	d.Street, d.House, d.Entrance, d.Floor = deref(street), deref(house), deref(entrance), deref(floor)
	d.Apartment, d.Intercom, d.AddressComment = deref(apt), deref(intercom), deref(addrComment)
	d.photoKey = deref(photoKey)
	d.HasPhoto = d.photoKey != ""
	d.MapsURL = mapsURL(d.Lat, d.Lng)
	o.Delivery = d

	rows, err := q.Query(ctx, `SELECT product_id, title, brand, image_key, unit_price, qty
		FROM order_items WHERE order_id = $1 ORDER BY title, id`, id)
	if err != nil {
		return nil, err
	}
	o.Items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		var it Item
		err := row.Scan(&it.ProductID, &it.Title, &it.Brand, &it.imageKey, &it.UnitPrice, &it.Qty)
		return it, err
	})
	if err != nil {
		return nil, err
	}
	rows, err = q.Query(ctx, `SELECT from_status::text, to_status::text, reason, actor_role::text, created_at
		FROM order_status_history WHERE order_id = $1 ORDER BY created_at, id`, id)
	if err != nil {
		return nil, err
	}
	o.History, err = pgx.CollectRows(rows, pgx.RowToStructByPos[HistoryEntry])
	if err != nil {
		return nil, err
	}
	if p.Role == auth.RoleClient {
		for i := range o.History {
			o.History[i].ActorRole = nil // клиенту не нужно знать, кто именно менял статус
		}
	}
	return &o, nil
}

// sign проставляет подписанные ссылки на фото (только после проверки доступа).
func (h *Handlers) sign(ctx context.Context, o *Detail) {
	if o.Delivery.photoKey != "" {
		if u, err := h.Media.PresignGet(ctx, o.Delivery.photoKey, PhotoURLTTL); err == nil {
			o.Delivery.PhotoURL = u
		}
	}
	for i := range o.Items {
		if k := o.Items[i].imageKey; k != nil {
			o.Items[i].ImageURL = media.PublicURL(h.PublicBase, *k) // pub/ — постоянная ссылка CDN
		}
	}
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	o, err := h.load(r.Context(), h.DB, p, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.sign(r.Context(), o)
	if p.Role == auth.RoleStore || p.Role == auth.RoleAdmin {
		o.NextStatuses = NextStatuses(o.Status)
		tc := ""
		if o.DeliveryTimeComment != nil {
			tc = *o.DeliveryTimeComment
		}
		o.CopyText = CopyText(o.Number, o.CustomerName, o.CustomerPhone, tc, o.Delivery)
	}
	w.Header().Set("Cache-Control", "no-store") // ПД — не кешировать
	httpx.JSON(w, http.StatusOK, o)
}

// ===================== Смена статуса =====================

type statusReq struct {
	Status Status `json:"status"`
	Reason string `json:"reason"`
	// Ожидаемый текущий статус (защита от гонки двух вкладок). Необязательно.
	From Status `json:"from"`
}

func (h *Handlers) setStatus(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req statusReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Reason = clean(req.Reason, true)
	if trimmedLen(req.Reason) > MaxReason {
		httpx.Fail(w, r, validationErr(map[string]string{"reason": "max"}))
		return
	}
	var out *Detail
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		where, scopeArg := scope(p)
		var cur Status
		err := tx.QueryRow(r.Context(), `SELECT o.status::text FROM orders o WHERE `+where+` AND o.id = $2 FOR UPDATE`, scopeArg, id).Scan(&cur)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if req.From != "" && req.From != cur {
			return &httpx.Error{Status: http.StatusConflict, Code: "status_changed", Message: "order status was changed by someone else",
				Fields: map[string]string{"current": string(cur)}}
		}
		switch err := CheckTransition(p.Role, cur, req.Status, req.Reason); {
		case errors.Is(err, ErrForbidden):
			return httpx.ErrForbidden
		case errors.Is(err, ErrReasonRequired):
			return validationErr(map[string]string{"reason": "required"})
		case errors.Is(err, ErrInvalidTransition):
			return &httpx.Error{Status: http.StatusConflict, Code: "invalid_transition",
				Message: "transition " + string(cur) + " -> " + string(req.Status) + " is not allowed",
				Fields:  map[string]string{"current": string(cur)}}
		case err != nil:
			return err
		}
		reason := req.Reason
		if req.Status != StatusRejected && req.Status != StatusCancelled {
			reason = ""
		}
		if _, err := tx.Exec(r.Context(), `UPDATE orders SET status = $2,
				rejection_reason = COALESCE(NULLIF($3, ''), rejection_reason) WHERE id = $1`, id, string(req.Status), reason); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `INSERT INTO order_status_history (order_id, from_status, to_status, actor_id, actor_role, reason)
				VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))`, id, string(cur), string(req.Status), p.UserID, string(p.Role), reason); err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "order.status", "order", id.String(),
			map[string]any{"from": cur, "to": req.Status}); err != nil {
			return err
		}
		out, err = h.load(r.Context(), tx, p, id)
		return err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.sign(r.Context(), out)
	out.NextStatuses = NextStatuses(out.Status)
	tc := ""
	if out.DeliveryTimeComment != nil {
		tc = *out.DeliveryTimeComment
	}
	out.CopyText = CopyText(out.Number, out.CustomerName, out.CustomerPhone, tc, out.Delivery)
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, out)
}
