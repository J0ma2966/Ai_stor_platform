package config

import (
	"strings"
	"testing"
)

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
