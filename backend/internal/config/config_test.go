package config

import (
	"strings"
	"testing"
)

func base(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("j", 32))
	t.Setenv("OTP_PEPPER", strings.Repeat("p", 16))
	// примерка выключена: её prod-требования (хранилище в UZ) проверяются отдельно
	t.Setenv("TRYON_PROVIDER", "off")
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
	t.Setenv("PAYMENT_RETURN_URL", "https://shop.uz/account/balance")
	t.Setenv("PAYME_ALLOWED_IPS", "185.234.113.0/24, nope")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("bad ip: %v", err)
	}
	t.Setenv("PAYME_ALLOWED_IPS", "185.234.113.0/24,10.0.0.1")
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}

func TestPaymentProdRules(t *testing.T) {
	base(t)
	t.Setenv("APP_ENV", "prod")
	t.Setenv("SMS_PROVIDER", "eskiz")
	t.Setenv("ESKIZ_EMAIL", "a@b.uz")
	t.Setenv("ESKIZ_PASSWORD", "pw")
	t.Setenv("PAYMENT_RETURN_URL", "https://shop.uz/account/balance")
	t.Setenv("PAYME_MERCHANT_ID", "m")
	t.Setenv("PAYME_KEY", "k")
	t.Setenv("CLICK_SERVICE_ID", "1")
	t.Setenv("CLICK_MERCHANT_ID", "2")
	t.Setenv("CLICK_SECRET_KEY", "s")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CLICK_ALLOWED_IPS") || !strings.Contains(err.Error(), "PAYME_ALLOWED_IPS") {
		t.Fatalf("prod must require provider IP allow-lists: %v", err)
	}
	t.Setenv("CLICK_ALLOWED_IPS", "10.0.0.0/8")
	t.Setenv("PAYME_ALLOWED_IPS", "10.0.0.0/8")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.PaymeTest {
		t.Fatal("PAYME_TEST must default to false in prod")
	}
}

func TestTryOnOffInProdNeedsNoBiometricStorage(t *testing.T) {
	c := Config{Env: "prod"}
	c.TryOn.Provider = "off"
	if errs := c.validateTryOn(); len(errs) != 0 {
		t.Fatalf("off in prod must not require try-on settings: %v", errs)
	}
	if c.TryOnEnabled() {
		t.Fatal("off must disable try-on")
	}
}

func TestTryOnEnabledInProdRequiresUZStorage(t *testing.T) {
	c := Config{Env: "prod"}
	c.TryOn.Provider = "gemini"
	c.TryOn.APIKey = "k"
	errs := c.validateTryOn()
	var all []string
	for _, e := range errs {
		all = append(all, e.Error())
	}
	s := strings.Join(all, "; ")
	if !strings.Contains(s, "BIOMETRIC_S3_ENDPOINT") || !strings.Contains(s, "BIOMETRIC_STORAGE_COUNTRY") {
		t.Fatalf("prod with try-on on must require UZ biometric storage, got %q", s)
	}
	c.TryOn.Provider = "mock"
	if len(c.validateTryOn()) == 0 {
		t.Fatal("mock must be refused in prod")
	}
}
