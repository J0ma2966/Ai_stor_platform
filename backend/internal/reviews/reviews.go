// Package reviews — отзывы и оценки товаров (docs/REVIEWS.md).
//
// Правила:
//   - отзыв оставляет только клиент, у которого есть доставленный заказ с этим товаром;
//   - один отзыв на товар от клиента, его можно изменить или удалить;
//   - отзыв публикуется сразу; администратор может скрыть его с причиной;
//   - магазин отвечает на отзывы о своих товарах, но скрыть их не может;
//   - средняя оценка и число отзывов хранятся в products (rating_avg, rating_count)
//     и пересчитываются в той же транзакции под блокировкой строки товара (Refresh).
package reviews

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
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

// Границы оценки. Нижняя граница продублирована в CHECK миграции 00040.
const (
	MinRating = 1
	MaxRating = 5
	MaxBody   = 2000
)

type Handlers struct {
	DB         *pgxpool.Pool
	PublicBase string
}

// PublicRoutes: GET /v1/products/{id}/reviews — видимые отзывы о товаре.
func (h *Handlers) PublicRoutes(r chi.Router) {
	r.Get("/products/{id}/reviews", h.publicList)
}

// ClientRoutes монтируются под /v1/reviews (роль client).
func (h *Handlers) ClientRoutes(r chi.Router) {
	r.Get("/products/{id}", h.mine)
	r.Put("/products/{id}", h.upsert)
	r.Delete("/products/{id}", h.remove)
}

// StoreRoutes монтируются под /v1/store/reviews (роль store).
func (h *Handlers) StoreRoutes(r chi.Router) {
	r.Get("/", h.storeList)
	r.Put("/{id}/reply", h.storeReply)
	r.Delete("/{id}/reply", h.storeDeleteReply)
}

// AdminRoutes монтируются под /v1/admin/reviews (роль admin).
func (h *Handlers) AdminRoutes(r chi.Router) {
	r.Get("/", h.adminList)
	r.Post("/{id}/status", h.adminModerate)
}

