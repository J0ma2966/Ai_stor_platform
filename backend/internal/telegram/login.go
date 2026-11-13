package telegram

// Вход клиентов через бота: «/start login_<id>» → кнопка «Войти» (знакомый
// пользователь) или «Поделиться номером» (новый). Логика аккаунтов — в
// auth/telegram_login.go, здесь только диалог в чате.

import (
	"context"
	"errors"
	"strings"

	"github.com/boutique-market/backend/internal/auth"
)

type loginTexts struct {
	expired, known, knownBtn, newUser, shareBtn, done, notYours, staff, blocked, needPhone string
}

var loginRU = loginTexts{
	expired:  "Ссылка для входа устарела. Нажмите «Войти через Telegram» на сайте или в приложении ещё раз.",
	known:    "Подтвердите вход в Boutique.\n\nНажимайте, только если вы сами сейчас входите на сайт или в приложение. Если нет — просто проигнорируйте это сообщение.",
	knownBtn: "✅ Войти",
	newUser: "Добро пожаловать в Boutique!\n\nЧтобы зарегистрироваться, поделитесь номером телефона кнопкой ниже: " +
		"он нужен для доставки заказов. Делайте это, только если вы сами сейчас входите на сайт или в приложение.",
	shareBtn:  "📱 Поделиться номером",
	done:      "Готово! Вернитесь на сайт или в приложение — вход выполнится сам.",
	notYours:  "Отправьте свой номер кнопкой «Поделиться номером» под полем ввода.",
	staff:     "Этот номер принадлежит сотруднику магазина. Для покупок используйте другой аккаунт.",
	blocked:   "Аккаунт заблокирован. Напишите в поддержку магазина.",
	needPhone: "Сначала поделитесь номером телефона кнопкой под полем ввода.",
}

var loginUZ = loginTexts{
	expired:  "Kirish havolasi eskirgan. Sayt yoki ilovada «Telegram orqali kirish» tugmasini qayta bosing.",
	known:    "Boutique'ga kirishni tasdiqlang.\n\nFaqat hozir o'zingiz saytga yoki ilovaga kirayotgan bo'lsangiz bosing. Aks holda bu xabarni e'tiborsiz qoldiring.",
	knownBtn: "✅ Kirish",
	newUser: "Boutique'ga xush kelibsiz!\n\nRo'yxatdan o'tish uchun quyidagi tugma orqali telefon raqamingizni yuboring: " +
		"u buyurtmalarni yetkazish uchun kerak. Faqat hozir o'zingiz saytga yoki ilovaga kirayotgan bo'lsangiz yuboring.",
	shareBtn:  "📱 Raqamni yuborish",
	done:      "Tayyor! Sayt yoki ilovaga qayting — kirish avtomatik bajariladi.",
	notYours:  "O'z raqamingizni maydon ostidagi «Raqamni yuborish» tugmasi orqali yuboring.",
	staff:     "Bu raqam do'kon xodimiga tegishli. Xaridlar uchun boshqa akkauntdan foydalaning.",
	blocked:   "Akkaunt bloklangan. Do'kon qo'llab-quvvatlash xizmatiga yozing.",
	needPhone: "Avval maydon ostidagi tugma orqali telefon raqamingizni yuboring.",
}

func texts(u *tgUser) loginTexts {
	if u != nil && strings.HasPrefix(u.LanguageCode, "uz") {
		return loginUZ
	}
	return loginRU
}

func loginStartID(text string) (string, bool) {
	arg, ok := strings.CutPrefix(strings.TrimSpace(text), "/start ")
	if !ok {
		return "", false
	}
	id, ok := strings.CutPrefix(strings.TrimSpace(arg), auth.TelegramStartPrefix)
	return id, ok && id != "" && len(id) <= 43
}

// isLoginMessage — сообщение относится ко входу клиента: ссылка входа или контакт.
func (h *Handlers) isLoginMessage(m *message) bool {
	if h.Login == nil || m.Chat.Type != "private" || m.From == nil || m.From.IsBot {
		return false
	}
	if m.Contact != nil {
		return true
	}
	_, ok := loginStartID(m.Text)
	return ok
}

