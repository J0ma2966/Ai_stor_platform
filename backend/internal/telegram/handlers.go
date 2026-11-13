package telegram

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/outbox"
	"github.com/boutique-market/backend/internal/platform/db"
)

// LinkTTL — сколько действует ссылка подключения бота.
const LinkTTL = 15 * time.Minute

// Confirmer — подтверждение заказа магазином (orders.Handlers.ConfirmByStore).
type Confirmer interface {
	ConfirmByStore(ctx context.Context, tx pgx.Tx, p auth.Principal, id uuid.UUID) error
}

// ConfirmPrefix — callback-данные кнопки «Подтвердить» (orders.CallbackConfirm).
const ConfirmPrefix = "confirm:"

// LoginService — вход клиентов через бота (auth.Service, auth/telegram_login.go).
type LoginService interface {
	TelegramLoginOpened(ctx context.Context, id string, tgUserID int64) (auth.TelegramLoginState, error)
	TelegramLoginPendingFor(ctx context.Context, tgUserID int64) (string, error)
	ConfirmTelegramLogin(ctx context.Context, id string, tgUserID int64, phone string) (auth.TelegramLoginState, error)
}

// LoginPrefix — callback-данные кнопки «Войти».
const LoginPrefix = "login:"

type Handlers struct {
	DB          *pgxpool.Pool
	Login       LoginService // nil — вход клиентов через бота выключен
	Bot         *Bot         // nil — бот не настроен
	BotUsername string       // без @
	Secret      string       // TELEGRAM_WEBHOOK_SECRET
	Orders      Confirmer
}

func (h *Handlers) enabled() bool { return h.Bot != nil && h.BotUsername != "" }

// StoreRoutes монтируются под /v1/store/telegram (только роль store).
func (h *Handlers) StoreRoutes(r chi.Router) {
	r.Get("/", h.status)
	r.Post("/link", h.link)
	r.Delete("/", h.unlink)
}

func (h *Handlers) status(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	var chat *int64
	if err := h.DB.QueryRow(r.Context(), `SELECT telegram_chat_id FROM stores WHERE id = $1`, p.StoreID).Scan(&chat); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{"available": h.enabled(), "connected": chat != nil, "bot_username": h.BotUsername})
}

// link выдаёт одноразовую ссылку t.me/<bot>?start=<код> на 15 минут.
func (h *Handlers) link(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		httpx.Fail(w, r, httpx.NewError(http.StatusServiceUnavailable, "telegram_disabled", "telegram bot is not configured"))
		return
	}
	p := auth.MustFrom(r.Context())
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	code := base64.RawURLEncoding.EncodeToString(raw) // 24 символа [A-Za-z0-9_-] — допустимо для start
	exp := time.Now().Add(LinkTTL)
	if _, err := h.DB.Exec(r.Context(), `UPDATE stores SET telegram_link_code = $2, telegram_link_expires_at = $3 WHERE id = $1`,
		p.StoreID, code, exp); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{"url": "https://t.me/" + h.BotUsername + "?start=" + code, "expires_at": exp})
}

