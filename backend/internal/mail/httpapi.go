package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

// HTTPAPI отправляет письма через почтовый сервис по HTTPS (порт 443). Нужен там,
// где хостинг закрывает исходящие SMTP-порты 25/465/587. Провайдеры: brevo, resend.
type HTTPAPI struct {
	Provider string // "brevo" | "resend"
	APIKey   string
	From     string // "Boutique <no-reply@shop.uz>"
	SiteName string
	Endpoint string // пусто — адрес провайдера; задаётся в тестах
	Client   *http.Client
}

var apiEndpoints = map[string]string{
	"brevo":  "https://api.brevo.com/v3/smtp/email",
	"resend": "https://api.resend.com/emails",
}

// KnownAPIProvider сообщает, умеет ли HTTPAPI работать с провайдером.
func KnownAPIProvider(p string) bool { _, ok := apiEndpoints[p]; return ok }

func (s *HTTPAPI) SendCode(ctx context.Context, to, code string, purpose Purpose, lang string) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("mail: bad MAIL_FROM: %w", err)
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil || strings.ContainsAny(to, "\r\n") {
		return errors.New("mail: bad recipient")
	}
	site := s.SiteName
	if site == "" {
		site = "Boutique"
	}
	name := from.Name
	if name == "" {
		name = site
	}
	subject, body := Compose(code, purpose, lang, site)

	var payload any
	hdr := http.Header{}
	switch s.Provider {
	case "brevo":
		payload = map[string]any{
			"sender":      map[string]string{"name": name, "email": from.Address},
			"to":          []map[string]string{{"email": rcpt.Address}},
			"subject":     subject,
			"textContent": body,
		}
		hdr.Set("api-key", s.APIKey)
	case "resend":
		payload = map[string]any{
			"from":    (&mail.Address{Name: name, Address: from.Address}).String(),
			"to":      []string{rcpt.Address},
			"subject": subject,
			"text":    body,
		}
		hdr.Set("Authorization", "Bearer "+s.APIKey)
	default:
		return fmt.Errorf("mail: unknown provider %q", s.Provider)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := s.Endpoint
	if endpoint == "" {
		endpoint = apiEndpoints[s.Provider]
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	hr.Header = hdr
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "application/json")
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(hr)
	if err != nil {
		return fmt.Errorf("mail: %s: %w", s.Provider, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		// текст ответа провайдера объясняет причину (неверный ключ, неподтверждённый отправитель)
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("mail: %s: status %d: %s", s.Provider, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
