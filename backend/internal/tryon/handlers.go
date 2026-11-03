package tryon

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/ratelimit"
	"github.com/boutique-market/backend/internal/tokens"
)

type Handlers struct {
	DB         *pgxpool.Pool
	Store      media.Storage
	Limiter    *ratelimit.Limiter
	Redis      *redis.Client
	PublicBase string
	Retention  Retention
	MaxActive  int    // активных генераций на клиента
	Provider   string // имя провайдера (для фронта: какие зоны поддерживаются)
	// Fitting — буфер счётчика «В примерочную» в Redis (nil — без счётчика).
	Fitting *FittingCounter
}

const (
	presignTTL = 15 * time.Minute
	idemScope  = "tryon_generation"
)

var (
	ruleCreate     = ratelimit.Rule{Name: "tryon_create", Limit: 30, Window: time.Hour}
	ruleFittingAdd = ratelimit.Rule{Name: "fitting_add_ip", Limit: 120, Window: time.Minute}
)

// ClientRoutes монтируются за RequireRole(client): /v1/tryon.
func (h *Handlers) ClientRoutes(r chi.Router) {
	r.Get("/quote", h.quote)
	r.Post("/consent", h.consent)
	r.Delete("/consent", h.revokeConsent)
	r.Get("/photos", h.listPhotos)
	r.Put("/photos/{angle}", h.savePhoto)
	r.Delete("/photos/{angle}", h.deletePhoto)
	r.Post("/generations", h.create)
	r.Get("/generations", h.list)
	r.Get("/generations/{id}", h.get)
	r.Post("/generations/{id}/save", h.save(true))
	r.Delete("/generations/{id}/save", h.save(false))
	r.Delete("/generations/{id}", h.delete)
}

// PublicRoutes: POST /v1/fitting/adds — счётчик «В примерочную» для статистики магазина.
func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Post("/fitting/adds", h.fittingAdd)
}

// AdminRoutes: /v1/admin/tryon — история генераций, статусы и ошибки (без изображений).
func (h *Handlers) AdminRoutes(r chi.Router) {
	r.Get("/generations", h.adminList)
	r.Get("/stats", h.adminStats)
}

// ---------- стоимость и согласие ----------

// SupportedZones — какие зоны умеет провайдер (фронт предупреждает о прочих).
func SupportedZones(provider string) []string {
	if provider == "fashn" {
		return []string{ZoneTop, ZoneBottom}
	}
	return []string{ZoneHeadwear, ZoneTop, ZoneBottom, ZoneShoes}
}

func (h *Handlers) quote(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	cost, err := tokens.Setting(r.Context(), h.DB, "generation_cost")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var bal int64
	var gender *string
	var consent, pd *time.Time
	var active int
	if err := h.DB.QueryRow(r.Context(), `SELECT token_balance, gender::text, tryon_consent_at, pd_consent_at,
		(SELECT count(*) FROM generations WHERE user_id = $1 AND status IN ('queued','running'))
		FROM users WHERE id = $1`, uid).Scan(&bal, &gender, &consent, &pd, &active); err != nil {
		httpx.Fail(w, r, db.NotFound(err))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"cost": cost, "balance": bal, "enough": bal >= cost,
		"consent": consent != nil, "registered": gender != nil && pd != nil, "gender": gender,
		"active": active, "max_active": h.MaxActive,
		"supported_zones": SupportedZones(h.Provider),
		"retention": map[string]any{
			"photo_hours": int(h.Retention.Photo.Hours()), "saved_photo_days": int(h.Retention.SavedPhoto.Hours() / 24),
			"result_days": int(h.Retention.Result.Hours() / 24), "saved_look_days": int(h.Retention.SavedLook.Hours() / 24),
		},
	})
}