// Review — отзыв в публичном списке. Все отзывы — от покупателей (проверено при создании).
type Review struct {
	ID         uuid.UUID  `json:"id"`
	Rating     int        `json:"rating"`
	Body       string     `json:"body"`
	AuthorName string     `json:"author_name"`
	Reply      *string    `json:"reply"`
	ReplyAt    *time.Time `json:"reply_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Full — отзыв для кабинетов: свой отзыв клиента, кабинет магазина, модерация.
type Full struct {
	Review
	ProductID    uuid.UUID `json:"product_id"`
	ProductTitle string    `json:"product_title,omitempty"`
	ImageURL     string    `json:"image_url,omitempty"`
	StoreID      uuid.UUID `json:"store_id,omitempty"`
	StoreName    string    `json:"store_name,omitempty"`
	Status       string    `json:"status"`
	Reason       *string   `json:"moderation_reason"`
}

func pathID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, httpx.ErrNotFound
	}
	return id, nil
}

// ---------- публичный список ----------

// publicList: GET /v1/products/{id}/reviews?rating=&cursor=&limit=
func (h *Handlers) publicList(w http.ResponseWriter, r *http.Request) {
	pid, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	where := []string{"r.product_id = $1", "r.status = 'visible'"}
	args := []any{pid}
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	if v := r.URL.Query().Get("rating"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < MinRating || n > MaxRating {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "rating out of range"))
			return
		}
		where = append(where, "r.rating = "+arg(n))
	}
	if c := r.URL.Query().Get("cursor"); c != "" {
		t, id, err := decodeCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_cursor", "invalid cursor"))
			return
		}
		where = append(where, "(r.created_at, r.id) < ("+arg(t)+", "+arg(id)+")")
	}
	limit := httpx.Limit(r, 10, 50)
	// nosemgrep: go.lang.security.injection.tainted-sql-string.tainted-sql-string -- только константы и плейсхолдеры $N
	sql := `SELECT r.id, r.rating, r.body, r.author_name, r.reply, r.reply_at, r.created_at, r.updated_at
		FROM product_reviews r
		JOIN products p ON p.id = r.product_id AND p.status = 'published'
		WHERE ` + strings.Join(where, " AND ") + ` ORDER BY r.created_at DESC, r.id DESC LIMIT ` + strconv.Itoa(limit+1)
	rows, err := h.DB.Query(r.Context(), sql, args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Review, error) {
		var v Review
		err := row.Scan(&v.ID, &v.Rating, &v.Body, &v.AuthorName, &v.Reply, &v.ReplyAt, &v.CreatedAt, &v.UpdatedAt)
		return v, err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = encodeCursor(items[limit-1].CreatedAt, items[limit-1].ID)
	}
	w.Header().Set("Cache-Control", "public, max-age=30")
	httpx.List(w, items, next)
}

// ---------- клиент ----------

type mineResp struct {
	CanReview bool  `json:"can_review"` // есть доставленный заказ с товаром
	Review    *Full `json:"review"`
}

// mine: GET /v1/reviews/products/{id} — может ли клиент оставить отзыв и его текущий отзыв.
func (h *Handlers) mine(w http.ResponseWriter, r *http.Request) {
	pid, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	uid := auth.MustFrom(r.Context()).UserID
	var resp mineResp
	if _, ok, err := purchase(r.Context(), h.DB, uid, pid); err != nil {
		httpx.Fail(w, r, err)
		return
	} else {
		resp.CanReview = ok
	}
	f, err := h.loadOwn(r.Context(), uid, pid)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, err)
		return
	}
	resp.Review = f
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, resp)
}

func (h *Handlers) loadOwn(ctx context.Context, uid, pid uuid.UUID) (*Full, error) {
	var f Full
	err := h.DB.QueryRow(ctx, `SELECT id, product_id, rating, body, author_name, reply, reply_at, created_at, updated_at, status::text, moderation_reason
		FROM product_reviews WHERE user_id = $1 AND product_id = $2`, uid, pid).
		Scan(&f.ID, &f.ProductID, &f.Rating, &f.Body, &f.AuthorName, &f.Reply, &f.ReplyAt, &f.CreatedAt, &f.UpdatedAt, &f.Status, &f.Reason)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

type querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// purchase ищет доставленный заказ клиента с товаром и возвращает имя получателя
// из последнего такого заказа (из него строится подпись «Азиз К.»).
func purchase(ctx context.Context, q querier, uid, pid uuid.UUID) (string, bool, error) {
	var name string
	err := q.QueryRow(ctx, `SELECT o.customer_name FROM orders o JOIN order_items oi ON oi.order_id = o.id
		WHERE o.user_id = $1 AND oi.product_id = $2 AND o.status = 'delivered'
		ORDER BY o.created_at DESC LIMIT 1`, uid, pid).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}

type upsertReq struct {
	Rating int    `json:"rating"`
	Body   string `json:"body" validate:"max=8000"` // байты; точная проверка — по символам ниже
}

// upsert: PUT /v1/reviews/products/{id} — создать или изменить свой отзыв.
func (h *Handlers) upsert(w http.ResponseWriter, r *http.Request) {
	pid, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req upsertReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Body = cleanText(req.Body)
	fields := map[string]string{}
	if req.Rating < MinRating || req.Rating > MaxRating {
		fields["rating"] = "range"
	}
	if len([]rune(req.Body)) > MaxBody {
		fields["body"] = "max"
	}
	if len(fields) > 0 {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "request validation failed", Fields: fields})
		return
	}
	uid := auth.MustFrom(r.Context()).UserID
	created := false
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		storeID, title, err := lockProduct(r.Context(), tx, pid, true)
		if err != nil {
			return err
		}
		name, ok, err := purchase(r.Context(), tx, uid, pid)
		if err != nil {
			return err
		}
		if !ok {
			return httpx.NewError(http.StatusForbidden, "review_not_allowed", "review is available after the order with this product is delivered")
		}
		var status string
		err = tx.QueryRow(r.Context(), `SELECT status::text FROM product_reviews WHERE user_id = $1 AND product_id = $2`, uid, pid).Scan(&status)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			created = true
			if _, err := tx.Exec(r.Context(), `INSERT INTO product_reviews (product_id, user_id, rating, body, author_name)
				VALUES ($1, $2, $3, $4, $5)`, pid, uid, req.Rating, req.Body, ShortName(name)); err != nil {
				return err
			}
		case err != nil:
			return err
		case status == "hidden":
			// иначе правка обходила бы модерацию; удалить скрытый отзыв можно
			return httpx.NewError(http.StatusConflict, "review_hidden", "review was hidden by moderator and cannot be edited")
		default:
			if _, err := tx.Exec(r.Context(), `UPDATE product_reviews SET rating = $3, body = $4 WHERE user_id = $1 AND product_id = $2`,
				uid, pid, req.Rating, req.Body); err != nil {
				return err
			}
		}
		if err := Refresh(r.Context(), tx, pid); err != nil {
			return err
		}
		if created {
			return notify.SendToStore(r.Context(), tx, storeID, notify.ReviewCreated,
				map[string]any{"product_id": pid, "title": title, "rating": req.Rating})
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f, err := h.loadOwn(r.Context(), uid, pid)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.JSON(w, status, f)
}

// remove: DELETE /v1/reviews/products/{id} — удалить свой отзыв.
func (h *Handlers) remove(w http.ResponseWriter, r *http.Request) {
	pid, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	uid := auth.MustFrom(r.Context()).UserID
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		if _, _, err := lockProduct(r.Context(), tx, pid, false); err != nil {
			return err
		}
		tag, err := tx.Exec(r.Context(), `DELETE FROM product_reviews WHERE user_id = $1 AND product_id = $2`, uid, pid)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.ErrNotFound
		}
		return Refresh(r.Context(), tx, pid)
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// lockProduct блокирует строку товара, чтобы параллельные отзывы не затёрли
// пересчёт средней оценки друг друга. published=true — товар должен быть на витрине.
func lockProduct(ctx context.Context, tx pgx.Tx, pid uuid.UUID, published bool) (uuid.UUID, string, error) {
	var storeID uuid.UUID
	var title, status string
	err := tx.QueryRow(ctx, `SELECT store_id, title, status::text FROM products WHERE id = $1 FOR UPDATE`, pid).Scan(&storeID, &title, &status)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && published && status != "published") {
		return uuid.Nil, "", httpx.ErrNotFound
	}
	return storeID, title, err
}

// Refresh пересчитывает среднюю оценку и число видимых отзывов товара.
// Вызывать в транзакции после изменения отзывов (строка товара уже заблокирована).
func Refresh(ctx context.Context, tx pgx.Tx, pid uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE products p SET rating_count = s.c, rating_avg = s.a
		FROM (SELECT count(*)::int AS c, COALESCE(round(avg(rating), 2), 0) AS a
		      FROM product_reviews WHERE product_id = $1 AND status = 'visible') s
		WHERE p.id = $1 AND (p.rating_count, p.rating_avg) IS DISTINCT FROM (s.c, s.a)`, pid)
	return err
}

