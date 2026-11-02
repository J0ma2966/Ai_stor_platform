package users

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
)

// PurgeFunc удаляет данные пользователя в других модулях (фото тела и результаты
// примерок). Вызывается в транзакции удаления аккаунта.
type PurgeFunc func(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error

type deleteReq struct {
	Confirm bool `json:"confirm" validate:"required"`
}

// deleteAccount — удаление аккаунта клиентом (отзыв согласия на обработку ПД).
// Телефон, пол, параметры тела, корзина, избранное, уведомления удаляются.
// Строка users остаётся обезличенной: на неё ссылаются заказы и журнал токенов,
// которые нужно хранить для учёта (в заказе остаются имя и телефон получателя —
// это данные сделки, срок их хранения определяется политикой конфиденциальности).
// Тот же номер телефона потом можно зарегистрировать заново как новый аккаунт.
func (h *Handlers) deleteAccount(w http.ResponseWriter, r *http.Request) {
	var req deleteReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	uid := auth.MustFrom(r.Context()).UserID
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `UPDATE users SET phone = NULL, gender = NULL, height_cm = NULL, weight_kg = NULL,
			body_params = '{}'::jsonb, pd_consent_at = NULL, status = 'blocked', deleted_at = now()
			WHERE id = $1 AND role = 'client' AND deleted_at IS NULL`, uid)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.ErrNotFound
		}
		for _, q := range []string{
			`DELETE FROM cart_items WHERE user_id = $1`,
			`DELETE FROM favorites WHERE user_id = $1`,
			`DELETE FROM notifications WHERE user_id = $1`,
			`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`,
		} {
			if _, err := tx.Exec(r.Context(), q, uid); err != nil {
				return err
			}
		}
		for _, purge := range h.Purge {
			if err := purge(r.Context(), tx, uid); err != nil {
				return err
			}
		}
		if err := audit.Log(r.Context(), tx, "user.delete", "user", uid.String(), nil); err != nil {
			return err
		}
		if h.Revoker != nil {
			return h.Revoker.RevokeUser(r.Context(), uid)
		}
		return nil
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
