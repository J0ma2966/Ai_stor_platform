package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestPasswordRoundTrip(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$") {
		t.Fatalf("unexpected hash format %q", h)
	}
	if ok, _ := VerifyPassword("correct horse battery", h); !ok {
		t.Fatal("valid password rejected")
	}
	if ok, _ := VerifyPassword("wrong password!!", h); ok {
		t.Fatal("wrong password accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
	if _, err := VerifyPassword("x", "$bcrypt$garbage"); err == nil {
		t.Fatal("garbage hash accepted")
	}
}

func TestJWT(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	s := NewSigner(secret, time.Minute)
	store := uuid.New()
	p := Principal{UserID: uuid.New(), Role: RoleStore, StoreID: store}
	tok, _, err := s.Sign(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Parse(tok)
	p.IssuedAt = got.IssuedAt
	if err != nil || got != p || got.IssuedAt == 0 {
		t.Fatalf("roundtrip: %v %+v", err, got)
	}

	t.Run("wrong secret", func(t *testing.T) {
		if _, err := NewSigner([]byte(strings.Repeat("x", 32)), time.Minute).Parse(tok); err == nil {
			t.Fatal("accepted token with wrong secret")
		}
	})
	t.Run("expired", func(t *testing.T) {
		old := NewSigner(secret, time.Minute)
		old.now = func() time.Time { return time.Now().Add(-time.Hour) }
		tok, _, _ := old.Sign(p)
		if _, err := s.Parse(tok); err == nil {
			t.Fatal("accepted expired token")
		}
	})
	t.Run("alg none", func(t *testing.T) {
		c := jwt.MapClaims{"sub": p.UserID.String(), "role": "admin", "iss": issuer, "exp": time.Now().Add(time.Hour).Unix()}
		tok, _ := jwt.NewWithClaims(jwt.SigningMethodNone, c).SignedString(jwt.UnsafeAllowNoneSignatureType)
		if _, err := s.Parse(tok); err == nil {
			t.Fatal("accepted alg=none token")
		}
	})
	t.Run("store role without store id", func(t *testing.T) {
		tok, _, _ := s.Sign(Principal{UserID: uuid.New(), Role: RoleStore})
		if _, err := s.Parse(tok); err == nil {
			t.Fatal("accepted store token without store_id")
		}
	})
}
