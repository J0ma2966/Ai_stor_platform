package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Role string

const (
	RoleClient Role = "client"
	RoleStore  Role = "store"
	RoleAdmin  Role = "admin"
)

// Principal — кто делает запрос. Все запросы к данным обязаны фильтроваться по нему.
type Principal struct {
	UserID  uuid.UUID
	Role    Role
	StoreID uuid.UUID // только для роли store
}

type claims struct {
	Role    Role   `json:"role"`
	StoreID string `json:"store_id,omitempty"`
	jwt.RegisteredClaims
}

const issuer = "boutique-api"

type Signer struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewSigner(secret []byte, ttl time.Duration) *Signer {
	return &Signer{secret: secret, ttl: ttl, now: time.Now}
}

func (s *Signer) Sign(p Principal) (string, time.Time, error) {
	now := s.now()
	exp := now.Add(s.ttl)
	c := claims{
		Role: p.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   p.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	if p.StoreID != uuid.Nil {
		c.StoreID = p.StoreID.String()
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.secret)
	return tok, exp, err
}

var ErrInvalidToken = errors.New("invalid token")

func (s *Signer) Parse(tok string) (Principal, error) {
	var c claims
	_, err := jwt.ParseWithClaims(tok, &c, func(t *jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(s.now), jwt.WithLeeway(5*time.Second))
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	uid, err := uuid.Parse(c.Subject)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	p := Principal{UserID: uid, Role: c.Role}
	switch c.Role {
	case RoleClient, RoleAdmin:
	case RoleStore:
		if p.StoreID, err = uuid.Parse(c.StoreID); err != nil {
			return Principal{}, ErrInvalidToken
		}
	default:
		return Principal{}, ErrInvalidToken
	}
	return p, nil
}

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext возвращает Principal; ok=false для анонимного запроса.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// MustFrom используется в хендлерах за RequireRole — там Principal гарантирован.
func MustFrom(ctx context.Context) Principal {
	p, ok := FromContext(ctx)
	if !ok {
		panic("auth: principal missing; route is not behind RequireRole")
	}
	return p
}