func (h *Handlers) unlink(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	if _, err := h.DB.Exec(r.Context(), `UPDATE stores SET telegram_chat_id = NULL, telegram_link_code = NULL WHERE id = $1`, p.StoreID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// UnlinkChat отвязывает чат от всех магазинов (бот заблокирован или чат удалён).
func UnlinkChat(ctx context.Context, pool *pgxpool.Pool, chatID int64) error {
	_, err := pool.Exec(ctx, `UPDATE stores SET telegram_chat_id = NULL WHERE telegram_chat_id = $1`, chatID)
	return err
}

// ---------- вебхук ----------

type tgUser struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	LanguageCode string `json:"language_code"`
}

type message struct {
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	From    *tgUser `json:"from"`
	Text    string  `json:"text"`
	Contact *struct {
		PhoneNumber string `json:"phone_number"`
		UserID      int64  `json:"user_id"`
	} `json:"contact"`
}

type update struct {
	Message       *message `json:"message"`
	CallbackQuery *struct {
		ID      string  `json:"id"`
		From    *tgUser `json:"from"`
		Data    string  `json:"data"`
		Message *struct {
			MessageID int64  `json:"message_id"`
			Text      string `json:"text"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

// Webhook: POST /v1/telegram/webhook. Telegram повторяет доставку, пока не получит
// 2xx, поэтому на ошибки обработки отвечаем 200 и пишем в лог.
func (h *Handlers) Webhook(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() || h.Secret == "" ||
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")), []byte(h.Secret)) != 1 {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadRequest, "bad_request", "cannot read body"))
		return
	}
	var u update
	if json.Unmarshal(body, &u) != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	ctx := r.Context()
	log := httpx.LoggerFrom(ctx)
	switch {
	case u.Message != nil && h.isLoginMessage(u.Message):
		if err := h.onLoginMessage(ctx, u.Message); err != nil {
			log.Warn("telegram login", "err", err.Error())
		}
	case u.Message != nil:
		if err := h.onMessage(ctx, u.Message.Chat.ID, u.Message.Text); err != nil {
			log.Warn("telegram message", "err", err.Error())
		}
	case u.CallbackQuery != nil && u.CallbackQuery.Message != nil && strings.HasPrefix(u.CallbackQuery.Data, LoginPrefix):
		q := u.CallbackQuery
		if err := h.onLoginCallback(ctx, q.ID, q.From, q.Message.Chat.ID, q.Message.MessageID, strings.TrimPrefix(q.Data, LoginPrefix)); err != nil {
			log.Warn("telegram login callback", "err", err.Error())
		}
	case u.CallbackQuery != nil && u.CallbackQuery.Message != nil:
		q := u.CallbackQuery
		if err := h.onCallback(ctx, q.ID, q.Message.Chat.ID, q.Message.MessageID, q.Message.Text, q.Data); err != nil {
			log.Warn("telegram callback", "err", err.Error())
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handlers) reply(ctx context.Context, chatID int64, text string) error {
	return outbox.ToTelegramChat(ctx, h.DB, chatID, outbox.Message{Text: text})
}

func (h *Handlers) onMessage(ctx context.Context, chatID int64, text string) error {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/start") {
		return h.reply(ctx, chatID, "Через этого бота можно войти на сайт и в приложение: нажмите «Войти через Telegram» "+
			"на странице входа. Бутикам бот присылает уведомления о заказах (кабинет магазина → Профиль → Telegram).")
	}
	code := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
	if code == "" || len(code) > 64 {
		return h.reply(ctx, chatID, "Откройте ссылку подключения из кабинета магазина (Профиль → Telegram).")
	}
	var name string
	err := h.DB.QueryRow(ctx, `UPDATE stores SET telegram_chat_id = $2, telegram_link_code = NULL, telegram_link_expires_at = NULL
		WHERE telegram_link_code = $1 AND telegram_link_expires_at > now() RETURNING name`, code, chatID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return h.reply(ctx, chatID, "Ссылка устарела. Получите новую в кабинете магазина (Профиль → Telegram).")
	}
	if err != nil {
		return err
	}
	return h.reply(ctx, chatID, "Готово! Бутик «"+name+"» подключён. Сюда будут приходить новые заказы, "+
		"их можно подтверждать кнопкой прямо в Telegram.")
}

func (h *Handlers) onCallback(ctx context.Context, callbackID string, chatID, messageID int64, msgText, data string) error {
	idStr, ok := strings.CutPrefix(data, ConfirmPrefix)
	orderID, err := uuid.Parse(idStr)
	if !ok || err != nil {
		return h.Bot.Answer(ctx, callbackID, "Неизвестная команда")
	}
	answer := "Заказ подтверждён"
	done := false
	err = db.Tx(ctx, h.DB, func(tx pgx.Tx) error {
		// заказ должен принадлежать магазину, к которому привязан этот чат
		var p auth.Principal
		err := tx.QueryRow(ctx, `SELECT u.id, u.store_id FROM orders o
			JOIN stores s ON s.id = o.store_id AND s.telegram_chat_id = $2 AND s.status = 'active'
			JOIN users u ON u.store_id = s.id AND u.role = 'store' AND u.status = 'active'
			WHERE o.id = $1 LIMIT 1`, orderID, chatID).Scan(&p.UserID, &p.StoreID)
		if errors.Is(err, pgx.ErrNoRows) {
			answer = "Заказ не найден"
			return nil
		}
		if err != nil {
			return err
		}
		p.Role = auth.RoleStore
		err = h.Orders.ConfirmByStore(ctx, tx, p, orderID)
		var he *httpx.Error
		if errors.As(err, &he) && (he.Code == "status_changed" || he.Code == "invalid_transition") {
			answer = "Статус заказа уже изменён на сайте"
			done = true
			return nil
		}
		if err == nil {
			done = true
		}
		return err
	})
	if err != nil {
		_ = h.Bot.Answer(ctx, callbackID, "Не получилось, попробуйте на сайте")
		return err
	}
	_ = h.Bot.Answer(ctx, callbackID, answer)
	if done {
		return h.Bot.MarkDone(ctx, chatID, messageID, msgText+"\n\n✅ "+answer, nil)
	}
	return nil
}
