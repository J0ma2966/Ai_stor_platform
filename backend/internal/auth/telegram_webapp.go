package auth

// Вход внутри Telegram Mini App (магазин открыт кнопкой в нашем боте).
//
// Telegram передаёт странице данные запуска (initData), подписанные токеном бота:
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app
// Сайт отправляет их как есть в POST /v1/auth/telegram/webapp {init_data}.
// Если этот Telegram-аккаунт уже привязан к покупателю (вход через бота
// делался раньше), выдаём обычную сессию. Если нет — 404 telegram_not_linked,
// и сайт предлагает обычный вход через бота (там человек делится номером).

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/ratelimit"
)

// TelegramWebAppMaxAge — сколько после открытия Mini App его данные годятся для входа.
const TelegramWebAppMaxAge = 24 * time.Hour

var (
	ruleTgWebAppIP = ratelimit.Rule{Name: "tg_webapp_ip", Limit: 60, Window: 15 * time.Minute}

	// ErrTelegramInitData — подпись не сошлась, данные устарели или испорчены.
	ErrTelegramInitData = errors.New("telegram init data invalid")
	// ErrTelegramNotLinked — такого Telegram-аккаунта у нас нет.
	ErrTelegramNotLinked = errors.New("telegram account not linked")
)

// VerifyTelegramInitData проверяет подпись initData и возвращает id пользователя Telegram.
func VerifyTelegramInitData(initData, botToken string, now time.Time) (int64, error) {
	if botToken == "" || initData == "" || len(initData) > 8192 {
		return 0, ErrTelegramInitData
	}
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return 0, ErrTelegramInitData
	}
	hash := vals.Get("hash")
	if len(hash) != 64 {
		return 0, ErrTelegramInitData
	}
	for _, v := range vals {
		if len(v) != 1 { // повторённый ключ — не Telegram
			return 0, ErrTelegramInitData
		}
	}
	want := telegramDataHash(vals, botToken)
	got, err := hex.DecodeString(hash)
	if err != nil || !hmac.Equal(got, want) {
		return 0, ErrTelegramInitData
	}
	ts, err := strconv.ParseInt(vals.Get("auth_date"), 10, 64)
	if err != nil {
		return 0, ErrTelegramInitData
	}
	at := time.Unix(ts, 0)
	if now.Sub(at) > TelegramWebAppMaxAge || at.Sub(now) > 5*time.Minute {
		return 0, ErrTelegramInitData
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(vals.Get("user")), &user); err != nil || user.ID <= 0 {
		return 0, ErrTelegramInitData
	}
	return user.ID, nil
}

type tgWebAppReq struct {
	InitData string `json:"init_data" validate:"required,max=8192"`
}

func (h *Handlers) telegramWebApp(w http.ResponseWriter, r *http.Request) {
	var req tgWebAppReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.Limiter.Check(w, r, ruleTgWebAppIP, httpx.ClientIP(r.Context())) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	sess, err := h.Svc.LoginTelegramWebApp(r.Context(), req.InitData, meta(r), time.Now())
	switch {
	case errors.Is(err, ErrTelegramInitData):
		httpx.Fail(w, r, httpx.NewError(http.StatusUnauthorized, "telegram_init_data_invalid", "telegram data is invalid or expired"))
		return
	case errors.Is(err, ErrTelegramNotLinked):
		httpx.Fail(w, r, httpx.NewError(http.StatusNotFound, "telegram_not_linked", "sign in through the bot first"))
		return
	}
	h.finish(w, r, sess, err)
}

// LoginTelegramWebApp — вход по данным запуска Mini App для уже привязанного аккаунта.
func (s *Service) LoginTelegramWebApp(ctx context.Context, initData string, meta ClientMeta, now time.Time) (*Session, error) {
	tgID, err := VerifyTelegramInitData(initData, s.TelegramBotToken, now)
	if err != nil {
		return nil, err
	}
	p := Principal{Role: RoleClient}
	var status string
	err = s.DB.QueryRow(ctx, `SELECT id, status FROM users WHERE telegram_user_id = $1 AND role = 'client' AND deleted_at IS NULL`,
		tgID).Scan(&p.UserID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTelegramNotLinked
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	return s.newSession(ctx, p, uuid.New(), meta)
}

// telegramDataHash — подпись Telegram: HMAC-SHA256 от строк «ключ=значение»
// (по алфавиту, без hash) с ключом HMAC-SHA256("WebAppData", токен бота).
func telegramDataHash(vals url.Values, botToken string) []byte {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + vals.Get(k)
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	m := hmac.New(sha256.New, secret.Sum(nil))
	m.Write([]byte(strings.Join(lines, "\n")))
	return m.Sum(nil)
}

// SignTelegramInitData подписывает данные так же, как Telegram (тесты и локальная проверка).
func SignTelegramInitData(vals url.Values, botToken string) string {
	out := url.Values{}
	for k, v := range vals {
		if k != "hash" {
			out[k] = v
		}
	}
	out.Set("hash", hex.EncodeToString(telegramDataHash(out, botToken)))
	return out.Encode()
}
