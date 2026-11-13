package auth

// Вход через нашего Telegram-бота (docs/AUTH.md). Работает одинаково на сайте и в
// приложении, без виджетов и сторонних скриптов:
//
//  1. POST /v1/auth/telegram/start → {login_id, poll_token, url, expires_at}
//     url = https://t.me/<bot>?start=login_<login_id>; poll_token знает только
//     устройство, которое начало вход.
//  2. Клиент открывает ссылку, Telegram присылает боту «/start login_<id>».
//     Знакомому пользователю бот показывает кнопку «Войти», новому — кнопку
//     «Поделиться номером» (Telegram отдаёт номер, привязанный к аккаунту).
//     Нажатие подтверждает вход (пакет telegram, вебхук).
//  3. Устройство опрашивает POST /v1/auth/telegram/poll {login_id, poll_token}
//     и после подтверждения получает обычную сессию (одноразово).
//
// Защита от подсунутой ссылки: вход подтверждается только явным нажатием в боте,
// а бот пишет, что нажимать нужно, только если вы сами начали вход на сайте.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/ratelimit"
)

// TelegramLoginTTL — сколько живёт попытка входа.
const TelegramLoginTTL = 5 * time.Minute

// TelegramStartPrefix — префикс параметра /start у ссылки входа (ссылки
// подключения магазинов — 24 символа без префикса).
const TelegramStartPrefix = "login_"

var (
	ruleTgStartIP = ratelimit.Rule{Name: "tg_start_ip", Limit: 30, Window: 15 * time.Minute}
	// опрос раз в 2 секунды × 5 минут = 150 запросов на одну попытку
	ruleTgPollIP = ratelimit.Rule{Name: "tg_poll_ip", Limit: 600, Window: 15 * time.Minute}

	// ErrTelegramLoginExpired — попытки нет или она истекла.
	ErrTelegramLoginExpired = errors.New("telegram login expired")
)

func tgKey(id string) string { return "tglogin:" + id }

func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// TelegramRoutes монтируется в /v1/auth/telegram (только если бот настроен).
func (h *Handlers) TelegramRoutes(r chi.Router) {
	r.Post("/start", h.telegramStart)
	r.Post("/poll", h.telegramPoll)
}

func (h *Handlers) telegramStart(w http.ResponseWriter, r *http.Request) {
	if !h.Limiter.Check(w, r, ruleTgStartIP, httpx.ClientIP(r.Context())) {
		return
	}
	id, err := randToken(16) // 22 символа [A-Za-z0-9_-]
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	poll, err := randToken(32)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	key := tgKey(id)
	pipe := h.Svc.Redis.TxPipeline()
	pipe.HSet(ctx, key, "p", hashHex(poll), "st", "pending")
	pipe.Expire(ctx, key, TelegramLoginTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"login_id":     id,
		"poll_token":   poll,
		"url":          "https://t.me/" + h.Svc.TelegramBot + "?start=" + TelegramStartPrefix + id,
		"bot_username": h.Svc.TelegramBot,
		"expires_at":   time.Now().Add(TelegramLoginTTL),
	})
}

type tgPollReq struct {
	LoginID   string `json:"login_id" validate:"required,max=64"`
	PollToken string `json:"poll_token" validate:"required,max=128"`
}

func (h *Handlers) telegramPoll(w http.ResponseWriter, r *http.Request) {
	var req tgPollReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !h.Limiter.Check(w, r, ruleTgPollIP, httpx.ClientIP(r.Context())) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	sess, err := h.Svc.PollTelegramLogin(r.Context(), req.LoginID, req.PollToken, meta(r))
	switch {
	case errors.Is(err, ErrTelegramLoginExpired):
		httpx.Fail(w, r, httpx.NewError(http.StatusGone, "telegram_login_expired", "login link expired, start again"))
		return
	case err == nil && sess == nil:
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "pending"})
		return
	}
	h.finish(w, r, sess, err)
}

// PollTelegramLogin возвращает nil, nil, пока вход не подтверждён в боте.
func (s *Service) PollTelegramLogin(ctx context.Context, id, poll string, meta ClientMeta) (*Session, error) {
	key := tgKey(id)
	v, err := s.Redis.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	if len(v) == 0 || subtle.ConstantTimeCompare([]byte(v["p"]), []byte(hashHex(poll))) != 1 {
		return nil, ErrTelegramLoginExpired
	}
	switch v["st"] {
	case "pending":
		return nil, nil
	case "blocked":
		s.Redis.Del(ctx, key)
		return nil, ErrAccountBlocked
	case "ok":
	default:
		return nil, ErrTelegramLoginExpired
	}
	// Del возвращает 1 только одному из параллельных опросов — сессия выдаётся один раз.
	if n, err := s.Redis.Del(ctx, key).Result(); err != nil || n != 1 {
		return nil, ErrTelegramLoginExpired
	}
	uid, err := uuid.Parse(v["uid"])
	if err != nil {
		return nil, ErrTelegramLoginExpired
	}
	var status string
	if err := s.DB.QueryRow(ctx, `SELECT status FROM users WHERE id = $1 AND role = 'client' AND deleted_at IS NULL`, uid).Scan(&status); err != nil {
		return nil, ErrTelegramLoginExpired
	}
	if status != "active" {
		return nil, ErrAccountBlocked
	}
	sess, err := s.newSession(ctx, Principal{UserID: uid, Role: RoleClient}, uuid.New(), meta)
	if err != nil {
		return nil, err
	}
	sess.IsNew = v["new"] == "1"
	return sess, nil
}

