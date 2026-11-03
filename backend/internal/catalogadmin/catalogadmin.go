// Package catalogadmin — модерация товаров, категории и настройки платформы
// (панель суперадминистратора, ТЗ 2.1, 5.5, 11.3). Каждое действие пишется
// в audit_log в той же транзакции.
package catalogadmin

import (
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/notify"
	"github.com/boutique-market/backend/internal/platform/db"
)

type Handlers struct {
	DB         *pgxpool.Pool
	PublicBase string
}

// AdminRoutes монтируются под /v1/admin (роль admin).
func (h *Handlers) AdminRoutes(r chi.Router) {
	r.Get("/products", h.products)
	r.Post("/products/{id}/status", h.moderate)
	r.Get("/categories", h.categories)
	r.Post("/categories", h.createCategory)
	r.Patch("/categories/{id}", h.updateCategory)
	r.Get("/settings", h.settings)
	r.Put("/settings", h.putSettings)
}

// PublicRoutes: /v1/platform — публичные настройки (стоимость генерации, контакты поддержки).
func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Get("/platform", h.publicSettings)
}

// ---------- модерация товаров ----------

type AdminProduct struct {
	ID        uuid.UUID `json:"id"`
	StoreID   uuid.UUID `json:"store_id"`
	StoreName string    `json:"store_name"`
	SKU       string    `json:"sku"`
	Title     string    `json:"title"`
	Brand     string    `json:"brand"`
	Price     int64     `json:"price"`
	InStock   bool      `json:"in_stock"`
	Status    string    `json:"status"`
	Reason    *string   `json:"moderation_reason"`
	ImageURL  string    `json:"image_url,omitempty"`
	Views     int64     `json:"views"`
	CreatedAt time.Time `json:"created_at"`
}

// products: GET /v1/admin/products?q=&store=&status=&cursor=&limit=
func (h *Handlers) products(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	where := []string{"true"}
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	if v := q.Get("store"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "store must be uuid"))
			return
		}
		where = append(where, "p.store_id = "+arg(id))
	}
	if v := q.Get("status"); v != "" {
		switch v {
		case "draft", "published", "unpublished", "blocked":
			where = append(where, "p.status = "+arg(v)+"::product_status")
		default:
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "unknown status"))
			return
		}
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		if len(v) > 100 {
			v = v[:100]
		}
		like := "%" + escapeLike(strings.ToLower(v)) + "%"
		where = append(where, "(lower(p.title || ' ' || p.brand) LIKE "+arg(like)+" OR lower(p.sku) = "+arg(strings.ToLower(v))+")")
	}
	if c := q.Get("cursor"); c != "" {
		t, id, err := decodeCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_cursor", "invalid cursor"))
			return
		}
		where = append(where, "(p.created_at, p.id) < ("+arg(t)+", "+arg(id)+")")
	}
	limit := httpx.Limit(r, 50, 200)
	// nosemgrep: go.lang.security.injection.tainted-sql-string.tainted-sql-string -- только константы и плейсхолдеры $N
	sql := `SELECT p.id, p.store_id, s.name, p.sku, p.title, p.brand, p.price, p.in_stock, p.status::text, p.moderation_reason,
			(SELECT key FROM product_images pi WHERE pi.product_id = p.id AND pi.angle = 'front'), p.views_count, p.created_at
		FROM products p JOIN stores s ON s.id = p.store_id
		WHERE ` + strings.Join(where, " AND ") + ` ORDER BY p.created_at DESC, p.id DESC LIMIT ` + strconv.Itoa(limit+1)
	rows, err := h.DB.Query(r.Context(), sql, args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := []AdminProduct{}
	for rows.Next() {
		var p AdminProduct
		var key *string
		if err := rows.Scan(&p.ID, &p.StoreID, &p.StoreName, &p.SKU, &p.Title, &p.Brand, &p.Price, &p.InStock, &p.Status, &p.Reason,
			&key, &p.Views, &p.CreatedAt); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if key != nil {
			p.ImageURL = media.PublicURL(h.PublicBase, *key)
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
		last := items[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	httpx.List(w, items, next)
}

type moderateReq struct {
	// unpublished — снять с публикации (магазин исправит и опубликует снова),
	// blocked — заблокировать (вернуть может только администратор),
	// draft — снять ограничение, товар возвращается магазину черновиком.
	Status string `json:"status" validate:"required,oneof=unpublished blocked draft"`
	Reason string `json:"reason" validate:"max=500"`
}

func (h *Handlers) moderate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req moderateReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Status != "draft" && len([]rune(req.Reason)) < 3 {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "reason is required", Fields: map[string]string{"reason": "required"}})
		return
	}
	var storeID uuid.UUID
	var title string
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		err := tx.QueryRow(r.Context(), `UPDATE products SET status = $2::product_status, moderation_reason = NULLIF($3, '')
			WHERE id = $1 RETURNING store_id, title`, id, req.Status, req.Reason).Scan(&storeID, &title)
		if err != nil {
			return db.NotFound(err)
		}
		if err := audit.Log(r.Context(), tx, "product.moderate", "product", id.String(), map[string]any{"status": req.Status, "reason": req.Reason}); err != nil {
			return err
		}
		return notify.SendToStore(r.Context(), tx, storeID, notify.ProductModerated,
			map[string]any{"product_id": id, "title": title, "status": req.Status, "reason": req.Reason})
	})
	if errors.Is(err, db.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "status": req.Status})
}

