package auth

// ВРЕМЕННО: регистрация и вход клиента по почте и паролю — только на период
// тестирования, пока нет SMS-провайдера. Включается EMAIL_AUTH_ENABLED
// (по умолчанию выключено в prod). Почта не подтверждается.
//
// Как удалить целиком (docs/EMAIL_AUTH.md): этот файл и email_temp_test.go,
// строки с EmailAuth в server.go и config.go, web/src/components/EmailAuthForm.tsx
// и его подключение в web/src/app/login/page.tsx, ключи emailAuth в messages,
// плюс новая миграция, которая повторяет Down из 00090_email_auth_testing.sql.

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
	"github.com/boutique-market/backend/internal/ratelimit"
)

var (
	ruleEmailRegisterIP = ratelimit.Rule{Name: "email_register_ip", Limit: 10, Window: time.Hour}
	ruleEmailLoginIP    = ratelimit.Rule{Name: "email_login_ip", Limit: 20, Window: 15 * time.Minute}
	ruleEmailLoginUser  = ratelimit.Rule{Name: "email_login_user", Limit: 10, Window: 15 * time.Minute}

	ErrEmailTaken = errors.New("email already registered")
)

// EmailRoutes монтируется в /v1/auth/email. status отвечает всегда (фронт решает,
// показывать ли форму); register и login есть только при enabled.
func (h *Handlers) EmailRoutes(enabled bool) func(chi.Router) {
	return func(r chi.Router) {
		r.Get("/status", func(w http.ResponseWriter, _ *http.Request) {
			httpx.JSON(w, http.StatusOK, map[string]any{"enabled": enabled})
		})
		if !enabled {
			return
		}
		r.Post("/register", h.emailRegister)
		r.Post("/login", h.emailLogin)
	}
}

type emailReq struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=10,max=128"`
}

// При входе длина пароля не проверяется строго: неверный пароль — bad_credentials.
type emailLoginReq struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

func normEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func (h *Handlers) emailRegister(w http.ResponseWriter, r *http.Request) {
	var req emailReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.Limiter.Check(w, r, ruleEmailRegisterIP, httpx.ClientIP(r.Context())) {
		return
	}
	sess, err := h.Svc.RegisterEmail(r.Context(), normEmail(req.Email), req.Password, meta(r))
	if errors.Is(err, ErrEmailTaken) {
		httpx.Fail(w, r, &httpx.Error{Status: http.StatusConflict, Code: "email_taken",
			Message: "email is already registered", Fields: map[string]string{"email": "taken"}})
		return
	}
	h.finish(w, r, sess, err)
}

func (h *Handlers) emailLogin(w http.ResponseWriter, r *http.Request) {
	var req emailLoginReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	email := normEmail(req.Email)
	if !h.Limiter.Check(w, r, ruleEmailLoginIP, httpx.ClientIP(r.Context())) ||
		!h.Limiter.Check(w, r, ruleEmailLoginUser, email) {
		return
	}
	sess, err := h.Svc.LoginEmail(r.Context(), email, req.Password, meta(r))
	h.finish(w, r, sess, err)
}

// RegisterEmail создаёт клиента без телефона. Дальше — тот же путь, что и после
// OTP: is_new_user=true, фронт ведёт на выбор пола и согласие (users.CompleteRegistration).
func (s *Service) RegisterEmail(ctx context.Context, email, password string, meta ClientMeta) (*Session, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	p := Principal{Role: RoleClient}
	err = s.DB.QueryRow(ctx, `INSERT INTO users (role, email, password_hash) VALUES ('client', $1, $2) RETURNING id`,
		email, hash).Scan(&p.UserID)
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

// PurgeEmail — для users.PurgeFunc: при удалении аккаунта стирает почту и пароль.
func PurgeEmail(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE users SET email = NULL, password_hash = NULL WHERE id = $1 AND role = 'client'`, userID)
	return err
}
