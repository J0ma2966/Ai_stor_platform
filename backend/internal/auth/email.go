package auth

// Регистрация и вход клиента по почте и паролю (docs/AUTH.md).
//
//   POST /v1/auth/email/code      {email, purpose: register|reset, lang?} — код на почту
//   POST /v1/auth/email/register  {email, code, password}  — новый аккаунт, почта подтверждена кодом
//   POST /v1/auth/email/login     {email, password}
//   POST /v1/auth/email/reset     {email, code, password}  — новый пароль по коду из письма
//
// Код — 6 цифр, 10 минут, 5 попыток, повторная отправка не чаще раза в минуту
// (те же механизмы, что у SMS-кодов: в Redis хранится только HMAC кода).

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/mail"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/ratelimit"
)

var (
	ruleEmailCodeIP      = ratelimit.Rule{Name: "email_code_ip", Limit: 20, Window: time.Hour}
	ruleEmailCodeAddr    = ratelimit.Rule{Name: "email_code_addr", Limit: 6, Window: time.Hour}
	ruleEmailVerifyIP    = ratelimit.Rule{Name: "email_verify_ip", Limit: 30, Window: 15 * time.Minute}
	ruleEmailLoginIP     = ratelimit.Rule{Name: "email_login_ip", Limit: 20, Window: 15 * time.Minute}
	ruleEmailLoginAddrIP = ratelimit.Rule{Name: "email_login_addr_ip", Limit: 10, Window: 15 * time.Minute}
	// общий лимит на адрес — от распределённого перебора; если его исчерпают,
	// клиент всё равно войдёт через восстановление пароля или Google.
	ruleEmailLoginAddr = ratelimit.Rule{Name: "email_login_addr", Limit: 50, Window: 15 * time.Minute}

	ErrEmailTaken = errors.New("email already registered")
)

// EmailRoutes монтируется в /v1/auth/email (только если вход по почте включён).
func (h *Handlers) EmailRoutes(r chi.Router) {
	r.Post("/code", h.emailCode)
	r.Post("/register", h.emailRegister)
	r.Post("/login", h.emailLogin)
	r.Post("/reset", h.emailReset)
}

func normEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func emailOTPKey(purpose mail.Purpose, email string) string {
	return "e:" + string(purpose) + ":" + email
}

type emailCodeReq struct {
	Email   string `json:"email" validate:"required,email,max=254"`
	Purpose string `json:"purpose" validate:"required,oneof=register reset"`
	Lang    string `json:"lang" validate:"omitempty,oneof=ru uz"`
}

func (h *Handlers) emailCode(w http.ResponseWriter, r *http.Request) {
	var req emailCodeReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := normEmail(req.Email)
	if !h.Limiter.Check(w, r, ruleEmailCodeIP, httpx.ClientIP(r.Context())) || !h.Limiter.Check(w, r, ruleEmailCodeAddr, email) {
		return
	}
	err := h.Svc.SendEmailCode(r.Context(), email, mail.Purpose(req.Purpose), req.Lang)
	if errors.Is(err, ErrOTPCooldown) {
		w.Header().Set("Retry-After", "60")
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "otp_cooldown", "wait before requesting a new code"))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Ответ одинаковый, есть аккаунт или нет (нет перечисления аккаунтов).
	httpx.JSON(w, http.StatusAccepted, map[string]any{"sent": true})
}

type emailCodePwReq struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Code     string `json:"code" validate:"required,len=6,numeric"`
	Password string `json:"password" validate:"required,min=10,max=128"`
}

func (h *Handlers) emailRegister(w http.ResponseWriter, r *http.Request) {
	var req emailCodePwReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.Limiter.Check(w, r, ruleEmailVerifyIP, httpx.ClientIP(r.Context())) {
		return
	}
	sess, err := h.Svc.RegisterEmail(r.Context(), normEmail(req.Email), req.Code, req.Password, meta(r))
	if errors.Is(err, ErrEmailTaken) {
		// Узнать это можно только с кодом из письма, то есть владельцу почты.
		httpx.Fail(w, r, &httpx.Error{Status: http.StatusConflict, Code: "email_taken",
			Message: "email is already registered", Fields: map[string]string{"email": "taken"}})
		return
	}
	h.finish(w, r, sess, err)
}

