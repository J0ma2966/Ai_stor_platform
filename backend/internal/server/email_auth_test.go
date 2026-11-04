package server

// ВРЕМЕННО: тесты входа по почте (auth/email_temp.go). Удаляются вместе с функцией.

import (
	"context"
	"testing"

	"github.com/boutique-market/backend/internal/config"
)

func TestEmailAuth_RegisterLoginAndDelete(t *testing.T) {
	e := setup(t)
	c := e.client()

	if r := c.must(200, "GET", "/v1/auth/email/status", nil); r.data()["enabled"] != true {
		t.Fatalf("status: %s", r.Raw)
	}
	if r := c.do("POST", "/v1/auth/email/register", map[string]any{"email": "not-an-email", "password": "long-enough-pw"}); r.Status != 422 {
		t.Fatalf("bad email: %d %s", r.Status, r.Raw)
	}
	if r := c.do("POST", "/v1/auth/email/register", map[string]any{"email": "a@test.uz", "password": "short"}); r.Status != 422 {
		t.Fatalf("short password: %d %s", r.Status, r.Raw)
	}

	r := c.must(200, "POST", "/v1/auth/email/register", map[string]any{"email": "Tester@Test.UZ", "password": "long-enough-pw"})
	if r.data()["is_new_user"] != true {
		t.Fatalf("register must report a new user: %s", r.Raw)
	}
	c.token = r.data()["access_token"].(string)
	me := c.must(200, "GET", "/v1/me", nil).data()
	if me["role"] != "client" || me["registration_complete"] != false {
		t.Fatalf("me after register: %v", me)
	}
	// Дальше обычный путь нового клиента: пол + согласие, приветственный бонус.
	c.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "male", "pd_consent": true})

	if r := e.client().do("POST", "/v1/auth/email/register", map[string]any{"email": "tester@test.uz", "password": "another-pw-123"}); r.Status != 409 || r.errCode() != "email_taken" {
		t.Fatalf("duplicate email: %d %s", r.Status, r.Raw)
	}

	d := e.client()
	if r := d.do("POST", "/v1/auth/email/login", map[string]any{"email": "tester@test.uz", "password": "wrong-password"}); r.Status != 401 || r.errCode() != "bad_credentials" {
		t.Fatalf("wrong password: %d %s", r.Status, r.Raw)
	}
	if r := d.do("POST", "/v1/auth/email/login", map[string]any{"email": "nobody@test.uz", "password": "whatever-pw"}); r.Status != 401 || r.errCode() != "bad_credentials" {
		t.Fatalf("unknown email: %d %s", r.Status, r.Raw)
	}
	r = d.must(200, "POST", "/v1/auth/email/login", map[string]any{"email": "TESTER@test.uz", "password": "long-enough-pw"})
	if r.data()["is_new_user"] != false {
		t.Fatalf("login must not be a new user: %s", r.Raw)
	}
	d.token = r.data()["access_token"].(string)
	// Refresh-cookie работает так же, как у входа по SMS.
	d.must(200, "POST", "/v1/auth/refresh", nil)

	// Почтовый аккаунт не входит в кабинет магазина/админа.
	if r := d.do("POST", "/v1/auth/login", map[string]any{"login": "tester@test.uz", "password": "long-enough-pw"}); r.Status == 200 {
		t.Fatal("email client must not log in via staff login")
	}

	// Удаление аккаунта стирает почту: тот же адрес можно зарегистрировать заново.
	d.must(204, "DELETE", "/v1/me", map[string]any{"confirm": true})
	var n int
	if err := e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM users WHERE email = 'tester@test.uz' OR (role = 'client' AND password_hash IS NOT NULL)`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("email/password must be purged on delete: n=%d err=%v", n, err)
	}
	if r := e.client().do("POST", "/v1/auth/email/login", map[string]any{"email": "tester@test.uz", "password": "long-enough-pw"}); r.Status != 401 {
		t.Fatalf("deleted account must not log in: %d %s", r.Status, r.Raw)
	}
	e.client().must(200, "POST", "/v1/auth/email/register", map[string]any{"email": "tester@test.uz", "password": "long-enough-pw"})
}

func TestEmailAuth_DisabledByFlag(t *testing.T) {
	e := setup(t, func(d *Deps) { d.Cfg.EmailAuthEnabled = false })
	c := e.client()
	if r := c.must(200, "GET", "/v1/auth/email/status", nil); r.data()["enabled"] != false {
		t.Fatalf("status: %s", r.Raw)
	}
	for _, p := range []string{"/v1/auth/email/register", "/v1/auth/email/login"} {
		if r := c.do("POST", p, map[string]any{"email": "a@test.uz", "password": "long-enough-pw"}); r.Status != 404 && r.Status != 405 {
			t.Fatalf("%s must be off: %d", p, r.Status)
		}
	}
}

func TestEmailAuth_DefaultOffInProd(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("EMAIL_AUTH_ENABLED", "")
	c, _ := config.Load() // остальные prod-проверки здесь не важны
	if c.EmailAuthEnabled {
		t.Fatal("email auth must be off in prod unless EMAIL_AUTH_ENABLED=true")
	}
	t.Setenv("APP_ENV", "dev")
	if c, _ := config.Load(); !c.EmailAuthEnabled {
		t.Fatal("email auth should default to on in dev")
	}
	t.Setenv("APP_ENV", "prod")
	t.Setenv("EMAIL_AUTH_ENABLED", "true")
	if c, _ := config.Load(); !c.EmailAuthEnabled {
		t.Fatal("explicit EMAIL_AUTH_ENABLED=true must work in prod")
	}
}
