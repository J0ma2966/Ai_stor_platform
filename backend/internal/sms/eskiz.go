// Package sms — отправка SMS через узбекских провайдеров. Сейчас — Eskiz
// (notify.eskiz.uz). Реализует auth.SMSSender.
//
// Eskiz пропускает только тексты, прошедшие модерацию в кабинете: шаблон
// задаётся SMS_OTP_TEMPLATE и должен совпадать с одобренным (код подставляется
// вместо %s). В тестовом режиме аккаунта разрешён только текст «Bu Eskiz dan test».
package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const DefaultEskizURL = "https://notify.eskiz.uz/api"

type Eskiz struct {
	BaseURL  string // без завершающего «/», по умолчанию DefaultEskizURL
	Email    string
	Password string
	From     string // имя отправителя / номер, по умолчанию «4546»
	Template string // fmt-шаблон с одним %s для кода
	HTTP     *http.Client

	mu    sync.Mutex
	token string
}

var ErrSend = errors.New("sms: provider rejected message")

func (e *Eskiz) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (e *Eskiz) base() string {
	if e.BaseURL == "" {
		return DefaultEskizURL
	}
	return strings.TrimRight(e.BaseURL, "/")
}

// SendOTP отправляет код. При 401 токен перевыпускается один раз.
func (e *Eskiz) SendOTP(ctx context.Context, phone, code string) error {
	msg := e.Template // тестовый режим Eskiz: фиксированный текст без кода
	if strings.Contains(e.Template, "%s") {
		msg = fmt.Sprintf(e.Template, code)
	}
	return e.SendText(ctx, phone, msg)
}

// SendText отправляет произвольный текст. Eskiz пропустит его, только если текст
// совпадает с шаблоном, одобренным в кабинете (уведомления о заказах — docs/NOTIFICATIONS.md).
func (e *Eskiz) SendText(ctx context.Context, phone, msg string) error {
	tok, err := e.getToken(ctx, false)
	if err != nil {
		return err
	}
	status, body, err := e.send(ctx, tok, phone, msg)
	if err == nil && status == http.StatusUnauthorized {
		if tok, err = e.getToken(ctx, true); err != nil {
			return err
		}
		status, body, err = e.send(ctx, tok, phone, msg)
	}
	if err != nil {
		return err
	}
	if status/100 != 2 {
		// телефон и код в ошибку не попадают
		return fmt.Errorf("%w: http %d: %s", ErrSend, status, short(body))
	}
	var res struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if json.Unmarshal(body, &res) != nil || (res.ID == "" && res.Status == "") {
		return fmt.Errorf("%w: unexpected response: %s", ErrSend, short(body))
	}
	if s := strings.ToLower(res.Status); s == "error" || s == "rejected" {
		return fmt.Errorf("%w: status %s", ErrSend, res.Status)
	}
	return nil
}

func (e *Eskiz) send(ctx context.Context, tok, phone, msg string) (int, []byte, error) {
	from := e.From
	if from == "" {
		from = "4546"
	}
	form := url.Values{
		"mobile_phone": {strings.TrimPrefix(phone, "+")},
		"message":      {msg},
		"from":         {from},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.base()+"/message/sms/send", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	return e.do(req)
}

func (e *Eskiz) do(req *http.Request) (int, []byte, error) {
	resp, err := e.client().Do(req) // #nosec G704 -- адрес провайдера из конфигурации
	if err != nil {
		return 0, nil, fmt.Errorf("sms: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, body, err
}

// getToken возвращает кэшированный токен или логинится (force — принудительно).
func (e *Eskiz) getToken(ctx context.Context, force bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.token != "" && !force {
		return e.token, nil
	}
	form := url.Values{"email": {e.Email}, "password": {e.Password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.base()+"/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	status, body, err := e.do(req)
	if err != nil {
		return "", err
	}
	var res struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if status/100 != 2 || json.Unmarshal(body, &res) != nil || res.Data.Token == "" {
		return "", fmt.Errorf("sms: eskiz login failed: http %d", status)
	}
	e.token = res.Data.Token
	return e.token, nil
}

func short(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
