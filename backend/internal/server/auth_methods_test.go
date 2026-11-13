package server

// Способы входа клиента: почта с кодом, Google, Telegram-бот; SMS выключается флагом.

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/mail"
	"github.com/boutique-market/backend/internal/telegram"
)

type captureMail struct {
	mu    sync.Mutex
	codes map[string]string
	sent  int
}

func (c *captureMail) SendCode(_ context.Context, to, code string, purpose mail.Purpose, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.codes[string(purpose)+":"+to] = code
	c.sent++
	return nil
}

func (c *captureMail) last(purpose, to string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.codes[purpose+":"+to]
}

func TestAuthMethods(t *testing.T) {
	e := setup(t)
	m := e.client().must(200, "GET", "/v1/auth/methods", nil).data()
	if m["email"] != true || m["sms"] != true || m["google"] != nil || m["telegram"] != nil {
		t.Fatalf("methods: %v", m)
	}
	// SMS выключен — маршрутов входа по номеру нет совсем.
	e = setup(t, func(d *Deps) { d.Cfg.SMSAuthEnabled = false; d.Cfg.EmailAuthEnabled = false })
	c := e.client()
	if m := c.must(200, "GET", "/v1/auth/methods", nil).data(); m["sms"] != false || m["email"] != false {
		t.Fatalf("methods: %v", m)
	}
	for _, p := range []string{"/v1/auth/otp/request", "/v1/auth/otp/verify", "/v1/auth/password/login", "/v1/auth/password/reset",
		"/v1/auth/email/code", "/v1/auth/email/login", "/v1/auth/google/", "/v1/auth/telegram/start"} {
		if r := c.do("POST", p, map[string]any{}); r.Status != 404 && r.Status != 405 {
			t.Errorf("%s must be off: %d %s", p, r.Status, r.Raw)
		}
	}
}

