package tryon

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/boutique-market/backend/internal/config"
)

// NewProvider создаёт провайдера по конфигурации (TRYON_PROVIDER).
// Для "off" возвращает nil: примерка выключена, воркер не запускается.
func NewProvider(c config.TryOnConfig) (Provider, error) {
	if c.Provider == "off" {
		return nil, nil
	}
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
	case "openai":
		o := c.OpenAI
		if o.APIKey == "" {
			// config.Load уже не пускает такую конфигурацию; защита на случай прямого вызова
			return nil, errors.New("tryon: OPENAI_API_KEY is required for TRYON_PROVIDER=openai")
		}
		return &OpenAI{APIKey: o.APIKey, BaseURL: o.BaseURL, Model: o.Model, Quality: o.Quality,
			Size: o.Size, Format: o.Format, HTTP: &http.Client{Timeout: o.Timeout}}, nil
	}
	return nil, fmt.Errorf("unknown tryon provider %q", c.Provider)
}