func (h *Handlers) emailReset(w http.ResponseWriter, r *http.Request) {
	var req emailCodePwReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.Limiter.Check(w, r, ruleEmailVerifyIP, httpx.ClientIP(r.Context())) {
		return
	}
	sess, err := h.Svc.ResetEmailPassword(r.Context(), normEmail(req.Email), req.Code, req.Password, meta(r))
	h.finish(w, r, sess, err)
}

type emailLoginReq struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

func (h *Handlers) emailLogin(w http.ResponseWriter, r *http.Request) {
	var req emailLoginReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := normEmail(req.Email)
	ip := httpx.ClientIP(r.Context())
	if !h.Limiter.Check(w, r, ruleEmailLoginIP, ip) || !h.Limiter.Check(w, r, ruleEmailLoginAddrIP, email+"|"+ip) ||
		!h.Limiter.Check(w, r, ruleEmailLoginAddr, email) {
		return
	}
	sess, err := h.Svc.LoginEmail(r.Context(), email, req.Password, meta(r))
	h.finish(w, r, sess, err)
}

// SendEmailCode отправляет код. Для восстановления пароля письмо уходит только
// существующему клиенту, но ответ API от этого не меняется.
func (s *Service) SendEmailCode(ctx context.Context, email string, purpose mail.Purpose, lang string) error {
	if purpose == mail.PurposeReset {
		var exists bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE email = $1 AND role = 'client' AND deleted_at IS NULL)`,
			email).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}
	code, err := s.EmailOTP.Issue(ctx, emailOTPKey(purpose, email))
	if err != nil {
		return err
	}
	return s.Mail.SendCode(ctx, email, code, purpose, lang)
}

// RegisterEmail создаёт клиента с подтверждённой почтой. Дальше — тот же путь,
// что у любого нового клиента: is_new_user=true, выбор пола и согласие.
func (s *Service) RegisterEmail(ctx context.Context, email, code, password string, meta ClientMeta) (*Session, error) {
	hash, err := HashPassword(password) // до проверки кода: неподходящий пароль не сжигает код
	if err != nil {
		return nil, err
	}
	if err := s.EmailOTP.Verify(ctx, emailOTPKey(mail.PurposeRegister, email), code); err != nil {
		return nil, err
	}
	p := Principal{Role: RoleClient}
	err = s.DB.QueryRow(ctx, `INSERT INTO users (role, email, email_verified_at, password_hash)
		VALUES ('client', $1, now(), $2) RETURNING id`, email, hash).Scan(&p.UserID)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return nil, ErrEmailTaken
	}
	if err != nil {
		return nil, err
	}
	sess, err := s.newSession(ctx, p, uuid.New(), meta)
	if err != nil {
		return nil, err
	}
	sess.IsNew = true
	return sess, nil
}

// ResetEmailPassword задаёт новый пароль по коду из письма. Подходит и тем, кто
// зарегистрировался через Google и хочет входить ещё и по паролю.
func (s *Service) ResetEmailPassword(ctx context.Context, email, code, password string, meta ClientMeta) (*Session, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	if err := s.EmailOTP.Verify(ctx, emailOTPKey(mail.PurposeReset, email), code); err != nil {
		return nil, err
	}
	p := Principal{Role: RoleClient}
	var status string
	err = db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `UPDATE users SET password_hash = $2, email_verified_at = COALESCE(email_verified_at, now())
			WHERE email = $1 AND role = 'client' AND deleted_at IS NULL RETURNING id, status`, email, hash).Scan(&p.UserID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOTPInvalid
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, p.UserID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	return s.newSession(ctx, p, uuid.New(), meta)
}

func (s *Service) LoginEmail(ctx context.Context, email, password string, meta ClientMeta) (*Session, error) {
	var (
		p      = Principal{Role: RoleClient}
		hash   string
		status string
	)
	err := s.DB.QueryRow(ctx, `SELECT id, password_hash, status FROM users
		WHERE email = $1 AND role = 'client' AND password_hash IS NOT NULL AND deleted_at IS NULL`, email).
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

// PurgeIdentities — для users.PurgeFunc: при удалении аккаунта стирает почту,
// пароль и привязки Telegram и Google, чтобы их можно было зарегистрировать заново.
func PurgeIdentities(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE users SET email = NULL, email_verified_at = NULL, password_hash = NULL,
		telegram_user_id = NULL, google_sub = NULL WHERE id = $1 AND role = 'client'`, userID)
	return err
}
