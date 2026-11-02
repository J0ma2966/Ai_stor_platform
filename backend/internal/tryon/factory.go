package tryon

import (
	"fmt"
	"net/http"

	"github.com/boutique-market/backend/internal/config"
)

// NewProvider создаёт провайдера по конфигурации (TRYON_PROVIDER).
func NewProvider(c config.TryOnConfig) (Provider, error) {
	hc := &http.Client{Timeout: c.Timeout}
	switch c.Provider {
	case "mock", "":
		return Mock{}, nil
	case "fashn":
		return &Fashn{APIKey: c.APIKey, BaseURL: c.Endpoint, HTTP: hc}, nil
	case "gemini":
		return &Gemini{APIKey: c.APIKey, Model: c.Model, BaseURL: c.Endpoint, HTTP: hc}, nil
	case "http":
		return &SelfHosted{Endpoint: c.Endpoint, APIKey: c.APIKey, HTTP: hc}, nil
	}
	return nil, fmt.Errorf("unknown tryon provider %q", c.Provider)
}