// ---------- категории ----------

type Category struct {
	ID       uuid.UUID  `json:"id"`
	Slug     string     `json:"slug"`
	ParentID *uuid.UUID `json:"parent_id"`
	Zone     string     `json:"zone"`
	NameRU   string     `json:"name_ru"`
	NameUZ   string     `json:"name_uz"`
	Sort     int        `json:"sort"`
	IsActive bool       `json:"is_active"`
	Products int64      `json:"products"`
}

func (h *Handlers) categories(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `SELECT c.id, c.slug, c.parent_id, c.zone::text, c.name_ru, c.name_uz, c.sort, c.is_active,
		(SELECT count(*) FROM products p WHERE p.category_id = c.id)
		FROM categories c ORDER BY c.sort, c.slug`)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Category])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.List(w, items, "")
}

type categoryReq struct {
	Slug     *string    `json:"slug" validate:"omitempty,min=2,max=63"`
	ParentID *uuid.UUID `json:"parent_id"`
	Zone     *string    `json:"zone" validate:"omitempty,oneof=headwear top bottom shoes"`
	NameRU   *string    `json:"name_ru" validate:"omitempty,min=1,max=100"`
	NameUZ   *string    `json:"name_uz" validate:"omitempty,min=1,max=100"`
	Sort     *int       `json:"sort" validate:"omitempty,min=0,max=10000"`
	IsActive *bool      `json:"is_active"`
}

func (h *Handlers) createCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	missing := map[string]string{}
	if req.Slug == nil {
		missing["slug"] = "required"
	}
	if req.Zone == nil {
		missing["zone"] = "required"
	}
	if req.NameRU == nil {
		missing["name_ru"] = "required"
	}
	if req.NameUZ == nil {
		missing["name_uz"] = "required"
	}
	if len(missing) > 0 {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "required fields", Fields: missing})
		return
	}
	if !validSlug(*req.Slug) {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "slug: a-z, 0-9, -", Fields: map[string]string{"slug": "format"}})
		return
	}
	sort, active := 100, true
	if req.Sort != nil {
		sort = *req.Sort
	}
	if req.IsActive != nil {
		active = *req.IsActive
	}
	var id uuid.UUID
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `INSERT INTO categories (slug, parent_id, zone, name_ru, name_uz, sort, is_active)
			VALUES ($1, $2, $3::category_zone, $4, $5, $6, $7) RETURNING id`,
			*req.Slug, req.ParentID, *req.Zone, strings.TrimSpace(*req.NameRU), strings.TrimSpace(*req.NameUZ), sort, active).Scan(&id); err != nil {
			return err
		}
		return audit.Log(r.Context(), tx, "category.create", "category", id.String(), map[string]any{"slug": *req.Slug})
	})
	if h.categoryErr(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (h *Handlers) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req categoryReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.Slug != nil && !validSlug(*req.Slug) {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "slug: a-z, 0-9, -", Fields: map[string]string{"slug": "format"}})
		return
	}
	if req.ParentID != nil && *req.ParentID == id {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "category cannot be its own parent", Fields: map[string]string{"parent_id": "self"}})
		return
	}
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `UPDATE categories SET
			slug = COALESCE($2, slug), parent_id = COALESCE($3, parent_id), zone = COALESCE($4::category_zone, zone),
			name_ru = COALESCE($5, name_ru), name_uz = COALESCE($6, name_uz), sort = COALESCE($7, sort),
			is_active = COALESCE($8, is_active)
			WHERE id = $1`, id, req.Slug, req.ParentID, req.Zone, req.NameRU, req.NameUZ, req.Sort, req.IsActive)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return db.ErrNotFound
		}
		return audit.Log(r.Context(), tx, "category.update", "category", id.String(), nil)
	})
	if h.categoryErr(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id})
}