func (h *Handlers) onLoginMessage(ctx context.Context, m *message) error {
	t := texts(m.From)
	chat := m.Chat.ID
	if m.Contact != nil {
		// Telegram ставит user_id только у собственного контакта отправителя.
		if m.Contact.UserID != m.From.ID {
			return h.send(ctx, chat, t.notYours, nil)
		}
		id, err := h.Login.TelegramLoginPendingFor(ctx, m.From.ID)
		if err != nil {
			return err
		}
		if id == "" {
			return h.send(ctx, chat, t.expired, removeKeyboard())
		}
		return h.confirm(ctx, chat, t, id, m.From.ID, auth.NormalizeTelegramPhone(m.Contact.PhoneNumber))
	}
	id, _ := loginStartID(m.Text)
	st, err := h.Login.TelegramLoginOpened(ctx, id, m.From.ID)
	if err != nil {
		return err
	}
	switch st {
	case auth.TgLoginKnown:
		return h.send(ctx, chat, t.known, map[string]any{
			"inline_keyboard": [][]inlineButton{{{Text: t.knownBtn, CallbackData: LoginPrefix + id}}},
		})
	case auth.TgLoginNew:
		return h.send(ctx, chat, t.newUser, shareKeyboard(t.shareBtn))
	default:
		return h.send(ctx, chat, t.expired, nil)
	}
}

func (h *Handlers) onLoginCallback(ctx context.Context, callbackID string, from *tgUser, chat, messageID int64, id string) error {
	if from == nil || h.Login == nil {
		return nil
	}
	t := texts(from)
	st, err := h.Login.ConfirmTelegramLogin(ctx, id, from.ID, "")
	switch {
	case errors.Is(err, auth.ErrAccountBlocked):
		_ = h.Bot.Answer(ctx, callbackID, t.blocked)
		return nil
	case err != nil:
		_ = h.Bot.Answer(ctx, callbackID, t.expired)
		return err
	case st == auth.TgLoginNew:
		_ = h.Bot.Answer(ctx, callbackID, t.needPhone)
		return h.send(ctx, chat, t.newUser, shareKeyboard(t.shareBtn))
	case st == auth.TgLoginExpired:
		_ = h.Bot.Answer(ctx, callbackID, t.expired)
		return h.Bot.MarkDone(ctx, chat, messageID, t.expired, nil)
	}
	_ = h.Bot.Answer(ctx, callbackID, "✅")
	return h.Bot.MarkDone(ctx, chat, messageID, t.done, nil)
}

func (h *Handlers) confirm(ctx context.Context, chat int64, t loginTexts, id string, tgUserID int64, phone string) error {
	if phone == "" {
		return h.send(ctx, chat, t.notYours, nil)
	}
	st, err := h.Login.ConfirmTelegramLogin(ctx, id, tgUserID, phone)
	switch {
	case errors.Is(err, auth.ErrBadCredentials):
		return h.send(ctx, chat, t.staff, removeKeyboard())
	case errors.Is(err, auth.ErrAccountBlocked):
		return h.send(ctx, chat, t.blocked, removeKeyboard())
	case err != nil:
		return err
	case st == auth.TgLoginExpired:
		return h.send(ctx, chat, t.expired, removeKeyboard())
	}
	return h.send(ctx, chat, t.done, removeKeyboard())
}

func shareKeyboard(label string) map[string]any {
	return map[string]any{
		"keyboard":          [][]map[string]any{{{"text": label, "request_contact": true}}},
		"resize_keyboard":   true,
		"one_time_keyboard": true,
	}
}

func removeKeyboard() map[string]any { return map[string]any{"remove_keyboard": true} }

// send — сразу через Bot API (не через очередь): ответ нужен, пока человек смотрит на чат.
func (h *Handlers) send(ctx context.Context, chat int64, text string, markup any) error {
	body := map[string]any{"chat_id": chat, "text": text, "link_preview_options": map[string]bool{"is_disabled": true}}
	if markup != nil {
		body["reply_markup"] = markup
	}
	_, err := h.Bot.call(ctx, "sendMessage", body)
	return err
}
