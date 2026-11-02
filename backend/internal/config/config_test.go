package config

import (
	"strings"
	"testing"
)

func base(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("j", 32))
	t.Setenv("OTP_PEPPER", strings.Repeat("p", 16))
}

func TestSMSProviderRules(t *testing.T) {
	base(t)
	if _, err := Load(); err != nil {
		t.Fatalf("dev with log sender must load: %v", err)
	}
	t.Setenv("APP_ENV", "prod")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMS_PROVIDER=log") {
		t.Fatalf("prod must reject log sender: %v", err)
	}
	t.Setenv("SMS_PROVIDER", "eskiz")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ESKIZ_EMAIL") {
		t.Fatalf("eskiz needs credentials: %v", err)
	}
	t.Setenv("ESKIZ_EMAIL", "a@b.uz")
	t.Setenv("ESKIZ_PASSWORD", "pw")
	t.Setenv("SMS_OTP_TEMPLATE", "Kod %d %s")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMS_OTP_TEMPLATE") {
		t.Fatalf("bad template: %v", err)
	}
	t.Setenv("SMS_OTP_TEMPLATE", "Kod: %s")
	if _, err := Load(); err != nil {
		t.Fatalf("valid eskiz config: %v", err)
	}
}

func TestPaymentConfig(t *testing.T) {
	base(t)
	t.Setenv("PAYME_MERCHANT_ID", "m")
	t.Setenv("PAYME_KEY", "k")
	c, err := Load()
	if err == nil || !strings.Contains(err.Error(), "PAYMENT_RETURN_URL") {
		t.Fatalf("return url required: %v", err)
	}
	if !c.PaymeEnabled() || c.ClickEnabled() || !c.PaymeTest || c.PaymeAccountKey != "payment_id" {
		t.Fatalf("defaults: %+v", c)
	}
	t.Setenv("PAYMENT_RETURN_URL", "https://shop.uz/ru/account/balance")
	t.Setenv("PAYME_ALLOWED_IPS", "185.234.113.0/24, nope")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("bad ip: %v", err)
	}
	t.Setenv("PAYME_ALLOWED_IPS", "185.234.113.0/24,10.0.0.1")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