func TestEmailAuth_CodeRegisterLoginReset(t *testing.T) {
	e := setup(t)
	c := e.client()
	const addr = "tester@test.uz"

	// без кода из письма зарегистрироваться нельзя
	if r := c.do("POST", "/v1/auth/email/register", map[string]any{"email": addr, "code": "123456", "password": "long-enough-pw"}); r.Status != 401 || r.errCode() != "otp_invalid" {
		t.Fatalf("register without code: %d %s", r.Status, r.Raw)
	}
	c.must(202, "POST", "/v1/auth/email/code", map[string]any{"email": "Tester@Test.UZ", "purpose": "register", "lang": "uz"})
	if r := c.do("POST", "/v1/auth/email/code", map[string]any{"email": addr, "purpose": "register"}); r.Status != 429 || r.errCode() != "otp_cooldown" {
		t.Fatalf("cooldown: %d %s", r.Status, r.Raw)
	}
	code := e.mail.last("register", addr)
	if len(code) != 6 {
		t.Fatalf("code not sent: %q", code)
	}
	if r := c.do("POST", "/v1/auth/email/register", map[string]any{"email": addr, "code": code, "password": "short"}); r.Status != 422 {
		t.Fatalf("short password: %d %s", r.Status, r.Raw)
	}
	r := c.must(200, "POST", "/v1/auth/email/register", map[string]any{"email": addr, "code": code, "password": "long-enough-pw"})
	if r.data()["is_new_user"] != true {
		t.Fatalf("register must report a new user: %s", r.Raw)
	}
	c.token = r.data()["access_token"].(string)
	me := c.must(200, "GET", "/v1/me", nil).data()
	if me["email"] != addr || me["registration_complete"] != false || me["needs_password"] != false || me["has_password"] != true {
		t.Fatalf("me after register: %v", me)
	}
	c.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "female", "pd_consent": true})
	var verified bool
	_ = e.d.DB.QueryRow(context.Background(), `SELECT email_verified_at IS NOT NULL FROM users WHERE email = $1`, addr).Scan(&verified)
	if !verified {
		t.Fatal("email must be verified")
	}
	// код одноразовый; адрес уже занят
	if r := e.client().do("POST", "/v1/auth/email/register", map[string]any{"email": addr, "code": code, "password": "long-enough-pw"}); r.Status != 401 {
		t.Fatalf("code reuse: %d %s", r.Status, r.Raw)
	}

	d := e.client()
	if r := d.do("POST", "/v1/auth/email/login", map[string]any{"email": addr, "password": "wrong-password"}); r.Status != 401 || r.errCode() != "bad_credentials" {
		t.Fatalf("wrong password: %d %s", r.Status, r.Raw)
	}
	r = d.must(200, "POST", "/v1/auth/email/login", map[string]any{"email": "TESTER@test.uz", "password": "long-enough-pw"})
	if r.data()["is_new_user"] != false {
		t.Fatalf("login: %s", r.Raw)
	}
	d.must(200, "POST", "/v1/auth/refresh", nil)

	// восстановление: письмо уходит только существующему клиенту, ответ одинаковый
	before := e.mail.sent
	e.client().must(202, "POST", "/v1/auth/email/code", map[string]any{"email": "nobody@test.uz", "purpose": "reset"})
	if e.mail.sent != before {
		t.Fatal("reset code must not be sent to unknown address")
	}
	e.client().must(202, "POST", "/v1/auth/email/code", map[string]any{"email": addr, "purpose": "reset"})
	rc := e.mail.last("reset", addr)
	f := e.client()
	f.must(200, "POST", "/v1/auth/email/reset", map[string]any{"email": addr, "code": rc, "password": "brand-new-password"})
	// старая сессия отозвана, старый пароль не подходит
	if r := d.do("POST", "/v1/auth/refresh", nil); r.Status != 401 {
		t.Fatalf("old session must be revoked: %d", r.Status)
	}
	if r := e.client().do("POST", "/v1/auth/email/login", map[string]any{"email": addr, "password": "long-enough-pw"}); r.Status != 401 {
		t.Fatalf("old password: %d", r.Status)
	}
	e.client().must(200, "POST", "/v1/auth/email/login", map[string]any{"email": addr, "password": "brand-new-password"})

	// удаление аккаунта освобождает адрес
	g := e.client()
	g.token = e.client().must(200, "POST", "/v1/auth/email/login", map[string]any{"email": addr, "password": "brand-new-password"}).data()["access_token"].(string)
	g.must(204, "DELETE", "/v1/me", map[string]any{"confirm": true})
	if r := e.client().do("POST", "/v1/auth/email/login", map[string]any{"email": addr, "password": "brand-new-password"}); r.Status != 401 {
		t.Fatalf("deleted account must not log in: %d", r.Status)
	}
	e.mr.FastForward(2 * time.Minute) // cooldown повторной отправки
	e.client().must(202, "POST", "/v1/auth/email/code", map[string]any{"email": addr, "purpose": "register"})
	e.client().must(200, "POST", "/v1/auth/email/register", map[string]any{"email": addr, "code": e.mail.last("register", addr), "password": "long-enough-pw"})
}

// ---------- Google ----------

type fakeGoogle struct {
	key *rsa.PrivateKey
	srv *httptest.Server
}

const googleClient = "123-web.apps.googleusercontent.com"

