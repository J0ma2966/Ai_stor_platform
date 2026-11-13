package auth

// Вход через Google (docs/AUTH.md).
//
//   POST /v1/auth/google {id_token}
//
// Сайт получает ID-токен кнопкой Google Identity Services, Android-приложение —
// через Credential Manager (serverClientId = веб-клиент). API проверяет подпись по
// ключам Google (JWKS), издателя, срок, aud ∈ GOOGLE_CLIENT_IDS и email_verified.
//
// Аккаунт ищется по google_sub, затем по подтверждённой почте. Если найден аккаунт
// с той же, но НЕ подтверждённой почтой (тестовая регистрация без кода), его пароль
// стирается и сессии отзываются: иначе тот, кто заранее занял чужой адрес, сохранил
// бы доступ к аккаунту настоящего владельца.

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/ratelimit"
)

const GoogleCertsURL = "https://www.googleapis.com/oauth2/v3/certs"

var (
	ruleGoogleIP = ratelimit.Rule{Name: "google_ip", Limit: 30, Window: 15 * time.Minute}

	ErrGoogleToken = errors.New("google: invalid id token")
)

// GoogleVerifier проверяет ID-токены Google. Ключи кешируются по Cache-Control.
type GoogleVerifier struct {
	ClientIDs []string
	CertsURL  string // по умолчанию GoogleCertsURL (тесты подменяют)
	HTTP      *http.Client

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

// GoogleIdentity — проверенные данные из ID-токена.
type GoogleIdentity struct {
	Sub   string
	Email string // пусто, если Google не подтвердил почту
	Name  string
}

type googleClaims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	EmailVerified any    `json:"email_verified"` // bool или "true"
	Name          string `json:"name"`
}

func (v *GoogleVerifier) Verify(ctx context.Context, raw string) (*GoogleIdentity, error) {
	if len(v.ClientIDs) == 0 {
		return nil, ErrGoogleToken
	}
	var c googleClaims
	_, err := jwt.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.key(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(time.Minute),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGoogleToken, err)
	}
	if c.Issuer != "accounts.google.com" && c.Issuer != "https://accounts.google.com" {
		return nil, ErrGoogleToken
	}
	if !slices.ContainsFunc(c.Audience, func(a string) bool { return slices.Contains(v.ClientIDs, a) }) {
		return nil, ErrGoogleToken
	}
	if c.Subject == "" || len(c.Subject) > 255 {
		return nil, ErrGoogleToken
	}
	id := &GoogleIdentity{Sub: c.Subject, Name: c.Name}
	verified := c.EmailVerified == true || c.EmailVerified == "true"
	if verified && c.Email != "" && len(c.Email) <= 254 && strings.Contains(c.Email, "@") {
		id.Email = normEmail(c.Email)
	}
	return id, nil
}

func (v *GoogleVerifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && time.Now().Before(v.expires) {
		return k, nil
	}
	// ключ не найден или кеш устарел — перечитываем (Google меняет ключи примерно раз в неделю)
	if err := v.fetch(ctx); err != nil {
		if k, ok := v.keys[kid]; ok {
			return k, nil // Google недоступен — пока работаем со старыми ключами
		}
		return nil, err
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("google: unknown key id")
}

func (v *GoogleVerifier) fetch(ctx context.Context) error {
	url := v.CertsURL
	if url == "" {
		url = GoogleCertsURL
	}
	client := v.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req) // #nosec G107 G704 -- адрес ключей Google из кода/тестов
	if err != nil {
		return fmt.Errorf("google certs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("google certs: http %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&set); err != nil {
		return fmt.Errorf("google certs: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return errors.New("google certs: no keys")
	}
	v.keys = keys
	v.expires = time.Now().Add(maxAge(resp.Header.Get("Cache-Control")))
	return nil
}

func maxAge(cc string) time.Duration {
	for _, part := range strings.Split(cc, ",") {
		if s, ok := strings.CutPrefix(strings.TrimSpace(part), "max-age="); ok {
			if n, err := strconv.Atoi(s); err == nil && n > 0 {
				return min(time.Duration(n)*time.Second, 24*time.Hour)
			}
		}
	}
	return time.Hour
}

// GoogleRoutes монтируется в /v1/auth/google (только если заданы GOOGLE_CLIENT_IDS).
func (h *Handlers) GoogleRoutes(r chi.Router) {
	r.Post("/", h.googleLogin)
}

type googleReq struct {
	IDToken string `json:"id_token" validate:"required,max=4096"`
}

func (h *Handlers) googleLogin(w http.ResponseWriter, r *http.Request) {
	var req googleReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.Limiter.Check(w, r, ruleGoogleIP, httpx.ClientIP(r.Context())) {
		return
	}
	id, err := h.Svc.Google.Verify(r.Context(), req.IDToken)
	if err != nil {
		httpx.LoggerFrom(r.Context()).Info("google sign-in rejected", "err", err.Error())
		httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "google_token_invalid", "google sign-in failed, try again"))
		return
	}
	sess, err := h.Svc.LoginGoogle(r.Context(), id, meta(r))
	h.finish(w, r, sess, err)
}

// LoginGoogle находит или создаёт клиента по аккаунту Google.
func (s *Service) LoginGoogle(ctx context.Context, id *GoogleIdentity, meta ClientMeta) (*Session, error) {
	var (
		p      = Principal{Role: RoleClient}
		status string
		isNew  bool
	)
	err := db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		var role Role
		err := tx.QueryRow(ctx, `SELECT id, role, status FROM users WHERE google_sub = $1 AND deleted_at IS NULL`, id.Sub).
			Scan(&p.UserID, &role, &status)
		if err == nil {
			if role != RoleClient {
				return ErrBadCredentials
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if id.Email != "" {
			var verified bool
			err = tx.QueryRow(ctx, `SELECT id, role, status, email_verified_at IS NOT NULL FROM users
				WHERE email = $1 AND deleted_at IS NULL FOR UPDATE`, id.Email).Scan(&p.UserID, &role, &status, &verified)
			if err == nil {
				if role != RoleClient {
					return ErrBadCredentials
				}
				if !verified {
					// адрес был занят без подтверждения — доступ по старому паролю закрываем
					if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, p.UserID); err != nil {
						return err
					}
				}
				_, err = tx.Exec(ctx, `UPDATE users SET google_sub = $2, email_verified_at = COALESCE(email_verified_at, now()),
					password_hash = CASE WHEN email_verified_at IS NULL THEN NULL ELSE password_hash END
					WHERE id = $1`, p.UserID, id.Sub)
				return err
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		var email *string
		if id.Email != "" {
			email = &id.Email
		}
		isNew, status = true, "active"
		return tx.QueryRow(ctx, `INSERT INTO users (role, google_sub, email, email_verified_at)
			VALUES ('client', $1, $2, CASE WHEN $2::text IS NULL THEN NULL ELSE now() END) RETURNING id`, id.Sub, email).Scan(&p.UserID)
	})
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	sess, err := s.newSession(ctx, p, uuid.New(), meta)
	if err != nil {
		return nil, err
	}
	sess.IsNew = isNew
	return sess, nil
}
