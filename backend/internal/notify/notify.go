// Package notify — уведомления пользователей всех ролей (кабинет клиента, магазина).
//
// Другие модули вызывают Send в своей транзакции, поэтому уведомление появляется
// ровно тогда, когда событие действительно зафиксировано (статус заказа, оплата,
// генерация). Тексты не хранятся: фронт переводит kind + data на ru/uz.
package notify

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
)

// Виды уведомлений. Новый вид — добавить сюда и перевод в web/messages/*.json (notifications.kinds).
const (
	OrderCreated        = "order_created"        // магазину: новый заказ
	OrderStatus         = "order_status"         // клиенту: статус заказа изменён
	ProductModerated    = "product_moderated"    // магазину: товар снят/заблокирован/возвращён
	PaymentPaid         = "payment_paid"         // клиенту: оплата прошла, токены начислены
	PaymentFailed       = "payment_failed"       // клиенту: оплата отклонена/отменена
	GenerationSucceeded = "generation_succeeded" // клиенту: примерка готова
	GenerationFailed    = "generation_failed"    // клиенту: ошибка генерации, токены возвращены
	ReviewCreated       = "review_created"       // магазину: новый отзыв о товаре
	ReviewReply         = "review_reply"         // клиенту: магазин ответил на отзыв
	ReviewHidden        = "review_hidden"        // клиенту: отзыв скрыт модератором
	System              = "system"               // системное сообщение
)

// Retention — сколько хранятся уведомления (janitor удаляет старые).
const Retention = 180 * 24 * time.Hour

type Execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Send создаёт уведомление пользователю. data — только идентификаторы и короткие
// значения (номер заказа, статус), без персональных данных.
func Send(ctx context.Context, q Execer, userID uuid.UUID, kind string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	_, err := q.Exec(ctx, `INSERT INTO notifications (user_id, kind, data) VALUES ($1, $2, $3)`, userID, kind, data)
	return err
}

// SendToStore уведомляет учётную запись магазина (она одна на магазин).
func SendToStore(ctx context.Context, q Execer, storeID uuid.UUID, kind string, data map[string]any) error {
	if data == nil {
		data = map[string]any{}
	}
	_, err := q.Exec(ctx, `INSERT INTO notifications (user_id, kind, data)
		SELECT id, $2, $3 FROM users WHERE store_id = $1 AND role = 'store'`, storeID, kind, data)
	return err
}

type Handlers struct{ DB *pgxpool.Pool }

// Routes монтируются под /v1/me/notifications (любая роль).
func (h *Handlers) Routes(r chi.Router) {
	r.Get("/", h.list)
	r.Get("/unread-count", h.unread)
	r.Post("/read", h.markRead)
}

type Item struct {
	ID        int64          `json:"id"`
	Kind      string         `json:"kind"`
	Data      map[string]any `json:"data"`
	Read      bool           `json:"read"`
	CreatedAt time.Time      `json:"created_at"`
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	limit := httpx.Limit(r, 20, 100)
	before := int64(1 << 62)
	if c := r.URL.Query().Get("cursor"); c != "" {
		v, err := strconv.ParseInt(c, 10, 64)
		if err != nil || v <= 0 {
			httpx.Fail(w, r, httpx.NewError(400, "bad_cursor", "invalid cursor"))
			return
		}
		before = v
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, kind, data, read_at IS NOT NULL, created_at FROM notifications
		WHERE user_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3`, uid, before, limit+1)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Item])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.FormatInt(items[limit-1].ID, 10)
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.List(w, items, next)
}

func (h *Handlers) unread(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	var n int64
	// считаем максимум 100: на бейдже всё равно «99+»
	if err := h.DB.QueryRow(r.Context(), `SELECT count(*) FROM (SELECT 1 FROM notifications WHERE user_id = $1 AND read_at IS NULL LIMIT 100) x`, uid).Scan(&n); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{"unread": n})
}

type readReq struct {
	IDs []int64 `json:"ids" validate:"max=100"`
	All bool    `json:"all"`
}

// markRead: {"ids":[...]} — отметить выбранные, {"all":true} — все.
func (h *Handlers) markRead(w http.ResponseWriter, r *http.Request) {
	var req readReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !req.All && len(req.IDs) == 0 {
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "ids or all required", Fields: map[string]string{"ids": "required"}})
		return
	}
	uid := auth.MustFrom(r.Context()).UserID
	var err error
	if req.All {
		_, err = h.DB.Exec(r.Context(), `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, uid)
	} else {
		_, err = h.DB.Exec(r.Context(), `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND id = ANY($2) AND read_at IS NULL`, uid, req.IDs)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ValidKind проверяет формат kind (как CHECK в БД).
func ValidKind(k string) bool {
	if len(k) < 3 || len(k) > 64 {
		return false
	}
	return strings.Trim(k, "abcdefghijklmnopqrstuvwxyz_") == ""
}