func (h *Handlers) consent(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	if _, err := h.DB.Exec(r.Context(), `UPDATE users SET tryon_consent_at = COALESCE(tryon_consent_at, now()) WHERE id = $1`, uid); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// revokeConsent — отзыв согласия: удаляем всю биометрию клиента.
func (h *Handlers) revokeConsent(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	err := db.Tx(r.Context(), h.DB, func(t pgx.Tx) error {
		var active int
		if err := t.QueryRow(r.Context(), `SELECT count(*) FROM generations WHERE user_id = $1 AND status IN ('queued','running')`, uid).Scan(&active); err != nil {
			return err
		}
		if active > 0 {
			return errInProgress
		}
		if _, err := t.Exec(r.Context(), `UPDATE users SET tryon_consent_at = NULL WHERE id = $1`, uid); err != nil {
			return err
		}
		return PurgeUser(r.Context(), t, h.Store, uid)
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

var errInProgress = httpx.NewError(http.StatusConflict, "generation_in_progress", "wait until the generation finishes")

// ---------- сохранённые фото ----------

type photoView struct {
	ID         uuid.UUID `json:"id"`
	Angle      string    `json:"angle"`
	Key        string    `json:"key"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func validAngle(a string) bool { return a == "front" || a == "side" || a == "back" }

func (h *Handlers) listPhotos(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	rows, err := h.DB.Query(r.Context(), `SELECT id, angle::text, key, created_at, last_used_at FROM body_photos
		WHERE user_id = $1 ORDER BY array_position(ARRAY['front','side','back'], angle::text)`, uid)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []photoView{}
	for rows.Next() {
		var p photoView
		if err := rows.Scan(&p.ID, &p.Angle, &p.Key, &p.CreatedAt, &p.LastUsedAt); err != nil {
			rows.Close()
			httpx.Fail(w, r, err)
			return
		}
		p.ExpiresAt = p.LastUsedAt.Add(h.Retention.SavedPhoto)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for i := range out {
		if out[i].URL, err = h.Store.PresignGet(r.Context(), out[i].Key, presignTTL); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	httpx.List(w, out, "")
}

type savePhotoReq struct {
	Key string `json:"key" validate:"required,max=200"`
}

// savePhoto: PUT /v1/tryon/photos/{angle} {"key"} — сохранить или заменить фото ракурса.
// Старый файл удалит janitor, когда он перестанет быть нужен генерациям.
func (h *Handlers) savePhoto(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	angle := chi.URLParam(r, "angle")
	var req savePhotoReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !validAngle(angle) || !media.OwnsKey(uid, media.PurposeBodyPhoto, req.Key) {
		httpx.Fail(w, r, validation(map[string]string{"key": "invalid"}))
		return
	}
	if err := h.requireConsent(r.Context(), uid); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := upsertPhoto(r.Context(), h.DB, uid, angle, req.Key); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.listPhotos(w, r)
}

type execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func upsertPhoto(ctx context.Context, q execer, uid uuid.UUID, angle, key string) error {
	_, err := q.Exec(ctx, `INSERT INTO body_photos (user_id, angle, key) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, angle) DO UPDATE SET key = EXCLUDED.key, created_at = now(), last_used_at = now()
		WHERE body_photos.key <> EXCLUDED.key`, uid, angle, key)
	if db.IsUniqueViolation(err, "body_photos_key_key") {
		return validation(map[string]string{"key": "already_saved"})
	}
	return err
}

func (h *Handlers) deletePhoto(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	var key string
	err := h.DB.QueryRow(r.Context(), `DELETE FROM body_photos WHERE user_id = $1 AND angle::text = $2 RETURNING key`,
		uid, chi.URLParam(r, "angle")).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Фото, нужное идущей/недавней генерации, удалит janitor по её сроку.
	if _, err := deleteUnreferenced(r.Context(), h.DB, h.Store, []string{key}, time.Now().Add(-h.Retention.Photo)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ---------- создание генерации ----------

type photosReq struct {
	Front string `json:"front" validate:"required,max=200"`
	Side  string `json:"side" validate:"required,max=200"`
	Back  string `json:"back" validate:"required,max=200"`
}

type bodyReq struct {
	HeightCM *int16             `json:"height_cm" validate:"omitempty,min=50,max=260"`
	WeightKG *float64           `json:"weight_kg" validate:"omitempty,min=20,max=400"`
	Params   map[string]float64 `json:"params" validate:"omitempty,max=20,dive,keys,min=1,max=32,alphanumunicode|containsany=_,endkeys,min=1,max=400"`
}

type createReq struct {
	ProductIDs []uuid.UUID `json:"product_ids" validate:"required,min=1,max=4,unique"`
	Photos     photosReq   `json:"photos"`
	SavePhotos bool        `json:"save_photos"`
	Body       *bodyReq    `json:"body"`
}

func validation(fields map[string]string) error {
	return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "request validation failed", Fields: fields}
}

func (h *Handlers) requireConsent(ctx context.Context, uid uuid.UUID) error {
	var consent *time.Time
	if err := h.DB.QueryRow(ctx, `SELECT tryon_consent_at FROM users WHERE id = $1`, uid).Scan(&consent); err != nil {
		return db.NotFound(err)
	}
	if consent == nil {
		return httpx.NewError(http.StatusConflict, "tryon_consent_required", "consent to photo processing is required")
	}
	return nil
}

// create: POST /v1/tryon/generations (заголовок Idempotency-Key обязателен — защита от
// двойного списания при повторной отправке). Списание токенов и постановка в очередь —
// одна транзакция. Ответ 202 + генерация в статусе queued.
func (h *Handlers) create(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idemKey) < 8 || len(idemKey) > 128 {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key header (8..128 chars) is required"))
		return
	}
	if h.Limiter != nil && !h.Limiter.Check(w, r, ruleCreate, p.UserID.String()) {
		return
	}
	var req createReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	keys := []string{req.Photos.Front, req.Photos.Side, req.Photos.Back}
	bad := map[string]string{}
	seen := map[string]bool{}
	for i, k := range keys {
		if !media.OwnsKey(p.UserID, media.PurposeBodyPhoto, k) || seen[k] {
			bad["photos."+Angles[i]] = "invalid"
		}
		seen[k] = true
	}
	if len(bad) > 0 {
		httpx.Fail(w, r, validation(bad))
		return
	}
	canon, _ := json.Marshal(req)
	reqHash := sha256.Sum256(canon)

	var status int
	var body []byte
	err := db.Tx(r.Context(), h.DB, func(t pgx.Tx) error {
		tag, err := t.Exec(r.Context(), `INSERT INTO idempotency_keys (user_id, scope, key, request_hash) VALUES ($1, $2, $3, $4)
			ON CONFLICT DO NOTHING`, p.UserID, idemScope, idemKey, reqHash[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			var prevHash []byte
			var prevStatus *int
			if err := t.QueryRow(r.Context(), `SELECT request_hash, status_code, response FROM idempotency_keys
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
		id, err := h.enqueue(r.Context(), t, p.UserID, &req, keys)
		if err != nil {
			return err
		}
		v, err := h.load(r.Context(), t, p.UserID, id)
		if err != nil {
			return err
		}
		body, _ = json.Marshal(map[string]any{"data": v})
		status = http.StatusAccepted
		_, err = t.Exec(r.Context(), `UPDATE idempotency_keys SET status_code = $4, response = $5
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

func (h *Handlers) enqueue(ctx context.Context, t pgx.Tx, uid uuid.UUID, req *createReq, keys []string) (uuid.UUID, error) {
	// Блокировка строки клиента сериализует его параллельные запуски (лимит активных).
	var gender *string
	var consent, pd *time.Time
	var height *int16
	var weight *float64
	var params []byte
	if err := t.QueryRow(ctx, `SELECT gender::text, tryon_consent_at, pd_consent_at, height_cm, weight_kg::float8, body_params
		FROM users WHERE id = $1 FOR UPDATE`, uid).Scan(&gender, &consent, &pd, &height, &weight, &params); err != nil {
		return uuid.Nil, db.NotFound(err)
	}
	if gender == nil || pd == nil {
		return uuid.Nil, httpx.NewError(http.StatusConflict, "registration_incomplete", "choose gender and accept the personal data policy first")
	}
	if consent == nil {
		return uuid.Nil, httpx.NewError(http.StatusConflict, "tryon_consent_required", "consent to photo processing is required")
	}
	var active int
	if err := t.QueryRow(ctx, `SELECT count(*) FROM generations WHERE user_id = $1 AND status IN ('queued','running')`, uid).Scan(&active); err != nil {
		return uuid.Nil, err
	}
	if active >= max(h.MaxActive, 1) {
		return uuid.Nil, httpx.NewError(http.StatusTooManyRequests, "too_many_active_generations", "wait until the current generation finishes")
	}

	// Товары: опубликованы, магазин активен, есть фото, по одному на зону манекена.
	rows, err := t.Query(ctx, `SELECT p.id, c.zone::text,
		       p.status = 'published' AND s.status = 'active'
		       AND EXISTS (SELECT 1 FROM product_images pi WHERE pi.product_id = p.id AND pi.angle = 'front')
		FROM products p JOIN categories c ON c.id = p.category_id JOIN stores s ON s.id = p.store_id
		WHERE p.id = ANY($1)`, req.ProductIDs)
	if err != nil {
		return uuid.Nil, err
	}
	type prod struct {
		ID   uuid.UUID
		Zone string
		OK   bool
	}
	prods, err := pgx.CollectRows(rows, pgx.RowToStructByPos[prod])
	if err != nil {
		return uuid.Nil, err
	}
	bad := map[string]string{}
	byID := map[uuid.UUID]prod{}
	for _, pr := range prods {
		byID[pr.ID] = pr
	}
	zones := map[string]bool{}
	for i, id := range req.ProductIDs {
		pr, ok := byID[id]
		switch {
		case !ok || !pr.OK:
			bad[fmt.Sprintf("product_ids[%d]", i)] = "unavailable"
		case zones[pr.Zone]:
			bad[fmt.Sprintf("product_ids[%d]", i)] = "zone_taken"
		}
		zones[pr.Zone] = true
	}
	if len(bad) > 0 {
		return uuid.Nil, validation(bad)
	}

	// Параметры тела: из запроса, иначе из профиля.
	b := Body{HeightCM: height, WeightKG: weight}
	_ = json.Unmarshal(params, &b.Params)
	if req.Body != nil {
		b = Body{HeightCM: req.Body.HeightCM, WeightKG: req.Body.WeightKG, Params: req.Body.Params}
	}
	bodyJSON, _ := json.Marshal(b)

	cost, err := tokens.Setting(ctx, t, "generation_cost")
	if err != nil {
		return uuid.Nil, err
	}
	id := uuid.New()
	if _, err := t.Exec(ctx, `INSERT INTO generations (id, user_id, status, cost, product_ids, photo_keys, body, gender)
		VALUES ($1, $2, 'queued', $3, $4, $5, $6, $7::gender)`, id, uid, cost, req.ProductIDs, keys, bodyJSON, gender); err != nil {
		return uuid.Nil, err
	}
	if cost > 0 {
		bal, err := tokens.Apply(ctx, t, tokens.Entry{UserID: uid, Op: tokens.OpGenerationCharge, Amount: -cost,
			ReferenceType: "generation", ReferenceID: &id})
		if errors.Is(err, tokens.ErrInsufficient) {
			return uuid.Nil, &httpx.Error{Status: http.StatusPaymentRequired, Code: "insufficient_tokens", Message: "not enough tokens",
				Fields: map[string]string{"cost": strconv.FormatInt(cost, 10), "balance": strconv.FormatInt(bal, 10)}}
		}
		if err != nil {
			return uuid.Nil, err
		}
	}
	if req.SavePhotos {
		for i, k := range keys {
			if err := upsertPhoto(ctx, t, uid, Angles[i], k); err != nil {
				return uuid.Nil, err
			}
		}
	}
	// сохранённые фото «использованы» — продлеваем срок хранения
	if _, err := t.Exec(ctx, `UPDATE body_photos SET last_used_at = now() WHERE user_id = $1 AND key = ANY($2)`, uid, keys); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// ---------- просмотр ----------

type productView struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Brand     string    `json:"brand"`
	Price     int64     `json:"price"`
	Zone      string    `json:"zone"`
	ImageURL  string    `json:"image_url"`
	StoreName string    `json:"store_name"`
	StoreSlug string    `json:"store_slug"`
	Available bool      `json:"available"`
}

type generationView struct {
	ID         uuid.UUID     `json:"id"`
	Status     string        `json:"status"`
	Cost       int64         `json:"cost"`
	ErrorCode  *string       `json:"error_code"`
	Refunded   bool          `json:"refunded"`
	Saved      bool          `json:"saved"`
	ResultURL  *string       `json:"result_url"`
	Photos     []photoRef    `json:"photos"`
	Products   []productView `json:"products"`
	Body       Body          `json:"body"`
	CreatedAt  time.Time     `json:"created_at"`
	StartedAt  *time.Time    `json:"started_at"`
	FinishedAt *time.Time    `json:"finished_at"`
	ExpiresAt  *time.Time    `json:"result_expires_at"`

	resultKey   *string
	photoKeys   []string
	productIDs  []uuid.UUID
	photoPurged bool
	savedAt     *time.Time
}

type photoRef struct {
	Angle string `json:"angle"`
	Key   string `json:"key"`
	URL   string `json:"url"`
}

const genCols = `id, status::text, cost, error_code, refunded, result_key, photo_keys, product_ids, body,
	created_at, started_at, finished_at, saved_at, photos_purged_at IS NOT NULL`

// SQL собирается из констант на этапе компиляции (без пользовательского ввода).
const (
	sqlGetGen  = `SELECT ` + genCols + ` FROM generations WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`
	sqlListGen = `SELECT ` + genCols + ` FROM generations
		WHERE user_id = $1 AND deleted_at IS NULL AND (created_at, id) < ($2, $3) AND (NOT $4 OR saved_at IS NOT NULL)
		ORDER BY created_at DESC, id DESC LIMIT $5`
)

type rowScanner interface{ Scan(...any) error }

func scanGen(s rowScanner) (*generationView, error) {
	var v generationView
	var body []byte
	if err := s.Scan(&v.ID, &v.Status, &v.Cost, &v.ErrorCode, &v.Refunded, &v.resultKey, &v.photoKeys, &v.productIDs,
		&body, &v.CreatedAt, &v.StartedAt, &v.FinishedAt, &v.savedAt, &v.photoPurged); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(body, &v.Body)
	v.Saved = v.savedAt != nil
	return &v, nil
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (h *Handlers) load(ctx context.Context, q queryer, uid, id uuid.UUID) (*generationView, error) {
	v, err := scanGen(q.QueryRow(ctx, sqlGetGen, id, uid))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := h.enrich(ctx, q, []*generationView{v}, true); err != nil {
		return nil, err
	}
	return v, nil
}

// enrich: товары (снимок не храним — карточка актуальна), подписанные ссылки.
func (h *Handlers) enrich(ctx context.Context, q queryer, gens []*generationView, withPhotos bool) error {
	var ids []uuid.UUID
	for _, g := range gens {
		ids = append(ids, g.productIDs...)
	}
	rows, err := q.Query(ctx, `SELECT p.id, p.title, p.brand, p.price, c.zone::text,
		       COALESCE((SELECT key FROM product_images pi WHERE pi.product_id = p.id AND pi.angle = 'front'), ''),
		       s.name, s.slug, p.status = 'published' AND s.status = 'active' AND p.in_stock
		FROM products p JOIN categories c ON c.id = p.category_id JOIN stores s ON s.id = p.store_id
		WHERE p.id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	byID := map[uuid.UUID]productView{}
	for rows.Next() {
		var pv productView
		var key string
		if err := rows.Scan(&pv.ID, &pv.Title, &pv.Brand, &pv.Price, &pv.Zone, &key, &pv.StoreName, &pv.StoreSlug, &pv.Available); err != nil {
			rows.Close()
			return err
		}
		pv.ImageURL = media.PublicURL(h.PublicBase, key)
		byID[pv.ID] = pv
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, g := range gens {
		g.Products = []productView{}
		for _, id := range g.productIDs {
			if pv, ok := byID[id]; ok {
				g.Products = append(g.Products, pv)
			}
		}
		if g.resultKey != nil {
			u, err := h.Store.PresignGet(ctx, *g.resultKey, presignTTL)
			if err != nil {
				return err
			}
			g.ResultURL = &u
			var exp time.Time
			if g.savedAt != nil {
				exp = g.savedAt.Add(h.Retention.SavedLook)
			} else if g.FinishedAt != nil {
				exp = g.FinishedAt.Add(h.Retention.Result)
			}
			if !exp.IsZero() {
				g.ExpiresAt = &exp
			}
		}
		g.Photos = []photoRef{}
		if withPhotos && !g.photoPurged {
			for i, k := range g.photoKeys {
				u, err := h.Store.PresignGet(ctx, k, presignTTL)
				if err != nil {
					return err
				}
				g.Photos = append(g.Photos, photoRef{Angle: Angles[min(i, len(Angles)-1)], Key: k, URL: u})
			}
		}
	}
	return nil
}

func (h *Handlers) get(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	v, err := h.load(r.Context(), h.DB, uid, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if v.Status == "queued" || v.Status == "running" {
		w.Header().Set("Retry-After", "3") // подсказка фронту для опроса
	}
	httpx.JSON(w, http.StatusOK, v)
}

// cursor = <created_at unix nano>_<id>
func parseCursor(s string) (time.Time, uuid.UUID, bool) {
	a, b, ok := strings.Cut(s, "_")
	if !ok {
		return time.Time{}, uuid.Nil, false
	}
	n, err1 := strconv.ParseInt(a, 10, 64)
	id, err2 := uuid.Parse(b)
	return time.Unix(0, n), id, err1 == nil && err2 == nil
}

func cursorOf(t time.Time, id uuid.UUID) string {
	return strconv.FormatInt(t.UnixNano(), 10) + "_" + id.String()
}

// list: GET /v1/tryon/generations?saved=true&limit=&cursor= — история примерок / сохранённые образы.
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	limit := httpx.Limit(r, 20, 50)
	ts, cid := time.Now().Add(time.Hour), uuid.Max
	if c := r.URL.Query().Get("cursor"); c != "" {
		var ok bool
		if ts, cid, ok = parseCursor(c); !ok {
			httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_cursor", "invalid cursor"))
			return
		}
	}
	savedOnly := r.URL.Query().Get("saved") == "true"
	rows, err := h.DB.Query(r.Context(), sqlListGen, uid, ts, cid, savedOnly, limit+1)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var gens []*generationView
	for rows.Next() {
		g, err := scanGen(rows)
		if err != nil {
			rows.Close()
			httpx.Fail(w, r, err)
			return
		}
		gens = append(gens, g)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(gens) > limit {
		gens = gens[:limit]
		next = cursorOf(gens[limit-1].CreatedAt, gens[limit-1].ID)
	}
	if err := h.enrich(r.Context(), h.DB, gens, false); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if gens == nil {
		gens = []*generationView{}
	}
	httpx.List(w, gens, next)
}

// save: «Сохранить образ» — результат хранится дольше (SavedLook).
func (h *Handlers) save(on bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := auth.MustFrom(r.Context()).UserID
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		tag, err := h.DB.Exec(r.Context(), `UPDATE generations SET saved_at = CASE WHEN $3 THEN COALESCE(saved_at, now()) END
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL AND status = 'succeeded' AND result_key IS NOT NULL`, id, uid, on)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if tag.RowsAffected() == 0 {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		v, err := h.load(r.Context(), h.DB, uid, id)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, v)
	}
}

// delete: удалить примерку из истории вместе с результатом и несохранёнными фото.
func (h *Handlers) delete(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	err = db.Tx(r.Context(), h.DB, func(t pgx.Tx) error {
		var status string
		var result *string
		var keys []string
		var purged bool
		if err := t.QueryRow(r.Context(), `SELECT status::text, result_key, photo_keys, photos_purged_at IS NOT NULL FROM generations
			WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL FOR UPDATE`, id, uid).Scan(&status, &result, &keys, &purged); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return httpx.ErrNotFound
			}
			return err
		}
		if status == "queued" || status == "running" {
			return errInProgress
		}
		if result != nil {
			if err := h.Store.Delete(r.Context(), *result); err != nil {
				return err
			}
		}
		if _, err := t.Exec(r.Context(), `UPDATE generations SET deleted_at = now(), result_key = NULL, saved_at = NULL,
			result_purged_at = COALESCE(result_purged_at, now()) WHERE id = $1`, id); err != nil {
			return err
		}
		if purged {
			return nil
		}
		return purgeGenerationPhotos(r.Context(), t, h.Store, id, keys, time.Now().Add(-h.Retention.Photo))
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ---------- счётчик примерочной ----------

type fittingAddReq struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
}

func (h *Handlers) fittingAdd(w http.ResponseWriter, r *http.Request) {
	if h.Limiter != nil && !h.Limiter.Check(w, r, ruleFittingAdd, httpx.ClientIP(r.Context())) {
		return
	}
	var req fittingAddReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	who := httpx.ClientIP(r.Context())
	if p, ok := auth.FromContext(r.Context()); ok {
		who = p.UserID.String()
	}
	// одно добавление товара на посетителя в сутки — накрутка статистики бессмысленна
	if h.Redis != nil {
		ok, err := h.Redis.SetNX(r.Context(), "fit:"+req.ProductID.String()+":"+who, 1, 24*time.Hour).Result()
		if err == nil && !ok {
			httpx.NoContent(w)
			return
		}
	}
	if h.Fitting != nil {
		if err := h.Fitting.Hit(r.Context(), req.ProductID); err != nil {
			httpx.LoggerFrom(r.Context()).Warn("fitting counter unavailable", slog.String("err", err.Error()))
		}
	}
	httpx.NoContent(w)
}

// ---------- админ ----------

type adminRow struct {
	ID         uuid.UUID   `json:"id"`
	UserID     uuid.UUID   `json:"user_id"`
	Phone      *string     `json:"phone"`
	Status     string      `json:"status"`
	Cost       int64       `json:"cost"`
	Refunded   bool        `json:"refunded"`
	Provider   *string     `json:"provider"`
	Attempts   int         `json:"attempts"`
	ErrorCode  *string     `json:"error_code"`
	Error      *string     `json:"error"`
	ProductIDs []uuid.UUID `json:"product_ids"`
	CreatedAt  time.Time   `json:"created_at"`
	StartedAt  *time.Time  `json:"started_at"`
	FinishedAt *time.Time  `json:"finished_at"`
}

// adminList: GET /v1/admin/tryon/generations?status=&user_id=&cursor= — без изображений:
// администратору биометрия клиента не нужна.
func (h *Handlers) adminList(w http.ResponseWriter, r *http.Request) {
	limit := httpx.Limit(r, 50, 200)
	ts, cid := time.Now().Add(time.Hour), uuid.Max
	if c := r.URL.Query().Get("cursor"); c != "" {
		var ok bool
		if ts, cid, ok = parseCursor(c); !ok {
			httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_cursor", "invalid cursor"))
			return
		}
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "", "queued", "running", "succeeded", "failed":
	default:
		httpx.Fail(w, r, validation(map[string]string{"status": "oneof"}))
		return
	}
	var userID *uuid.UUID
	if s := r.URL.Query().Get("user_id"); s != "" {
		u, err := uuid.Parse(s)
		if err != nil {
			httpx.Fail(w, r, validation(map[string]string{"user_id": "uuid"}))
			return
		}
		userID = &u
	}
	rows, err := h.DB.Query(r.Context(), `SELECT g.id, g.user_id, u.phone, g.status::text, g.cost, g.refunded, g.provider, g.attempts,
		       g.error_code, g.error, g.product_ids, g.created_at, g.started_at, g.finished_at
		FROM generations g JOIN users u ON u.id = g.user_id
		WHERE (g.created_at, g.id) < ($1, $2) AND ($3 = '' OR g.status::text = $3) AND ($4::uuid IS NULL OR g.user_id = $4)
		ORDER BY g.created_at DESC, g.id DESC LIMIT $5`, ts, cid, status, userID, limit+1)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[adminRow])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = cursorOf(items[limit-1].CreatedAt, items[limit-1].ID)
	}
	httpx.List(w, items, next)
}

// adminStats: запуски и успешные генерации (дашборд суперадмина, ТЗ 11.1).
func (h *Handlers) adminStats(w http.ResponseWriter, r *http.Request) {
	var s struct {
		Queued     int64   `json:"queued"`
		Running    int64   `json:"running"`
		Day        int64   `json:"started_24h"`
		DayOK      int64   `json:"succeeded_24h"`
		DayFailed  int64   `json:"failed_24h"`
		Month      int64   `json:"started_30d"`
		MonthOK    int64   `json:"succeeded_30d"`
		AvgSeconds float64 `json:"avg_seconds_24h"`
	}
	err := h.DB.QueryRow(r.Context(), `SELECT
		count(*) FILTER (WHERE status = 'queued'),
		count(*) FILTER (WHERE status = 'running'),
		count(*) FILTER (WHERE created_at > now() - interval '24 hours'),
		count(*) FILTER (WHERE status = 'succeeded' AND created_at > now() - interval '24 hours'),
		count(*) FILTER (WHERE status = 'failed' AND created_at > now() - interval '24 hours'),
		count(*) FILTER (WHERE created_at > now() - interval '30 days'),
		count(*) FILTER (WHERE status = 'succeeded' AND created_at > now() - interval '30 days'),
		COALESCE(avg(extract(epoch FROM finished_at - created_at)) FILTER (WHERE status = 'succeeded' AND created_at > now() - interval '24 hours'), 0)
		FROM generations WHERE status IN ('queued','running') OR created_at > now() - interval '30 days'`).
		Scan(&s.Queued, &s.Running, &s.Day, &s.DayOK, &s.DayFailed, &s.Month, &s.MonthOK, &s.AvgSeconds)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, s)
}
