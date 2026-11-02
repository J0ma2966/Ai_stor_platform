// Package admin — операции суперадминистратора. Каждое действие пишется в audit_log
// в той же транзакции.
package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/tokens"
)

type Handlers struct {
	DB      *pgxpool.Pool
	Revoker *auth.Revoker
}

func (h *Handlers) Routes(r chi.Router) {
	r.Post("/stores", h.createStore)
	r.Post("/stores/{id}/status", h.setStoreStatus)
	r.Post("/users/{id}/tokens", h.adjustTokens)
	r.Post("/users/{id}/status", h.setUserStatus)
}

type createStoreReq struct {
	Slug     string `json:"slug" validate:"required,min=2,max=63"`
	Name     string `json:"name" validate:"required,min=1,max=200"`
	Login    string `json:"login" validate:"required,min=3,max=64"`
	Password string `json:"password" validate:"required,min=10,max=128"`
	Phone    string `json:"phone,omitempty" validate:"omitempty,e164"`
}

// createStore: магазин и его единственная учётная запись; статус pending_activation.
func (h *Handlers) createStore(w http.ResponseWriter, r *http.Request) {
	var req createStoreReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(422, "weak_password", err.Error()))
		return
	}
	var storeID, userID uuid.UUID
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `INSERT INTO stores (slug, name, phone) VALUES ($1, $2, NULLIF($3, '')) RETURNING id`,
			req.Slug, req.Name, req.Phone).Scan(&storeID); err != nil {
			return err
		}
		if err := tx.QueryRow(r.Context(), `INSERT INTO users (role, login, password_hash, store_id) VALUES ('store', $1, $2, $3) RETURNING id`,
			req.Login, hash, storeID).Scan(&userID); err != nil {
			return err
		}
		return audit.Log(r.Context(), tx, "store.create", "store", storeID.String(), map[string]any{"slug": req.Slug, "login": req.Login})
	})
	switch {
	case db.IsUniqueViolation(err, "stores_slug_key"):
		httpx.Fail(w, r, httpx.NewError(409, "slug_taken", "slug already used"))
	case db.IsUniqueViolation(err, "users_login_key"):
		httpx.Fail(w, r, httpx.NewError(409, "login_taken", "login already used"))
	case isCheck(err):
		httpx.Fail(w, r, httpx.NewError(422, "validation_failed", "slug or login has invalid format"))
	case err != nil:
		httpx.Fail(w, r, err)
	default:
		httpx.JSON(w, http.StatusCreated, map[string]any{"store_id": storeID, "user_id": userID, "status": "pending_activation"})
	}
}

type storeStatusReq struct {
	Status string `json:"status" validate:"required,oneof=active blocked archived"`
	Reason string `json:"reason,omitempty" validate:"max=500"`
}

func (h *Handlers) setStoreStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req storeStatusReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `
			UPDATE stores SET status = $2::store_status,
			  activated_at = CASE WHEN $2 = 'active' THEN COALESCE(activated_at, now()) ELSE activated_at END
			WHERE id = $1`, id, req.Status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return db.ErrNotFound
		}
		if req.Status != "active" {
			// блокировка/архив сразу выкидывает сессии магазина: refresh в БД + access через Redis
			if _, err := tx.Exec(r.Context(), `
				UPDATE refresh_tokens SET revoked_at = now()
				WHERE revoked_at IS NULL AND user_id IN (SELECT id FROM users WHERE store_id = $1)`, id); err != nil {
				return err
			}
			rows, err := tx.Query(r.Context(), `SELECT id FROM users WHERE store_id = $1`, id)
			if err != nil {
				return err
			}
			ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
			if err != nil {
				return err
			}
			// до коммита: если Redis недоступен, операция откатывается и админ повторит её
			if err := h.Revoker.RevokeUser(r.Context(), ids...); err != nil {
				return err
			}
		}
		return audit.Log(r.Context(), tx, "store.status", "store", id.String(), map[string]any{"status": req.Status, "reason": req.Reason})
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

type userStatusReq struct {
	Status string `json:"status" validate:"required,oneof=active blocked"`
	Reason string `json:"reason" validate:"required,min=3,max=500"`
}

// setUserStatus — блокировка/разблокировка клиента; блокировка действует мгновенно.
func (h *Handlers) setUserStatus(w http.ResponseWriter, r *http.Request) {
	uid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req userStatusReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		tag, err := tx.Exec(r.Context(), `UPDATE users SET status = $2::user_status WHERE id = $1 AND role = 'client'`, uid, req.Status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return db.ErrNotFound
		}
		if req.Status == "blocked" {
			if _, err := tx.Exec(r.Context(), `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, uid); err != nil {
				return err
			}
			if err := h.Revoker.RevokeUser(r.Context(), uid); err != nil {
				return err
			}
		}
		return audit.Log(r.Context(), tx, "user.status", "user", uid.String(), map[string]any{"status": req.Status, "reason": req.Reason})
	})
	if errors.Is(err, db.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"id": uid, "status": req.Status})
}

type adjustReq struct {
	Amount int64  `json:"amount" validate:"required,ne=0,min=-100000,max=100000"`
	Reason string `json:"reason" validate:"required,min=3,max=500"`
}

// adjustTokens — ручная корректировка с обязательной причиной.
func (h *Handlers) adjustTokens(w http.ResponseWriter, r *http.Request) {
	uid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req adjustReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	actor := auth.MustFrom(r.Context()).UserID
	var bal int64
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var role string
		if err := tx.QueryRow(r.Context(), `SELECT role FROM users WHERE id = $1`, uid).Scan(&role); err != nil {
			return db.NotFound(err)
		}
		if role != "client" {
			return db.ErrNotFound
		}
		bal, err = tokens.Apply(r.Context(), tx, tokens.Entry{UserID: uid, Op: tokens.OpAdminAdjustment, Amount: req.Amount, Reason: req.Reason, ActorID: &actor})
		if err != nil {
			return err
		}
		return audit.Log(r.Context(), tx, "tokens.adjust", "user", uid.String(), map[string]any{"amount": req.Amount, "reason": req.Reason})
	})
	switch {
	case errors.Is(err, db.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrNotFound)
	case errors.Is(err, tokens.ErrInsufficient):
		httpx.Fail(w, r, httpx.NewError(422, "insufficient_tokens", "balance cannot go below zero"))
	case err != nil:
		httpx.Fail(w, r, err)
	default:
		httpx.JSON(w, http.StatusOK, map[string]any{"user_id": uid, "token_balance": bal})
	}
}

// CreateAdmin — для команды `api create-admin` (первичная настройка).
func CreateAdmin(ctx context.Context, pool *pgxpool.Pool, login, password string) (uuid.UUID, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = pool.QueryRow(ctx, `INSERT INTO users (role, login, password_hash) VALUES ('admin', $1, $2) RETURNING id`, login, hash).Scan(&id)
	return id, err
}