func newFakeGoogle(t *testing.T) *fakeGoogle {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	g := &fakeGoogle{key: k}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kid": "k1", "kty": "RSA", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes()),
		}}})
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGoogle) token(t *testing.T, claims jwt.MapClaims) string {
	base := jwt.MapClaims{"iss": "https://accounts.google.com", "aud": googleClient, "sub": "g-1",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "email": "g@test.uz", "email_verified": true}
	for k, v := range claims {
		if v == nil {
			delete(base, k)
		} else {
			base[k] = v
		}
	}
	tk := jwt.NewWithClaims(jwt.SigningMethodRS256, base)
	tk.Header["kid"] = "k1"
	s, err := tk.SignedString(g.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGoogleLogin(t *testing.T) {
	g := newFakeGoogle(t)
	e := setup(t, func(d *Deps) {
		d.Cfg.GoogleClientIDs = []string{googleClient}
		d.GoogleCertsURL = g.srv.URL
	})
	if m := e.client().must(200, "GET", "/v1/auth/methods", nil).data(); m["google"].(map[string]any)["client_id"] != googleClient {
		t.Fatalf("methods: %v", m)
	}
	bad := map[string]jwt.MapClaims{
		"wrong aud":    {"aud": "other.apps.googleusercontent.com"},
		"wrong issuer": {"iss": "https://evil.test"},
		"expired":      {"exp": time.Now().Add(-time.Hour).Unix(), "iat": time.Now().Add(-2 * time.Hour).Unix()},
		"no sub":       {"sub": nil},
	}
	for name, cl := range bad {
		if r := e.client().do("POST", "/v1/auth/google/", map[string]any{"id_token": g.token(t, cl)}); r.Status != 401 || r.errCode() != "google_token_invalid" {
			t.Errorf("%s: %d %s", name, r.Status, r.Raw)
		}
	}
	// чужая подпись
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "accounts.google.com", "aud": googleClient, "sub": "x",
		"exp": time.Now().Add(time.Hour).Unix()})
	forged.Header["kid"] = "k1"
	fs, _ := forged.SignedString(other)
	if r := e.client().do("POST", "/v1/auth/google/", map[string]any{"id_token": fs}); r.Status != 401 {
		t.Fatalf("forged token: %d", r.Status)
	}

	c := e.client()
	r := c.must(200, "POST", "/v1/auth/google/", map[string]any{"id_token": g.token(t, nil)})
	if r.data()["is_new_user"] != true {
		t.Fatalf("new google user: %s", r.Raw)
	}
	c.token = r.data()["access_token"].(string)
	me := c.must(200, "GET", "/v1/me", nil).data()
	if me["email"] != "g@test.uz" || me["has_google"] != true || me["needs_password"] != false {
		t.Fatalf("me: %v", me)
	}
	r = e.client().must(200, "POST", "/v1/auth/google/", map[string]any{"id_token": g.token(t, nil)})
	if r.data()["is_new_user"] != false || r.data()["user"].(map[string]any)["id"] != me["id"] {
		t.Fatalf("second login must reuse account: %s", r.Raw)
	}

	// Аккаунт, занятый без подтверждения почты (старая тестовая регистрация):
	// Google привязывается, пароль «захватчика» стирается.
	ctx := context.Background()
	if _, err := e.d.DB.Exec(ctx, `INSERT INTO users (role, email, password_hash) VALUES ('client', 'victim@test.uz', 'x')`); err != nil {
		t.Fatal(err)
	}
	r = e.client().must(200, "POST", "/v1/auth/google/", map[string]any{"id_token": g.token(t, jwt.MapClaims{"sub": "g-2", "email": "Victim@test.uz"})})
	if r.data()["is_new_user"] != false {
		t.Fatalf("must link to existing account: %s", r.Raw)
	}
	var pw *string
	var sub string
	_ = e.d.DB.QueryRow(ctx, `SELECT password_hash, google_sub FROM users WHERE email = 'victim@test.uz'`).Scan(&pw, &sub)
	if pw != nil || sub != "g-2" {
		t.Fatalf("unverified account: pw=%v sub=%s", pw, sub)
	}
	// непроверенная Google почта не привязывается к чужому аккаунту
	r = e.client().must(200, "POST", "/v1/auth/google/", map[string]any{"id_token": g.token(t, jwt.MapClaims{"sub": "g-3", "email": "g@test.uz", "email_verified": false})})
	if r.data()["is_new_user"] != true {
		t.Fatalf("unverified google email must create a separate account: %s", r.Raw)
	}
}

