// Package users — профиль клиента и завершение регистрации.
package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/tokens"
)

type Handlers struct {
	DB      *pgxpool.Pool
	Revoker *auth.Revoker // отзыв access-токенов при удалении аккаунта
	Purge   []PurgeFunc   // очистка данных в других модулях при удалении аккаунта
	SMSAuth bool          // вход по номеру и SMS включён (SMS_AUTH_ENABLED)
}

// Routes монтируются за RequireRole(client) — кроме GET /me, доступного всем ролям.
func (h *Handlers) Routes(r chi.Router) {
	r.Get("/", h.me)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireRole(auth.RoleClient))
		r.Patch("/", h.update)
		r.Post("/complete-registration", h.completeRegistration)
		r.Get("/tokens", h.tokenHistory)
		r.Delete("/", h.deleteAccount)
	})
}

type Me struct {
	ID           uuid.UUID       `json:"id"`
	Role         string          `json:"role"`
	Phone        *string         `json:"phone,omitempty"`
	Login        *string         `json:"login,omitempty"`
	StoreID      *uuid.UUID      `json:"store_id,omitempty"`
	Gender       *string         `json:"gender"`
	Lang         string          `json:"lang"`
	TokenBalance int64           `json:"token_balance"`
	HeightCM     *int16          `json:"height_cm"`
	WeightKG     *float64        `json:"weight_kg"`
	BodyParams   json.RawMessage `json:"body_params"`
	Registered   bool            `json:"registration_complete"`
	HasPassword  bool            `json:"has_password"`
	Email        *string         `json:"email,omitempty"`
	HasTelegram  bool            `json:"has_telegram"`
	HasGoogle    bool            `json:"has_google"`
	// NeedsPassword — клиент вошёл по SMS и ещё не задал пароль: без пароля ему
	// пришлось бы каждый раз платить за SMS. Остальным способам входа пароль не нужен.
	NeedsPassword bool       `json:"needs_password"`
	PDConsentAt   *time.Time `json:"pd_consent_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (h *Handlers) load(r *http.Request, id uuid.UUID) (*Me, error) {
	var m Me
	err := h.DB.QueryRow(r.Context(), `
		SELECT id, role, phone, login, store_id, gender, lang, token_balance, height_cm, weight_kg::float8, body_params, pd_consent_at, created_at,
		       password_hash IS NOT NULL, email, telegram_user_id IS NOT NULL, google_sub IS NOT NULL
		FROM users WHERE id = $1`, id).
		Scan(&m.ID, &m.Role, &m.Phone, &m.Login, &m.StoreID, &m.Gender, &m.Lang, &m.TokenBalance, &m.HeightCM, &m.WeightKG, &m.BodyParams, &m.PDConsentAt, &m.CreatedAt, &m.HasPassword,
			&m.Email, &m.HasTelegram, &m.HasGoogle)
	if err != nil {
		return nil, db.NotFound(err)
	}
	m.Registered = m.Role != "client" || (m.Gender != nil && m.PDConsentAt != nil)
	m.NeedsPassword = h.SMSAuth && m.Role == "client" && !m.HasPassword && m.Phone != nil &&
		m.Email == nil && !m.HasTelegram && !m.HasGoogle
	return &m, nil
}

func (h *Handlers) me(w http.ResponseWriter, r *http.Request) {
	m, err := h.load(r, auth.MustFrom(r.Context()).UserID)
	if errors.Is(err, db.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

type completeReq struct {
	Gender  string `json:"gender" validate:"required,oneof=male female"`
	Consent bool   `json:"pd_consent" validate:"required"` // согласие на обработку ПД и изображений
	Lang    string `json:"lang,omitempty" validate:"omitempty,oneof=ru uz"`
}

// completeRegistration — обязательный шаг выбора пола и согласия. Начисляет
// приветственный бонус строго один раз (уникальный индекс token_ledger_welcome_once).
func (h *Handlers) completeRegistration(w http.ResponseWriter, r *http.Request) {
	var req completeReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	uid := auth.MustFrom(r.Context()).UserID
	err := db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(r.Context(), `
			UPDATE users SET gender = $2, pd_consent_at = COALESCE(pd_consent_at, now()),
			       lang = COALESCE(NULLIF($3, '')::ui_lang, lang)
			WHERE id = $1`, uid, req.Gender, req.Lang); err != nil {
			return err
		}
		bonus, err := tokens.Setting(r.Context(), tx, "welcome_bonus")
		if err != nil || bonus <= 0 {
			return err
		}
		_, err = tokens.Apply(r.Context(), tx, tokens.Entry{UserID: uid, Op: tokens.OpWelcomeBonus, Amount: bonus})
		if errors.Is(err, tokens.ErrDuplicate) {
			return nil // уже начислен — повторно не начисляем
		}
		return err
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.me(w, r)
}

type updateReq struct {
	Gender     *string          `json:"gender" validate:"omitempty,oneof=male female"`
	Lang       *string          `json:"lang" validate:"omitempty,oneof=ru uz"`
	HeightCM   *int16           `json:"height_cm" validate:"omitempty,min=50,max=260"`
	WeightKG   *float64         `json:"weight_kg" validate:"omitempty,min=20,max=400"`
	BodyParams *json.RawMessage `json:"body_params"`
}

func (h *Handlers) update(w http.ResponseWriter, r *http.Request) {
	var req updateReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body any
	if req.BodyParams != nil {
		var m map[string]float64 // только числовые параметры, не больше 20
		if err := json.Unmarshal(*req.BodyParams, &m); err != nil || len(m) > 20 || !validBodyParams(m) {
			httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "body_params must be an object of numbers", Fields: map[string]string{"body_params": "invalid"}})
			return
		}
		body = m
	}
	uid := auth.MustFrom(r.Context()).UserID
	_, err := h.DB.Exec(r.Context(), `
		UPDATE users SET
		  gender      = COALESCE($2::gender, gender),
		  lang        = COALESCE($3::ui_lang, lang),
		  height_cm   = COALESCE($4, height_cm),
		  weight_kg   = COALESCE($5, weight_kg),
		  body_params = COALESCE($6, body_params)
		WHERE id = $1`, uid, req.Gender, req.Lang, req.HeightCM, req.WeightKG, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.me(w, r)
}

type ledgerRow struct {
	ID           int64     `json:"id"`
	Op           string    `json:"op"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	Reason       *string   `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// tokenHistory — история операций с курсором по id (keyset-пагинация).
func (h *Handlers) tokenHistory(w http.ResponseWriter, r *http.Request) {
	uid := auth.MustFrom(r.Context()).UserID
	limit := httpx.Limit(r, 20, 100)
	before := int64(1 << 62)
	if c := r.URL.Query().Get("cursor"); c != "" {
		v, err := strconv.ParseInt(c, 10, 64)
		if err != nil {
			httpx.Fail(w, r, httpx.NewError(400, "bad_cursor", "invalid cursor"))
			return
		}
		before = v
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, op, amount, balance_after, reason, created_at FROM token_ledger
		WHERE user_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3`, uid, before, limit+1)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ledgerRow])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = strconv.FormatInt(items[limit-1].ID, 10)
	}
	httpx.List(w, items, next)
}

// validBodyParams — те же ограничения, что у параметров тела в примерке (tryon.bodyReq):
// имя 1..32 символа (буквы, цифры, _), значение 1..400.
func validBodyParams(m map[string]float64) bool {
	for k, v := range m {
		n := utf8.RuneCountInString(k)
		if n < 1 || n > 32 || v < 1 || v > 400 {
			return false
		}
		for _, r := range k {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
				return false
			}
		}
	}
	return true
}
