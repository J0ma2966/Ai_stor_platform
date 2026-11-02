// Package products — публичный каталог и управление товарами магазина.
// Правило изоляции: любой запрос магазина содержит "store_id = $principal.StoreID".
package products

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
)

type Handlers struct {
	DB    *pgxpool.Pool
	Media media.Storage
}

type Product struct {
	ID          uuid.UUID         `json:"id"`
	StoreID     uuid.UUID         `json:"store_id"`
	StoreName   string            `json:"store_name"`
	CategoryID  uuid.UUID         `json:"category_id"`
	SKU         string            `json:"sku,omitempty"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Brand       string            `json:"brand"`
	Price       int64             `json:"price"`
	InStock     bool              `json:"in_stock"`
	Status      string            `json:"status,omitempty"`
	Reason      *string           `json:"moderation_reason,omitempty"`
	Images      map[string]string `json:"images"`
	CreatedAt   time.Time         `json:"created_at"`
	sortVal     string
	imageKeys   map[string]string
}

func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Get("/categories", h.categories)
	r.Get("/products", h.list)
	r.Get("/products/{id}", h.get)
}

func (h *Handlers) StoreRoutes(r chi.Router) {
	r.Get("/products", h.storeList)
	r.Post("/products", h.storeCreate)
	r.Patch("/products/{id}/status", h.storeSetStatus)
}

func (h *Handlers) categories(w http.ResponseWriter, r *http.Request) {
	type cat struct {
		ID     uuid.UUID `json:"id"`
		Slug   string    `json:"slug"`
		Zone   string    `json:"zone"`
		NameRU string    `json:"name_ru"`
		NameUZ string    `json:"name_uz"`
	}
	rows, err := h.DB.Query(r.Context(), `SELECT id, slug, zone, name_ru, name_uz FROM categories WHERE is_active ORDER BY sort, slug`)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[cat])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	httpx.JSON(w, http.StatusOK, items)
}

var sorts = map[string]struct {
	col  string // выражение сортировки
	desc bool
	cast string
}{
	"new":        {"p.created_at", true, "timestamptz"},
	"price_asc":  {"p.price", false, "bigint"},
	"price_desc": {"p.price", true, "bigint"},
	"popular":    {"p.views_count", true, "bigint"},
}

// list: GET /v1/products?q=&category=&store=&brand=&price_min=&price_max=&in_stock=&sort=&cursor=&limit=
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sortName := q.Get("sort")
	if sortName == "" {
		sortName = "new"
	}
	srt, ok := sorts[sortName]
	if !ok {
		httpx.Fail(w, r, httpx.NewError(400, "bad_sort", "sort must be one of new, price_asc, price_desc, popular"))
		return
	}
	where := []string{"p.status = 'published'", "s.status = 'active'"}
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	for _, f := range []struct{ name, col string }{{"category", "p.category_id"}, {"store", "p.store_id"}} {
		if v := q.Get(f.name); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				httpx.Fail(w, r, httpx.NewError(400, "bad_filter", f.name+" must be uuid"))
				return
			}
			where = append(where, f.col+" = "+arg(id))
		}
	}
	if v := strings.TrimSpace(q.Get("q")); v != "" {
		if len(v) > 100 {
			v = v[:100]
		}
		where = append(where, "lower(p.title || ' ' || p.brand) LIKE "+arg("%"+escapeLike(strings.ToLower(v))+"%"))
	}
	if v := q.Get("brand"); v != "" {
		where = append(where, "lower(p.brand) = "+arg(strings.ToLower(v)))
	}
	for _, f := range []struct{ name, op string }{{"price_min", ">="}, {"price_max", "<="}} {
		if v := q.Get(f.name); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				httpx.Fail(w, r, httpx.NewError(400, "bad_filter", f.name+" must be a non-negative integer"))
				return
			}
			where = append(where, "p.price "+f.op+" "+arg(n))
		}
	}
	if q.Get("in_stock") == "true" {
		where = append(where, "p.in_stock")
	}
	dir, cmp := "ASC", ">"
	if srt.desc {
		dir, cmp = "DESC", "<"
	}
	if c := q.Get("cursor"); c != "" {
		val, id, err := decodeCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_cursor", "invalid cursor"))
			return
		}
		where = append(where, fmt.Sprintf("(%s, p.id) %s (%s::%s, %s)", srt.col, cmp, arg(val), srt.cast, arg(id)))
	}
	limit := httpx.Limit(r, 24, 60)
	sql := fmt.Sprintf(`
		SELECT p.id, p.store_id, s.name, p.category_id, p.title, p.brand, p.price, p.in_stock, p.created_at, (%s)::text,
		       COALESCE((SELECT jsonb_object_agg(angle, key) FROM product_images pi WHERE pi.product_id = p.id), '{}')
		FROM products p JOIN stores s ON s.id = p.store_id
		WHERE %s ORDER BY %s %s, p.id %s LIMIT %d`,
		srt.col, strings.Join(where, " AND "), srt.col, dir, dir, limit+1)
	rows, err := h.DB.Query(r.Context(), sql, args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var items []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.StoreID, &p.StoreName, &p.CategoryID, &p.Title, &p.Brand, &p.Price, &p.InStock, &p.CreatedAt, &p.sortVal, &p.imageKeys); err != nil {
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
		last := items[limit-1]
		next = encodeCursor(last.sortVal, last.ID)
	}
	for i := range items {
		items[i].Images = h.sign(r.Context(), items[i].imageKeys)
	}
	if items == nil {
		items = []Product{}
	}
	httpx.List(w, items, next)
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var p Product
	err = h.DB.QueryRow(r.Context(), `
		UPDATE products p SET views_count = views_count + 1
		FROM stores s
		WHERE p.id = $1 AND s.id = p.store_id AND p.status = 'published' AND s.status = 'active'
		RETURNING p.id, p.store_id, s.name, p.category_id, p.title, p.description, p.brand, p.price, p.in_stock, p.created_at,
		  COALESCE((SELECT jsonb_object_agg(angle, key) FROM product_images pi WHERE pi.product_id = p.id), '{}')`, id).
		Scan(&p.ID, &p.StoreID, &p.StoreName, &p.CategoryID, &p.Title, &p.Description, &p.Brand, &p.Price, &p.InStock, &p.CreatedAt, &p.imageKeys)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p.Images = h.sign(r.Context(), p.imageKeys)
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handlers) sign(ctx context.Context, keys map[string]string) map[string]string {
	out := make(map[string]string, len(keys))
	for angle, k := range keys {
		if u, err := h.Media.PresignGet(ctx, k, time.Hour); err == nil {
			out[angle] = u
		}
	}
	return out
}

// ---------- кабинет магазина ----------

func (h *Handlers) storeList(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	rows, err := h.DB.Query(r.Context(), `
		SELECT p.id, p.store_id, p.category_id, p.sku, p.title, p.brand, p.price, p.in_stock, p.status, p.moderation_reason, p.created_at
		FROM products p WHERE p.store_id = $1 ORDER BY p.created_at DESC LIMIT 500`, p.StoreID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items := []Product{}
	for rows.Next() {
		var it Product
		if err := rows.Scan(&it.ID, &it.StoreID, &it.CategoryID, &it.SKU, &it.Title, &it.Brand, &it.Price, &it.InStock, &it.Status, &it.Reason, &it.CreatedAt); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.List(w, items, "")
}

type createReq struct {
	CategoryID  uuid.UUID         `json:"category_id" validate:"required"`
	SKU         string            `json:"sku" validate:"required,min=1,max=64"`
	Title       string            `json:"title" validate:"required,min=1,max=300"`
	Description string            `json:"description" validate:"required,min=1,max=10000"`
	Brand       string            `json:"brand" validate:"required,min=1,max=120"`
	Price       int64             `json:"price" validate:"required,gt=0"`
	InStock     *bool             `json:"in_stock" validate:"required"`
	Images      map[string]string `json:"images" validate:"required,len=3,dive,keys,oneof=front side back,endkeys,required"`
}

func (h *Handlers) storeCreate(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	var req createReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for angle, key := range req.Images {
		if !media.OwnsKey(p.UserID, media.PurposeProductImage, key) {
			httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "image key not owned", Fields: map[string]string{"images." + angle: "not_owned"}})
			return
		}
	}
	var id uuid.UUID
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO products (store_id, category_id, sku, title, description, brand, price, in_stock)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			p.StoreID, req.CategoryID, req.SKU, req.Title, req.Description, req.Brand, req.Price, *req.InStock).Scan(&id); err != nil {
			return err
		}
		for angle, key := range req.Images {
			if _, err := tx.Exec(r.Context(), `INSERT INTO product_images (product_id, angle, key) VALUES ($1, $2, $3)`, id, angle, key); err != nil {
				return err
			}
		}
		return nil
	})
	if db.IsUniqueViolation(err, "products_store_id_sku_key") {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "sku_taken", "product with this SKU already exists"))
		return
	}
	if err != nil {
		if isFKViolation(err) {
			httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "unknown category", Fields: map[string]string{"category_id": "unknown"}})
			return
		}
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "status": "draft"})
}

type statusReq struct {
	Status string `json:"status" validate:"required,oneof=published draft"`
}

// storeSetStatus: магазин публикует/снимает свой товар, но не может вернуть
// товар, заблокированный администратором.
func (h *Handlers) storeSetStatus(w http.ResponseWriter, r *http.Request) {
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
	tag, err := h.DB.Exec(r.Context(), `
		UPDATE products SET status = $3::product_status,
		       published_at = CASE WHEN $3 = 'published' THEN COALESCE(published_at, now()) ELSE published_at END
		WHERE id = $1 AND store_id = $2 AND status NOT IN ('blocked', 'unpublished')`, id, p.StoreID, req.Status)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		// не различаем «чужой» и «не существует» — чтобы не раскрывать чужие id
		httpx.Fail(w, r, httpx.NewError(http.StatusNotFound, "not_found", "product not found or locked by moderation"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "status": req.Status})
}

func encodeCursor(val string, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(val + "|" + id.String()))
}

func decodeCursor(c string) (string, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return "", uuid.Nil, err
	}
	i := strings.LastIndexByte(string(b), '|')
	if i < 0 {
		return "", uuid.Nil, errors.New("bad cursor")
	}
	id, err := uuid.Parse(string(b[i+1:]))
	return string(b[:i]), id, err
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
