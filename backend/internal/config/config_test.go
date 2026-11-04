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

func TestOpenAIConfig(t *testing.T) {
	base(t)
	t.Setenv("TRYON_PROVIDER", "openai")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY is required") {
		t.Fatalf("missing key must be a config error even in dev: %v", err)
	}
	t.Setenv("OPENAI_API_KEY", "sk-test")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	o := c.TryOn.OpenAI
	if o.Model != "gpt-image-2.5-sunburst" || o.Quality != "high" || o.Size != "1536x1024" || o.Format != "jpeg" || o.Timeout.Seconds() != 180 {
		t.Fatalf("defaults: %+v", o)
	}
	if c.TryOn.JobTimeout() < o.Timeout {
		t.Fatalf("job timeout %s must cover the OpenAI request timeout %s", c.TryOn.JobTimeout(), o.Timeout)
	}
	if strings.Contains(c.String(), "sk-test") {
		t.Fatal("API key must not be printed")
	}
	for k, v := range map[string]string{"OPENAI_IMAGE_QUALITY": "ultra", "OPENAI_IMAGE_FORMAT": "gif", "OPENAI_IMAGE_SIZE": "1000x1000"} {
		t.Setenv(k, v)
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), k) {
			t.Fatalf("%s=%s must be rejected: %v", k, v, err)
		}
		t.Setenv(k, "")
	}
	for _, s := range []string{"auto", "1536x1024", "1024x1536", "2304x1536", "3840x2160"} {
		if err := ValidOpenAISize(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"1536x", "1536x1024x2", "4096x4096", "3072x512", "1530x1024"} {
		if ValidOpenAISize(s) == nil {
			t.Fatalf("%s must be invalid", s)
		}
	}
}

func TestOpenAINotInProdWithoutApproval(t *testing.T) {
	base(t)
	t.Setenv("APP_ENV", "prod")
	t.Setenv("TRYON_PROVIDER", "openai")
	t.Setenv("OPENAI_API_KEY", "sk-test")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OPENAI_PROD_APPROVED") {
		t.Fatalf("prod must not enable OpenAI automatically: %v", err)
	}
	t.Setenv("OPENAI_PROD_APPROVED", "true")
	if _, err := Load(); err != nil && strings.Contains(err.Error(), "OPENAI_PROD_APPROVED") {
		t.Fatalf("approval flag ignored: %v", err)
	}
}
