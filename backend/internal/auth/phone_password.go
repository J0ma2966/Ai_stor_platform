package auth

// Пароль клиента. SMS нужна только для регистрации и восстановления пароля,
// а обычный вход идёт по телефону и паролю — так каждое посещение не стоит денег.
//
//   POST /v1/auth/password/login  {phone, password}         — вход по паролю
//   POST /v1/auth/password/reset  {phone, code, password}   — новый пароль по коду из SMS
//                                                              (код запрашивается через /otp/request)
//   POST /v1/auth/password        {password, current_password?} — задать или сменить пароль
//                                                              (только вошедший клиент)
//
// Смена и сброс пароля отзывают все refresh-токены клиента (другие устройства
// выходят, когда истечёт их access-токен, не позже 15 минут), а текущее устройство
// получает новую сессию.

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/ratelimit"
)

var (
	rulePwLoginIP      = ratelimit.Rule{Name: "pw_login_ip", Limit: 20, Window: 15 * time.Minute}
	rulePwLoginPhoneIP = ratelimit.Rule{Name: "pw_login_phone_ip", Limit: 10, Window: 15 * time.Minute}
	// общий лимит на номер — от распределённого перебора; если его исчерпают,
	// клиент всё равно может войти по SMS или сбросить пароль.
	rulePwLoginPhone = ratelimit.Rule{Name: "pw_login_phone", Limit: 50, Window: 15 * time.Minute}
	rulePwResetIP    = ratelimit.Rule{Name: "pw_reset_ip", Limit: 30, Window: 15 * time.Minute}
	rulePwSetUser    = ratelimit.Rule{Name: "pw_set_user", Limit: 10, Window: 15 * time.Minute}

	ErrPasswordRequired = errors.New("current password required")
)

// PasswordRoutes монтируется в /v1/auth/password.
func (h *Handlers) PasswordRoutes(r chi.Router) {
	r.Post("/login", h.passwordLogin)
	r.Post("/reset", h.passwordReset)
	r.With(RequireRole(RoleClient)).Post("/", h.setPassword)
}

type pwLoginReq struct {
	Phone    string `json:"phone" validate:"required,e164"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

func (h *Handlers) passwordLogin(w http.ResponseWriter, r *http.Request) {
	var req pwLoginReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ip := httpx.ClientIP(r.Context())
	if !h.phoneAllowed(w, r, req.Phone) || !h.Limiter.Check(w, r, rulePwLoginIP, ip) ||
		!h.Limiter.Check(w, r, rulePwLoginPhoneIP, req.Phone+"|"+ip) || !h.Limiter.Check(w, r, rulePwLoginPhone, req.Phone) {
		return
	}
	sess, err := h.Svc.LoginPhonePassword(r.Context(), req.Phone, req.Password, meta(r))
	h.finish(w, r, sess, err)
}

type pwResetReq struct {
	Phone    string `json:"phone" validate:"required,e164"`
	Code     string `json:"code" validate:"required,len=6,numeric"`
	Password string `json:"password" validate:"required,min=10,max=128"`
}

func (h *Handlers) passwordReset(w http.ResponseWriter, r *http.Request) {
	var req pwResetReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.phoneAllowed(w, r, req.Phone) || !h.Limiter.Check(w, r, rulePwResetIP, httpx.ClientIP(r.Context())) {
		return
	}
	sess, err := h.Svc.ResetPassword(r.Context(), req.Phone, req.Code, req.Password, meta(r))
	h.finish(w, r, sess, err)
}

type setPwReq struct {
	Password        string `json:"password" validate:"required,min=10,max=128"`
	CurrentPassword string `json:"current_password" validate:"max=128"`
}

func (h *Handlers) setPassword(w http.ResponseWriter, r *http.Request) {
	var req setPwReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	uid := MustFrom(r.Context()).UserID
	if !h.Limiter.Check(w, r, rulePwSetUser, uid.String()) {
		return
	}
	sess, err := h.Svc.SetPassword(r.Context(), uid, req.CurrentPassword, req.Password, meta(r))
	switch {
	case errors.Is(err, ErrPasswordRequired):
		httpx.Fail(w, r, &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "current_password_required",
			Message: "current password is required", Fields: map[string]string{"current_password": "required"}})
		return
	case errors.Is(err, ErrBadCredentials):
		httpx.Fail(w, r, &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "current_password_wrong",
			Message: "current password is wrong", Fields: map[string]string{"current_password": "wrong"}})
		return
	}
	h.finish(w, r, sess, err)
}

// LoginPhonePassword — вход клиента по телефону и паролю. Клиент без пароля
// получает тот же ответ bad_credentials, что и при неверном пароле (нет перечисления).
func (s *Service) LoginPhonePassword(ctx context.Context, phone, password string, meta ClientMeta) (*Session, error) {
	var (
		p      = Principal{Role: RoleClient}
		hash   string
		status string
	)
	err := s.DB.QueryRow(ctx, `SELECT id, password_hash, status FROM users
		WHERE phone = $1 AND role = 'client' AND password_hash IS NOT NULL AND deleted_at IS NULL`, phone).
		Scan(&p.UserID, &hash, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = VerifyPassword(password, dummyHash)
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, err
	}
	if ok, err := VerifyPassword(password, hash); err != nil || !ok {
		return nil, ErrBadCredentials
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	return s.newSession(ctx, p, uuid.New(), meta)
}

// ResetPassword подтверждает телефон кодом из SMS и задаёт новый пароль.
// Неизвестный номер регистрируется (как при обычном входе по SMS), поэтому
// ответ не выдаёт, есть ли аккаунт; is_new_user ведёт такого клиента на завершение регистрации.
func (s *Service) ResetPassword(ctx context.Context, phone, code, password string, meta ClientMeta) (*Session, error) {
	hash, err := HashPassword(password) // до проверки кода: неподходящий пароль не сжигает код
	if err != nil {
		return nil, err
	}
	if err := s.OTP.Verify(ctx, phone, code); err != nil {
		return nil, err
	}
	p, isNew, err := s.clientByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}
	if err := s.storePassword(ctx, p.UserID, hash); err != nil {
		return nil, err
	}
	sess, err := s.newSession(ctx, p, uuid.New(), meta)
	if err != nil {
		return nil, err
	}
	sess.IsNew = isNew
	return sess, nil
}

// SetPassword задаёт первый пароль (после регистрации по SMS) или меняет
// существующий — тогда нужен текущий пароль.
func (s *Service) SetPassword(ctx context.Context, uid uuid.UUID, current, password string, meta ClientMeta) (*Session, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	var (
		old    *string
		status string
	)
	err = s.DB.QueryRow(ctx, `SELECT password_hash, status FROM users WHERE id = $1 AND role = 'client' AND deleted_at IS NULL`, uid).
		Scan(&old, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	if old != nil {
		if current == "" {
			return nil, ErrPasswordRequired
		}
		if ok, err := VerifyPassword(current, *old); err != nil || !ok {
			return nil, ErrBadCredentials
		}
	}
	if err := s.storePassword(ctx, uid, hash); err != nil {
		return nil, err
	}
	return s.newSession(ctx, Principal{UserID: uid, Role: RoleClient}, uuid.New(), meta)
}

// storePassword сохраняет хеш и отзывает все refresh-токены клиента.
func (s *Service) storePassword(ctx context.Context, uid uuid.UUID, hash string) error {
	return db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1 AND role = 'client'`, uid, hash); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, uid)
		return err
	})
}
