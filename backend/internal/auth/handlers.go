package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/ratelimit"
)

const refreshCookie = "rt"

type Handlers struct {
	Svc             *Service
	AllowedPrefixes []string // пусто = любые номера
	Limiter         *ratelimit.Limiter
	CookieDomain    string
	CookieSecure    bool
}

var (
	ruleOTPPhone = ratelimit.Rule{Name: "otp_phone", Limit: 5, Window: time.Hour}
	ruleOTPIP    = ratelimit.Rule{Name: "otp_ip", Limit: 20, Window: time.Hour}
	ruleVerifyIP = ratelimit.Rule{Name: "otp_verify_ip", Limit: 30, Window: 15 * time.Minute}
	ruleLoginIP  = ratelimit.Rule{Name: "login_ip", Limit: 20, Window: 15 * time.Minute}
	// логин+IP: злоумышленник не может заблокировать вход админу с другого адреса;
	// login (общий): ограничивает распределённый перебор одного аккаунта.
	ruleLoginUserIP = ratelimit.Rule{Name: "login_user_ip", Limit: 10, Window: 15 * time.Minute}
	ruleLoginUser   = ratelimit.Rule{Name: "login_user", Limit: 100, Window: 15 * time.Minute}
	// refresh идёт на каждую полную загрузку страницы, а за одним IP оператора/офиса
	// сидит много людей; токен 256-битный, так что лимит защищает от нагрузки, не от подбора.
	ruleRefreshIP = ratelimit.Rule{Name: "refresh_ip", Limit: 600, Window: 15 * time.Minute}
)

func (h *Handlers) Routes(r chi.Router) {
	r.Post("/otp/request", h.requestOTP)
	r.Post("/otp/verify", h.verifyOTP)
	r.Post("/login", h.login)
	r.Post("/refresh", h.refresh)
	r.Post("/logout", h.logout)
}

type otpRequest struct {
	Phone string `json:"phone" validate:"required,e164"`
}

func (h *Handlers) requestOTP(w http.ResponseWriter, r *http.Request) {
	var req otpRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.phoneAllowed(w, r, req.Phone) {
		return
	}
	if !h.Limiter.Check(w, r, ruleOTPIP, httpx.ClientIP(r.Context())) || !h.Limiter.Check(w, r, ruleOTPPhone, req.Phone) {
		return
	}
	err := h.Svc.RequestOTP(r.Context(), req.Phone)
	if errors.Is(err, ErrOTPCooldown) {
		w.Header().Set("Retry-After", "60")
		httpx.Fail(w, r, httpx.NewError(http.StatusTooManyRequests, "otp_cooldown", "wait before requesting a new code"))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Одинаковый ответ для нового и существующего номера (нет перечисления аккаунтов).
	httpx.JSON(w, http.StatusAccepted, map[string]any{"sent": true})
}

type otpVerify struct {
	Phone string `json:"phone" validate:"required,e164"`
	Code  string `json:"code" validate:"required,len=6,numeric"`
}

func (h *Handlers) verifyOTP(w http.ResponseWriter, r *http.Request) {
	var req otpVerify
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.phoneAllowed(w, r, req.Phone) || !h.Limiter.Check(w, r, ruleVerifyIP, httpx.ClientIP(r.Context())) {
		return
	}
	sess, err := h.Svc.VerifyOTP(r.Context(), req.Phone, req.Code, meta(r))
	h.finish(w, r, sess, err)
}

type loginReq struct {
	Login    string `json:"login" validate:"required,min=3,max=64"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ip := httpx.ClientIP(r.Context())
	if !h.Limiter.Check(w, r, ruleLoginIP, ip) || !h.Limiter.Check(w, r, ruleLoginUserIP, req.Login+"|"+ip) ||
		!h.Limiter.Check(w, r, ruleLoginUser, req.Login) {
		return
	}
	sess, err := h.Svc.Login(r.Context(), req.Login, req.Password, meta(r))
	h.finish(w, r, sess, err)
}

func (h *Handlers) refresh(w http.ResponseWriter, r *http.Request) {
	if !h.Limiter.Check(w, r, ruleRefreshIP, httpx.ClientIP(r.Context())) {
		return
	}
	c, err := r.Cookie(refreshCookie)
	if err != nil || c.Value == "" {
		httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "refresh_invalid", "refresh token missing"))
		return
	}
	sess, err := h.Svc.Refresh(r.Context(), c.Value, meta(r))
	if err != nil {
		h.clearCookie(w)
	}
	h.finish(w, r, sess, err)
}

func (h *Handlers) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookie); err == nil && c.Value != "" {
		if err := h.Svc.Logout(r.Context(), c.Value); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	h.clearCookie(w)
	httpx.NoContent(w)
}

func (h *Handlers) finish(w http.ResponseWriter, r *http.Request, sess *Session, err error) {
	switch {
	case err == nil:
	case errors.Is(err, ErrOTPInvalid):
		httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "otp_invalid", "code is invalid or expired"))
		return
	case errors.Is(err, ErrBadCredentials):
		httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "bad_credentials", "invalid login or password"))
		return
	case errors.Is(err, ErrAccountBlocked):
		httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "account_blocked", "account is blocked"))
		return
	case errors.Is(err, ErrStoreInactive):
		httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "store_not_active", "store is not active"))
		return
	case errors.Is(err, ErrRefreshInvalid):
		httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "refresh_invalid", "refresh token invalid"))
		return
	default:
		httpx.Fail(w, r, err)
		return
	}
	// #nosec G124 -- HttpOnly+SameSite всегда; Secure=false допускается только вне prod (проверяет config.Load)
	// Secure = COOKIE_SECURE; config.Load не даёт запустить prod с false.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: sess.refreshToken, Path: "/v1/auth", Domain: h.CookieDomain,
		Expires: sess.refreshExp, HttpOnly: true, Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token": sess.AccessToken,
		"expires_at":   sess.ExpiresAt,
		"is_new_user":  sess.IsNew,
		"user":         map[string]any{"id": sess.Principal.UserID, "role": sess.Principal.Role},
	})
}

func (h *Handlers) clearCookie(w http.ResponseWriter) {
	// #nosec G124 -- см. выше
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Value: "", Path: "/v1/auth", Domain: h.CookieDomain, MaxAge: -1,
		HttpOnly: true, Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handlers) phoneAllowed(w http.ResponseWriter, r *http.Request, phone string) bool {
	if len(h.AllowedPrefixes) == 0 {
		return true
	}
	for _, p := range h.AllowedPrefixes {
		if strings.HasPrefix(phone, p) {
			return true
		}
	}
	httpx.Fail(w, r, &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "phone_not_allowed",
		Message: "phone numbers of this country are not supported", Fields: map[string]string{"phone": "country"}})
	return false
}

func meta(r *http.Request) ClientMeta {
	return ClientMeta{UserAgent: r.UserAgent(), IP: httpx.ClientIP(r.Context())}
}
