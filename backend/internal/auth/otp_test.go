package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newOTP(t *testing.T) (*OTP, *miniredis.Miniredis) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewOTP(rdb, []byte("pepper-pepper-pepper"), 5*time.Minute, time.Minute, 5), mr
}

func TestOTPFlow(t *testing.T) {
	ctx := context.Background()
	o, mr := newOTP(t)
	const phone = "+998901234567"
	code, err := o.Issue(ctx, phone)
	if err != nil || len(code) != 6 {
		t.Fatalf("issue: %v %q", err, code)
	}
	if stored := mr.HGet("otp:"+phone, "h"); stored == code || stored == "" {
		t.Fatal("code must be stored hashed")
	}
	if _, err := o.Issue(ctx, phone); !errors.Is(err, ErrOTPCooldown) {
		t.Fatalf("expected cooldown, got %v", err)
	}
	if err := o.Verify(ctx, phone, "000000x"); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal("wrong code accepted")
	}
	if err := o.Verify(ctx, phone, code); err != nil {
		t.Fatalf("valid code rejected: %v", err)
	}
	if err := o.Verify(ctx, phone, code); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal("code reused")
	}
}

func TestOTPMaxTriesBurnsCode(t *testing.T) {
	ctx := context.Background()
	o, _ := newOTP(t)
	const phone = "+998901111111"
	code, _ := o.Issue(ctx, phone)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		_ = o.Verify(ctx, phone, wrong)
	}
	if err := o.Verify(ctx, phone, code); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal("code must be burned after max tries")
	}
}

func TestOTPExpires(t *testing.T) {
	ctx := context.Background()
	o, mr := newOTP(t)
	const phone = "+998902222222"
	code, _ := o.Issue(ctx, phone)
	mr.FastForward(6 * time.Minute)
	if err := o.Verify(ctx, phone, code); !errors.Is(err, ErrOTPInvalid) {
		t.Fatal("expired code accepted")
	}
}