// ---------- вызывается вебхуком бота (пакет telegram) ----------

// TelegramLoginState — что бот должен показать по ссылке входа.
type TelegramLoginState int

const (
	TgLoginExpired TelegramLoginState = iota // ссылка устарела или уже использована
	TgLoginKnown                             // пользователь уже есть: кнопка «Войти»
	TgLoginNew                               // новый пользователь: кнопка «Поделиться номером»
)

var tgConfirmScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'st') ~= 'pending' then return 0 end
redis.call('HSET', KEYS[1], 'st', ARGV[1], 'uid', ARGV[2], 'new', ARGV[3])
return 1
`)

// TelegramLoginOpened — пользователь Telegram открыл ссылку входа.
func (s *Service) TelegramLoginOpened(ctx context.Context, id string, tgUserID int64) (TelegramLoginState, error) {
	st, err := s.Redis.HGet(ctx, tgKey(id), "st").Result()
	if errors.Is(err, redis.Nil) || (err == nil && st != "pending") {
		return TgLoginExpired, nil
	}
	if err != nil {
		return TgLoginExpired, err
	}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE telegram_user_id = $1 AND role = 'client' AND deleted_at IS NULL)`,
		tgUserID).Scan(&exists); err != nil {
		return TgLoginExpired, err
	}
	// какую попытку подтверждает присланный номер (контакт приходит без параметра)
	if err := s.Redis.Set(ctx, "tglogin:chat:"+strconv.FormatInt(tgUserID, 10), id, TelegramLoginTTL).Err(); err != nil {
		return TgLoginExpired, err
	}
	if exists {
		return TgLoginKnown, nil
	}
	return TgLoginNew, nil
}

// TelegramLoginPendingFor — последняя открытая этим пользователем попытка входа.
func (s *Service) TelegramLoginPendingFor(ctx context.Context, tgUserID int64) (string, error) {
	id, err := s.Redis.Get(ctx, "tglogin:chat:"+strconv.FormatInt(tgUserID, 10)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return id, err
}

// ConfirmTelegramLogin подтверждает попытку входа. phone — номер, которым
// пользователь поделился в боте (пусто для кнопки «Войти» у знакомого пользователя).
// Возвращает TgLoginNew, если нужен номер, а его нет.
func (s *Service) ConfirmTelegramLogin(ctx context.Context, id string, tgUserID int64, phone string) (TelegramLoginState, error) {
	key := tgKey(id)
	st, err := s.Redis.HGet(ctx, key, "st").Result()
	if errors.Is(err, redis.Nil) || (err == nil && st != "pending") {
		return TgLoginExpired, nil
	}
	if err != nil {
		return TgLoginExpired, err
	}
	var (
		uid    uuid.UUID
		status string
		isNew  bool
		need   bool
	)
	err = db.Tx(ctx, s.DB, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id, status FROM users WHERE telegram_user_id = $1 AND role = 'client' AND deleted_at IS NULL`,
			tgUserID).Scan(&uid, &status)
		if err == nil {
			if phone != "" {
				// номер из Telegram подтверждён — дополняем профиль, если номера не было
				_, err = tx.Exec(ctx, `UPDATE users SET phone = $2 WHERE id = $1 AND phone IS NULL
					AND NOT EXISTS (SELECT 1 FROM users WHERE phone = $2)`, uid, phone)
			}
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if phone == "" {
			need = true
			return nil
		}
		var role Role
		err = tx.QueryRow(ctx, `SELECT id, role, status FROM users WHERE phone = $1 FOR UPDATE`, phone).Scan(&uid, &role, &status)
		if err == nil {
			if role != RoleClient {
				return ErrBadCredentials // номер сотрудника не даёт клиентский вход
			}
			// клиент с этим номером уже есть (старый вход по SMS) — привязываем Telegram
			_, err = tx.Exec(ctx, `UPDATE users SET telegram_user_id = NULL WHERE telegram_user_id = $1`, tgUserID)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE users SET telegram_user_id = $2 WHERE id = $1`, uid, tgUserID)
			}
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		isNew, status = true, "active"
		return tx.QueryRow(ctx, `INSERT INTO users (role, phone, telegram_user_id) VALUES ('client', $1, $2) RETURNING id`,
			phone, tgUserID).Scan(&uid)
	})
	if err != nil {
		return TgLoginExpired, err
	}
	if need {
		return TgLoginNew, nil
	}
	newFlag := "0"
	if isNew {
		newFlag = "1"
	}
	st2 := "ok"
	if status != "active" {
		st2 = "blocked"
	}
	// попытка могла истечь или быть подтверждена параллельно: меняем только ожидающую
	ok, err := tgConfirmScript.Run(ctx, s.Redis, []string{key}, st2, uid.String(), newFlag).Int()
	if err != nil || ok != 1 {
		return TgLoginExpired, err
	}
	s.Redis.Del(ctx, "tglogin:chat:"+strconv.FormatInt(tgUserID, 10))
	if st2 == "blocked" {
		return TgLoginExpired, ErrAccountBlocked
	}
	return TgLoginKnown, nil
}

// NormalizeTelegramPhone приводит номер из контакта Telegram к E.164 (+998…).
func NormalizeTelegramPhone(p string) string {
	p = strings.TrimSpace(p)
	digits := strings.TrimPrefix(p, "+")
	if len(digits) < 8 || len(digits) > 15 || strings.Trim(digits, "0123456789") != "" {
		return ""
	}
	return "+" + digits
}
