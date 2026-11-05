// Package telegram — бот магазина: уведомления о заказах и кнопка «Подтвердить».
//
// Подключение: магазин в кабинете получает ссылку t.me/<bot>?start=<код>,
// открывает её, Telegram присылает боту «/start <код>» — чат привязывается к
// магазину. Telegram доставляет обновления на вебхук
// POST /v1/telegram/webhook с секретом в заголовке X-Telegram-Bot-Api-Secret-Token.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/boutique-market/backend/internal/outbox"
)

const DefaultAPI = "https://api.telegram.org"

type Bot struct {
	Token   string
	BaseURL string // по умолчанию DefaultAPI (тесты подменяют)
	HTTP    *http.Client
}

func (b *Bot) client() *http.Client {
	if b.HTTP != nil {
		return b.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (b *Bot) base() string {
	if b.BaseURL == "" {
		return DefaultAPI
	}
	return strings.TrimRight(b.BaseURL, "/")
}

type apiResp struct {
	OK          bool            `json:"ok"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
	Result      json.RawMessage `json:"result"`
}

// call вызывает метод Bot API. Ошибки 400/403 (бот заблокирован, чат не найден)
// помечаются outbox.ErrPermanent — повторять их бесполезно.
func (b *Bot) call(ctx context.Context, method string, body any) (json.RawMessage, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base()+"/bot"+b.Token+"/"+method, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client().Do(req) // #nosec G704 -- адрес Bot API из конфигурации
	if err != nil {
		// в тексте ошибки net/http есть URL с токеном — не выносим его наружу
		return nil, fmt.Errorf("telegram %s: request failed", method)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var r apiResp
	if json.Unmarshal(raw, &r) != nil {
		return nil, fmt.Errorf("telegram %s: http %d", method, resp.StatusCode)
	}
	if !r.OK {
		err := fmt.Errorf("telegram %s: %d %s", method, r.ErrorCode, r.Description)
		if r.ErrorCode == 403 || (r.ErrorCode == 400 && strings.Contains(r.Description, "chat not found")) {
			return nil, errors.Join(outbox.ErrPermanent, err)
		}
		return nil, err
	}
	return r.Result, nil
}

type inlineButton struct {
	Text         string `json:"text"`
	URL          string `json:"url,omitempty"`
	CallbackData string `json:"callback_data,omitempty"`
}

type replyMarkup struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

func markup(m outbox.Message) *replyMarkup {
	if len(m.Buttons) == 0 {
		return nil
	}
	rm := &replyMarkup{}
	for _, row := range m.Buttons {
		var out []inlineButton
		for _, b := range row {
			out = append(out, inlineButton{Text: b.Text, URL: b.URL, CallbackData: b.Data})
		}
		rm.InlineKeyboard = append(rm.InlineKeyboard, out)
	}
	return rm
}

// Send реализует outbox.TelegramSender. Текст без разметки (parse_mode не задан),
// поэтому названия товаров не нужно экранировать.
func (b *Bot) Send(ctx context.Context, chatID int64, m outbox.Message) error {
	body := map[string]any{"chat_id": chatID, "text": m.Text, "link_preview_options": map[string]bool{"is_disabled": true}}
	if rm := markup(m); rm != nil {
		body["reply_markup"] = rm
	}
	_, err := b.call(ctx, "sendMessage", body)
	return err
}

// Answer отвечает на нажатие кнопки (всплывающая подсказка у пользователя).
func (b *Bot) Answer(ctx context.Context, callbackID, text string) error {
	_, err := b.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": callbackID, "text": text})
	return err
}

// MarkDone дописывает к сообщению строку итога и убирает кнопку «Подтвердить».
func (b *Bot) MarkDone(ctx context.Context, chatID, messageID int64, text string, keep []outbox.Button) error {
	body := map[string]any{"chat_id": chatID, "message_id": messageID, "text": text,
		"link_preview_options": map[string]bool{"is_disabled": true}}
	if len(keep) > 0 {
		body["reply_markup"] = markup(outbox.Message{Buttons: [][]outbox.Button{keep}})
	}
	_, err := b.call(ctx, "editMessageText", body)
	return err
}

// SetWebhook регистрирует вебхук (вызывается при старте API, если задан TELEGRAM_WEBHOOK_URL).
func (b *Bot) SetWebhook(ctx context.Context, url, secret string) error {
	_, err := b.call(ctx, "setWebhook", map[string]any{
		"url": url, "secret_token": secret, "allowed_updates": []string{"message", "callback_query"},
		"drop_pending_updates": false,
	})
	return err
}