// ---------- Telegram ----------

func TestTelegramLogin(t *testing.T) {
	fake := &fakeTelegram{}
	api := httptest.NewServer(fake)
	t.Cleanup(api.Close)
	e := setup(t, func(d *Deps) {
		d.Telegram = &telegram.Bot{Token: "123:ABC", BaseURL: api.URL}
		d.Cfg.TelegramBotUsername = "boutique_test_bot"
		d.Cfg.TelegramWebhookSecret = tgSecret
		d.Cfg.SMSAuthEnabled = false
	})
	hook := func(upd map[string]any) {
		t.Helper()
		b, _ := json.Marshal(upd)
		req, _ := http.NewRequest("POST", e.srv.URL+"/v1/telegram/webhook", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", tgSecret)
		res, err := http.DefaultClient.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("webhook: %v %v", err, res)
		}
		res.Body.Close()
	}
	const tgUser = 777001
	from := map[string]any{"id": tgUser, "is_bot": false, "language_code": "ru"}
	msg := func(extra map[string]any) map[string]any {
		m := map[string]any{"chat": map[string]any{"id": tgUser, "type": "private"}, "from": from}
		for k, v := range extra {
			m[k] = v
		}
		return map[string]any{"message": m}
	}
	lastText := func() (string, map[string]any) {
		calls := fake.find("sendMessage")
		if len(calls) == 0 {
			t.Fatal("bot sent nothing")
		}
		c := calls[len(calls)-1]
		rm, _ := c["reply_markup"].(map[string]any)
		return c["text"].(string), rm
	}

	if m := e.client().must(200, "GET", "/v1/auth/methods", nil).data(); m["telegram"].(map[string]any)["bot_username"] != "boutique_test_bot" || m["sms"] != false {
		t.Fatalf("methods: %v", m)
	}

	// 1. новый пользователь: ссылка → «Поделиться номером» → контакт → сессия
	c := e.client()
	st := c.must(200, "POST", "/v1/auth/telegram/start", nil).data()
	id, poll := st["login_id"].(string), st["poll_token"].(string)
	if st["url"] != "https://t.me/boutique_test_bot?start=login_"+id {
		t.Fatalf("url: %v", st["url"])
	}
	pollReq := map[string]any{"login_id": id, "poll_token": poll}
	if r := c.must(200, "POST", "/v1/auth/telegram/poll", pollReq); r.data()["status"] != "pending" {
		t.Fatalf("pending: %s", r.Raw)
	}
	if r := c.do("POST", "/v1/auth/telegram/poll", map[string]any{"login_id": id, "poll_token": "guess"}); r.Status != 410 {
		t.Fatalf("wrong poll token: %d", r.Status)
	}
	hook(msg(map[string]any{"text": "/start login_" + id}))
	if _, rm := lastText(); rm["keyboard"] == nil {
		t.Fatalf("new user must get share-contact keyboard: %v", rm)
	}
	// чужой контакт не принимается
	hook(msg(map[string]any{"contact": map[string]any{"phone_number": "998901112233", "user_id": 42}}))
	if r := c.must(200, "POST", "/v1/auth/telegram/poll", pollReq); r.data()["status"] != "pending" {
		t.Fatal("foreign contact must not confirm")
	}
	hook(msg(map[string]any{"contact": map[string]any{"phone_number": "998901112233", "user_id": tgUser}}))
	r := c.must(200, "POST", "/v1/auth/telegram/poll", pollReq)
	if r.data()["is_new_user"] != true || r.data()["access_token"] == nil {
		t.Fatalf("session: %s", r.Raw)
	}
	// одноразово
	if r := c.do("POST", "/v1/auth/telegram/poll", pollReq); r.Status != 410 {
		t.Fatalf("poll reuse: %d", r.Status)
	}
	c.token = r.data()["access_token"].(string)
	me := c.must(200, "GET", "/v1/me", nil).data()
	if me["phone"] != "+998901112233" || me["has_telegram"] != true || me["needs_password"] != false {
		t.Fatalf("me: %v", me)
	}

	// 2. знакомый пользователь: кнопка «Войти»
	d := e.client()
	st = d.must(200, "POST", "/v1/auth/telegram/start", nil).data()
	id2 := st["login_id"].(string)
	hook(msg(map[string]any{"text": "/start login_" + id2}))
	_, rm := lastText()
	kb, _ := rm["inline_keyboard"].([]any)
	if len(kb) == 0 {
		t.Fatalf("known user must get inline login button: %v", rm)
	}
	// кнопку нажимает другой человек (ему переслали сообщение) — вход не подтверждается
	cb := func(user int64) map[string]any {
		return map[string]any{"callback_query": map[string]any{"id": "cb1", "data": "login:" + id2,
			"from":    map[string]any{"id": user},
			"message": map[string]any{"message_id": 1, "text": "x", "chat": map[string]any{"id": user}}}}
	}
	hook(cb(999))
	poll2 := map[string]any{"login_id": id2, "poll_token": st["poll_token"]}
	if r := d.must(200, "POST", "/v1/auth/telegram/poll", poll2); r.data()["status"] != "pending" {
		t.Fatalf("stranger confirmed login: %s", r.Raw)
	}
	hook(cb(tgUser))
	r = d.must(200, "POST", "/v1/auth/telegram/poll", poll2)
	if r.data()["is_new_user"] != false || r.data()["user"].(map[string]any)["id"] != me["id"] {
		t.Fatalf("known login: %s", r.Raw)
	}

	// 3. просроченная ссылка
	st = e.client().must(200, "POST", "/v1/auth/telegram/start", nil).data()
	e.mr.FastForward(6 * time.Minute)
	hook(msg(map[string]any{"text": "/start login_" + st["login_id"].(string)}))
	if txt, _ := lastText(); !strings.Contains(txt, "устарела") {
		t.Fatalf("expired: %s", txt)
	}

	// 4. существующий клиент с тем же номером (старый вход по SMS) получает привязку
	if _, err := e.d.DB.Exec(context.Background(), `INSERT INTO users (role, phone) VALUES ('client', '+998905550000')`); err != nil {
		t.Fatal(err)
	}
	from = map[string]any{"id": 777002, "language_code": "uz"}
	g := e.client()
	st = g.must(200, "POST", "/v1/auth/telegram/start", nil).data()
	hook(map[string]any{"message": map[string]any{"chat": map[string]any{"id": 777002, "type": "private"}, "from": from, "text": "/start login_" + st["login_id"].(string)}})
	hook(map[string]any{"message": map[string]any{"chat": map[string]any{"id": 777002, "type": "private"}, "from": from,
		"contact": map[string]any{"phone_number": "+998905550000", "user_id": 777002}}})
	r = g.must(200, "POST", "/v1/auth/telegram/poll", map[string]any{"login_id": st["login_id"], "poll_token": st["poll_token"]})
	if r.data()["is_new_user"] != false {
		t.Fatalf("existing phone must be linked: %s", r.Raw)
	}
	if txt, _ := lastText(); !strings.Contains(txt, "Tayyor") {
		t.Fatalf("uz text: %s", txt)
	}
}

func TestAuthConfigDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("JWT_SECRET", strings.Repeat("j", 32))
	t.Setenv("OTP_PEPPER", strings.Repeat("p", 16))
	t.Setenv("APP_ENV", "prod")
	c, _ := config.Load()
	if !c.EmailAuthEnabled || c.SMSAuthEnabled {
		t.Fatalf("prod defaults: email=%t sms=%t", c.EmailAuthEnabled, c.SMSAuthEnabled)
	}
}