func (h *Handlers) categoryErr(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, db.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case db.IsUniqueViolation(err, "categories_slug_key"):
		httpx.Fail(w, r, httpx.NewError(409, "slug_taken", "slug already used"))
	case isFK(err):
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "unknown parent", Fields: map[string]string{"parent_id": "unknown"}})
	default:
		httpx.Fail(w, r, err)
	}
	return true
}

func validSlug(s string) bool {
	if len(s) < 2 || len(s) > 63 || s[0] == '-' {
		return false
	}
	return strings.Trim(s, "abcdefghijklmnopqrstuvwxyz0123456789-") == ""
}

// ---------- настройки платформы ----------

// Settings — известные ключи platform_settings. Неизвестные ключи не принимаются.
type Settings struct {
	GenerationCost  *int64  `json:"generation_cost" validate:"omitempty,min=0,max=100000"`
	WelcomeBonus    *int64  `json:"welcome_bonus" validate:"omitempty,min=0,max=100000"`
	SupportPhone    *string `json:"support_phone" validate:"omitempty,max=30"`
	SupportEmail    *string `json:"support_email" validate:"omitempty,max=200"`
	SupportTelegram *string `json:"support_telegram" validate:"omitempty,max=64"`
}

var settingKeys = []string{"generation_cost", "welcome_bonus", "support_phone", "support_email", "support_telegram"}

func (h *Handlers) loadSettings(r *http.Request) (map[string]json.RawMessage, error) {
	rows, err := h.DB.Query(r.Context(), `SELECT key, value FROM platform_settings WHERE key = ANY($1)`, settingKeys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var v json.RawMessage
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (h *Handlers) settings(w http.ResponseWriter, r *http.Request) {
	m, err := h.loadSettings(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, m)
}

func (h *Handlers) putSettings(w http.ResponseWriter, r *http.Request) {
	var req Settings
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if req.SupportEmail != nil && *req.SupportEmail != "" && !strings.Contains(*req.SupportEmail, "@") {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "invalid email", Fields: map[string]string{"support_email": "email"}})
		return
	}
	if req.SupportPhone != nil && *req.SupportPhone != "" && !httpx.IsE164(*req.SupportPhone) {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "invalid phone", Fields: map[string]string{"support_phone": "e164"}})
		return
	}
	vals := map[string]any{}
	if req.GenerationCost != nil {
		vals["generation_cost"] = *req.GenerationCost
	}
	if req.WelcomeBonus != nil {
		vals["welcome_bonus"] = *req.WelcomeBonus
	}
	for k, v := range map[string]*string{"support_phone": req.SupportPhone, "support_email": req.SupportEmail, "support_telegram": req.SupportTelegram} {
		if v != nil {
			vals[k] = strings.TrimSpace(*v)
		}
	}
	actor := auth.MustFrom(r.Context()).UserID
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		for k, v := range vals {
			b, _ := json.Marshal(v)
			if _, err := tx.Exec(r.Context(), `INSERT INTO platform_settings (key, value, updated_by, updated_at) VALUES ($1, $2, $3, now())
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`, k, b, actor); err != nil {
				return err
			}
		}
		return audit.Log(r.Context(), tx, "settings.update", "platform_settings", "", vals)
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.settings(w, r)
}

func (h *Handlers) publicSettings(w http.ResponseWriter, r *http.Request) {
	m, err := h.loadSettings(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	delete(m, "welcome_bonus")
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.JSON(w, http.StatusOK, m)
}

// ---------- утилиты ----------

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	ts, ids, ok := strings.Cut(string(b), "|")
	if !ok {
		return time.Time{}, uuid.Nil, errors.New("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(ids)
	return t, id, err
}

func isFK(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