// PurgeUser удаляет отзывы клиента при удалении аккаунта и пересчитывает рейтинги.
func PurgeUser(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	rows, err := tx.Query(ctx, `DELETE FROM product_reviews WHERE user_id = $1 RETURNING product_id`, userID)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, pid := range ids {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM products WHERE id = $1 FOR UPDATE`, pid); err != nil {
			return err
		}
		if err := Refresh(ctx, tx, pid); err != nil {
			return err
		}
	}
	return nil
}

// ShortName превращает имя получателя заказа в подпись отзыва: «Азиз Каримов» → «Азиз К.».
// Фамилия и телефон в публичный отзыв не попадают.
func ShortName(full string) string {
	full = tagRe.ReplaceAllString(full, " ")
	words := strings.FieldsFunc(full, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || r == '<' || r == '>'
	})
	if len(words) == 0 {
		return ""
	}
	first := []rune(words[0])
	if len(first) > 30 {
		first = first[:30]
	}
	out := string(first)
	if len(words) > 1 {
		if r := []rune(words[1]); unicode.IsLetter(r[0]) {
			out += " " + string(unicode.ToUpper(r[0])) + "."
		}
	}
	return out
}

var tagRe = regexp.MustCompile(`<[^>]*>`)

// cleanText убирает управляющие символы (кроме переводов строк) и лишние пробелы по краям.
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// ---------- кабинет магазина ----------

const fullCols = `r.id, r.product_id, r.rating, r.body, r.author_name, r.reply, r.reply_at, r.created_at, r.updated_at,
	r.status::text, r.moderation_reason, p.title, p.store_id, s.name,
	(SELECT key FROM product_images pi WHERE pi.product_id = p.id AND pi.angle = 'front')`

func (h *Handlers) scanFull(rows pgx.Rows) ([]Full, error) {
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Full, error) {
		var f Full
		var key *string
		err := row.Scan(&f.ID, &f.ProductID, &f.Rating, &f.Body, &f.AuthorName, &f.Reply, &f.ReplyAt, &f.CreatedAt, &f.UpdatedAt,
			&f.Status, &f.Reason, &f.ProductTitle, &f.StoreID, &f.StoreName, &key)
		if key != nil {
			f.ImageURL = media.PublicURL(h.PublicBase, *key)
		}
		return f, err
	})
}

// listFull — общий список для кабинета магазина и модерации.
func (h *Handlers) listFull(w http.ResponseWriter, r *http.Request, where []string, args []any) {
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }
	q := r.URL.Query()
	if v := q.Get("rating"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < MinRating || n > MaxRating {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "rating out of range"))
			return
		}
		where = append(where, "r.rating = "+arg(n))
	}
	if v := q.Get("product"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "product must be uuid"))
			return
		}
		where = append(where, "r.product_id = "+arg(id))
	}
	if q.Get("unanswered") == "true" {
		where = append(where, "r.reply IS NULL")
	}
	if c := q.Get("cursor"); c != "" {
		t, id, err := decodeCursor(c)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_cursor", "invalid cursor"))
			return
		}
		where = append(where, "(r.created_at, r.id) < ("+arg(t)+", "+arg(id)+")")
	}
	limit := httpx.Limit(r, 20, 100)
	// nosemgrep: go.lang.security.injection.tainted-sql-string.tainted-sql-string -- только константы и плейсхолдеры $N
	sql := `SELECT ` + fullCols + ` FROM product_reviews r JOIN products p ON p.id = r.product_id JOIN stores s ON s.id = p.store_id
		WHERE ` + strings.Join(where, " AND ") + ` ORDER BY r.created_at DESC, r.id DESC LIMIT ` + strconv.Itoa(limit+1)
	rows, err := h.DB.Query(r.Context(), sql, args...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := h.scanFull(rows)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = encodeCursor(items[limit-1].CreatedAt, items[limit-1].ID)
	}
	httpx.List(w, items, next)
}

// storeList: GET /v1/store/reviews?product=&rating=&unanswered=true&cursor= — видимые отзывы о товарах магазина.
func (h *Handlers) storeList(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	h.listFull(w, r, []string{"p.store_id = $1", "r.status = 'visible'"}, []any{p.StoreID})
}

type replyReq struct {
	Text string `json:"text" validate:"required,max=8000"`
}

// storeReply: PUT /v1/store/reviews/{id}/reply — ответ магазина (создать или изменить).
func (h *Handlers) storeReply(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req replyReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Text = cleanText(req.Text)
	if n := len([]rune(req.Text)); n == 0 || n > MaxBody {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "request validation failed", Fields: map[string]string{"text": "length"}})
		return
	}
	p := auth.MustFrom(r.Context())
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var userID, productID uuid.UUID
		var title string
		var had bool
		err := tx.QueryRow(r.Context(), `
			WITH old AS (SELECT r.id, r.reply IS NOT NULL AS had FROM product_reviews r JOIN products p ON p.id = r.product_id
			             WHERE r.id = $1 AND p.store_id = $2 AND r.status = 'visible' FOR UPDATE OF r)
			UPDATE product_reviews r SET reply = $3, reply_at = now() FROM old, products p
			WHERE r.id = old.id AND p.id = r.product_id
			RETURNING r.user_id, r.product_id, p.title, old.had`, id, p.StoreID, req.Text).Scan(&userID, &productID, &title, &had)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound // чужой и несуществующий отзыв не различаются
		}
		if err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "review.reply", "review", id.String(), nil); err != nil {
			return err
		}
		if had {
			return nil
		}
		return notify.Send(r.Context(), tx, userID, notify.ReviewReply, map[string]any{"product_id": productID, "title": title})
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": id, "reply": req.Text})
}

// storeDeleteReply: DELETE /v1/store/reviews/{id}/reply.
func (h *Handlers) storeDeleteReply(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	p := auth.MustFrom(r.Context())
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `UPDATE product_reviews r SET reply = NULL, reply_at = NULL FROM products p
			WHERE r.id = $1 AND p.id = r.product_id AND p.store_id = $2 AND r.reply IS NOT NULL`, id, p.StoreID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.ErrNotFound
		}
		return audit.Log(r.Context(), tx, "review.reply_delete", "review", id.String(), nil)
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ---------- модерация ----------

// adminList: GET /v1/admin/reviews?status=&rating=&store=&product=&unanswered=&cursor=
func (h *Handlers) adminList(w http.ResponseWriter, r *http.Request) {
	where := []string{"true"}
	var args []any
	q := r.URL.Query()
	if v := q.Get("status"); v != "" {
		if v != "visible" && v != "hidden" {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "unknown status"))
			return
		}
		args = append(args, v)
		where = append(where, "r.status = $"+strconv.Itoa(len(args))+"::review_status")
	}
	if v := q.Get("store"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_filter", "store must be uuid"))
			return
		}
		args = append(args, id)
		where = append(where, "p.store_id = $"+strconv.Itoa(len(args)))
	}
	h.listFull(w, r, where, args)
}

type moderateReq struct {
	Status string `json:"status" validate:"required,oneof=visible hidden"`
	Reason string `json:"reason" validate:"max=500"`
}

// adminModerate: POST /v1/admin/reviews/{id}/status — скрыть (с причиной) или вернуть отзыв.
func (h *Handlers) adminModerate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req moderateReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Status == "hidden" && len([]rune(req.Reason)) < 3 {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "reason is required", Fields: map[string]string{"reason": "required"}})
		return
	}
	if req.Status == "visible" {
		req.Reason = ""
	}
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var pid, userID uuid.UUID
		if err := tx.QueryRow(r.Context(), `SELECT product_id, user_id FROM product_reviews WHERE id = $1`, id).Scan(&pid, &userID); err != nil {
			return db.NotFound(err)
		}
		var title string
		if err := tx.QueryRow(r.Context(), `SELECT title FROM products WHERE id = $1 FOR UPDATE`, pid).Scan(&title); err != nil {
			return err
		}
		var old string
		if err := tx.QueryRow(r.Context(), `
			WITH old AS (SELECT status FROM product_reviews WHERE id = $1)
			UPDATE product_reviews SET status = $2::review_status, moderation_reason = NULLIF($3, '')
			WHERE id = $1 RETURNING (SELECT status::text FROM old)`, id, req.Status, req.Reason).Scan(&old); err != nil {
			return err
		}
		if err := Refresh(r.Context(), tx, pid); err != nil {
			return err
		}
		if err := audit.Log(r.Context(), tx, "review.moderate", "review", id.String(), map[string]any{"status": req.Status, "reason": req.Reason}); err != nil {
			return err
		}
		if old != req.Status && req.Status == "hidden" {
			return notify.Send(r.Context(), tx, userID, notify.ReviewHidden,
				map[string]any{"product_id": pid, "title": title, "reason": req.Reason})
		}
		return nil
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
