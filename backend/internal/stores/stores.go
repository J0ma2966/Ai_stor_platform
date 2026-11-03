// Package stores — публичные страницы бутиков, профиль магазина в кабинете
// и статистика магазина. Магазин видит только свои данные: каждый запрос
// кабинета содержит store_id = principal.StoreID.
package stores

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
)

type Handlers struct {
	DB         *pgxpool.Pool
	PublicBase string
}

// PublicRoutes: /v1/stores, /v1/stores/{slug}.
func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Get("/stores", h.list)
	r.Get("/stores/{slug}", h.get)
}

// StoreRoutes монтируются под /v1/store (роль store).
func (h *Handlers) StoreRoutes(r chi.Router) {
	r.Get("/profile", h.profile)
	r.Patch("/profile", h.updateProfile)
	r.Get("/stats", h.stats)
}

// Contacts — контакты бутика для клиентов. Только известные поля.
type Contacts struct {
	Telegram  string `json:"telegram,omitempty" validate:"omitempty,max=64"`
	Instagram string `json:"instagram,omitempty" validate:"omitempty,max=64"`
	Website   string `json:"website,omitempty" validate:"omitempty,max=200,url"`
	Email     string `json:"email,omitempty" validate:"omitempty,max=200,email"`
	Hours     string `json:"hours,omitempty" validate:"omitempty,max=200"` // часы работы
}

type Store struct {
	ID           uuid.UUID `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	LogoURL      string    `json:"logo_url,omitempty"`
	CoverURL     string    `json:"cover_url,omitempty"`
	Phone        *string   `json:"phone,omitempty"`
	Contacts     Contacts  `json:"contacts"`
	Address      *string   `json:"address,omitempty"`
	Lat          *float64  `json:"lat,omitempty"`
	Lng          *float64  `json:"lng,omitempty"`
	ProductCount int64     `json:"product_count"`
	Status       string    `json:"status,omitempty"` // только в кабинете
	LogoKey      *string   `json:"logo_key,omitempty"`
	CoverKey     *string   `json:"cover_key,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

const storeCols = `s.id, s.slug, s.name, s.description, s.logo_key, s.cover_key, s.phone, s.contacts, s.address,
	s.lat::float8, s.lng::float8, s.status::text, s.created_at,
	(SELECT count(*) FROM products p WHERE p.store_id = s.id AND p.status = 'published')`

func (h *Handlers) scan(row pgx.Row, s *Store) error {
	var contacts []byte
	if err := row.Scan(&s.ID, &s.Slug, &s.Name, &s.Description, &s.LogoKey, &s.CoverKey, &s.Phone, &contacts, &s.Address,
		&s.Lat, &s.Lng, &s.Status, &s.CreatedAt, &s.ProductCount); err != nil {
		return err
	}
	_ = json.Unmarshal(contacts, &s.Contacts)
	if s.LogoKey != nil {
		s.LogoURL = media.PublicURL(h.PublicBase, *s.LogoKey)
	}
	if s.CoverKey != nil {
		s.CoverURL = media.PublicURL(h.PublicBase, *s.CoverKey)
	}
	return nil
}

// list — активные бутики (их немного, без пагинации; ?q= — поиск по названию).
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if utf8.RuneCountInString(q) > 100 {
		q = string([]rune(q)[:100])
	}
	limit := httpx.Limit(r, 100, 200)
	rows, err := h.DB.Query(r.Context(), storesListSQL, escapeLike(q), limit)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := []Store{}
	for rows.Next() {
		var s Store
		if err := h.scan(rows, &s); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		s.public()
		items = append(items, s)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.List(w, items, "")
}

// public убирает служебные поля из публичного ответа.
func (s *Store) public() {
	s.Status, s.LogoKey, s.CoverKey = "", nil, nil
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var s Store
	err := h.scan(h.DB.QueryRow(r.Context(), storeBySlugSQL, slug), &s)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s.public()
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.JSON(w, http.StatusOK, s)
}

// ---------- кабинет магазина ----------

func (h *Handlers) profile(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	var s Store
	if err := h.scan(h.DB.QueryRow(r.Context(), storeByIDSQL, p.StoreID), &s); err != nil {
		httpx.Fail(w, r, db.NotFound(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, s)
}

type profileReq struct {
	Name        *string   `json:"name" validate:"omitempty,min=1,max=200"`
	Description *string   `json:"description" validate:"omitempty,max=5000"`
	LogoKey     *string   `json:"logo_key" validate:"omitempty,max=200"`
	CoverKey    *string   `json:"cover_key" validate:"omitempty,max=200"`
	Phone       *string   `json:"phone" validate:"omitempty,e164"`
	Contacts    *Contacts `json:"contacts"`
	Address     *string   `json:"address" validate:"omitempty,max=300"`
	Lat         *float64  `json:"lat" validate:"omitempty,min=-90,max=90"`
	Lng         *float64  `json:"lng" validate:"omitempty,min=-180,max=180"`
	ClearLogo   bool      `json:"clear_logo"`
	ClearCover  bool      `json:"clear_cover"`
}

// updateProfile — частичное обновление. Логотип и обложку загружают через
// POST /v1/uploads?purpose=store, ключ должен принадлежать этому магазину.
func (h *Handlers) updateProfile(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	var req profileReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	fields := map[string]string{}
	for name, k := range map[string]*string{"logo_key": req.LogoKey, "cover_key": req.CoverKey} {
		if k != nil && !media.OwnsKey(p.UserID, media.PurposeStoreBranding, *k) {
			fields[name] = "not_owned"
		}
	}
	if (req.Lat == nil) != (req.Lng == nil) {
		fields["lat"] = "pair"
	}
	if req.Contacts != nil {
		if err := httpx.Validate(req.Contacts); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if len(fields) > 0 {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "invalid profile", Fields: fields})
		return
	}
	var contacts any
	if req.Contacts != nil {
		contacts = req.Contacts
	}
	trim := func(s *string) *string {
		if s == nil {
			return nil
		}
		v := strings.TrimSpace(*s)
		return &v
	}
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		_, err := tx.Exec(r.Context(), `UPDATE stores SET
			name        = COALESCE(NULLIF($2, ''), name),
			description = COALESCE($3, description),
			logo_key    = CASE WHEN $10 THEN NULL ELSE COALESCE($4, logo_key) END,
			cover_key   = CASE WHEN $11 THEN NULL ELSE COALESCE($5, cover_key) END,
			phone       = COALESCE($6, phone),
			contacts    = COALESCE($7, contacts),
			address     = COALESCE($8, address),
			lat         = COALESCE($9, lat),
			lng         = COALESCE($12, lng)
			WHERE id = $1`,
			p.StoreID, trim(req.Name), trim(req.Description), req.LogoKey, req.CoverKey, req.Phone, contacts, trim(req.Address),
			req.Lat, req.ClearLogo, req.ClearCover, req.Lng)
		if err != nil {
			return err
		}
		return audit.Log(r.Context(), tx, "store.profile", "store", p.StoreID.String(), nil)
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.profile(w, r)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Запросы собираются из констант на этапе компиляции.
const (
	storesListSQL = `SELECT ` + storeCols + ` FROM stores s
		WHERE s.status = 'active' AND ($1 = '' OR lower(s.name) LIKE '%' || $1 || '%')
		ORDER BY s.name LIMIT $2`
	storeBySlugSQL = `SELECT ` + storeCols + ` FROM stores s WHERE s.slug = $1 AND s.status = 'active'`
	storeByIDSQL   = `SELECT ` + storeCols + ` FROM stores s WHERE s.id = $1`
)
