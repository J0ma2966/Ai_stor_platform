package auth

import (
	"net/http"
	"strings"

	"github.com/boutique-market/backend/internal/httpx"
)

// Authenticate разбирает Bearer-токен, если он есть. Невалидный токен => 401
// (а не «аноним»), чтобы фронт сразу пошёл за refresh.
func Authenticate(s *Signer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" {
				next.ServeHTTP(w, r)
				return
			}
			tok, ok := strings.CutPrefix(h, "Bearer ")
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			p, err := s.Parse(tok)
			if err != nil {
				httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "token_invalid", "access token invalid or expired"))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// RequireRole пропускает только аутентифицированных пользователей с одной из ролей.
// Доступ по умолчанию запрещён: роут без RequireRole — публичный и не видит Principal.
func RequireRole(roles ...Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := FromContext(r.Context())
			if !ok {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			for _, role := range roles {
				if p.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.Fail(w, r, httpx.ErrForbidden)
		})
	}
}
