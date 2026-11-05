package server

import "testing"

// Клиент регистрируется по SMS, задаёт пароль и дальше входит по паролю без SMS.
func TestClientPassword_SetLoginChangeReset(t *testing.T) {
	e := setup(t)
	const phone = "+998901212121"
	c := e.client()
	c.loginClient(phone)

	me := c.must(200, "GET", "/v1/me", nil).data()
	if me["has_password"] != false {
		t.Fatalf("new SMS client must not have a password: %v", me)
	}
	// Пока пароля нет, вход по паролю невозможен (тот же ответ, что и при неверном пароле).
	if r := e.client().do("POST", "/v1/auth/password/login", map[string]any{"phone": phone, "password": "anything-123"}); r.errCode() != "bad_credentials" {
		t.Fatalf("login without password set: %d %s", r.Status, r.Raw)
	}
	if r := c.do("POST", "/v1/auth/password", map[string]any{"password": "short"}); r.Status != 422 {
		t.Fatalf("short password: %d %s", r.Status, r.Raw)
	}
	r := c.must(200, "POST", "/v1/auth/password", map[string]any{"password": "first-password-1"})
	c.token = r.data()["access_token"].(string)
	if me := c.must(200, "GET", "/v1/me", nil).data(); me["has_password"] != true {
		t.Fatalf("has_password after set: %v", me)
	}
	// Текущее устройство получило новую сессию: refresh-cookie рабочая.
	c.must(200, "POST", "/v1/auth/refresh", nil)

	// Вход по паролю: SMS не отправляется.
	e.sms.codes = map[string]string{}
	d := e.client()
	if r := d.do("POST", "/v1/auth/password/login", map[string]any{"phone": phone, "password": "wrong-password"}); r.Status != 401 || r.errCode() != "bad_credentials" {
		t.Fatalf("wrong password: %d %s", r.Status, r.Raw)
	}
	if r := d.do("POST", "/v1/auth/password/login", map[string]any{"phone": "+998909999999", "password": "first-password-1"}); r.errCode() != "bad_credentials" {
		t.Fatalf("unknown phone: %d %s", r.Status, r.Raw)
	}
	if r := d.do("POST", "/v1/auth/password/login", map[string]any{"phone": "+12025550123", "password": "first-password-1"}); r.errCode() != "phone_not_allowed" {
		t.Fatalf("foreign phone: %d %s", r.Status, r.Raw)
	}
	r = d.must(200, "POST", "/v1/auth/password/login", map[string]any{"phone": phone, "password": "first-password-1"})
	if r.data()["is_new_user"] != false || r.data()["user"].(map[string]any)["role"] != "client" {
		t.Fatalf("password login: %s", r.Raw)
	}
	d.token = r.data()["access_token"].(string)
	if e.sms.last(phone) != "" {
		t.Fatal("password login must not send SMS")
	}

	// Смена пароля требует текущий.
	if r := d.do("POST", "/v1/auth/password", map[string]any{"password": "second-password-2"}); r.errCode() != "current_password_required" {
		t.Fatalf("change without current: %d %s", r.Status, r.Raw)
	}
	if r := d.do("POST", "/v1/auth/password", map[string]any{"password": "second-password-2", "current_password": "nope-nope-nope"}); r.errCode() != "current_password_wrong" {
		t.Fatalf("change with wrong current: %d %s", r.Status, r.Raw)
	}
	r = d.must(200, "POST", "/v1/auth/password", map[string]any{"password": "second-password-2", "current_password": "first-password-1"})
	d.token = r.data()["access_token"].(string)
	d.must(200, "POST", "/v1/auth/refresh", nil)
	// Другое устройство (c) выкинуто: его refresh-токен отозван.
	if r := c.do("POST", "/v1/auth/refresh", nil); r.Status != 401 {
		t.Fatalf("other device refresh after password change: %d %s", r.Status, r.Raw)
	}
	if r := e.client().do("POST", "/v1/auth/password/login", map[string]any{"phone": phone, "password": "first-password-1"}); r.Status != 401 {
		t.Fatalf("old password still works: %d", r.Status)
	}

	// Забыли пароль: код из SMS + новый пароль.
	f := e.client()
	e.mr.Del("otp:cd:" + phone)
	f.must(202, "POST", "/v1/auth/otp/request", map[string]any{"phone": phone})
	code := e.sms.last(phone)
	// Слишком короткий пароль не сжигает код.
	if r := f.do("POST", "/v1/auth/password/reset", map[string]any{"phone": phone, "code": code, "password": "short"}); r.Status != 422 {
		t.Fatalf("reset short: %d %s", r.Status, r.Raw)
	}
	if r := f.do("POST", "/v1/auth/password/reset", map[string]any{"phone": phone, "code": "000000", "password": "third-password-3"}); r.errCode() != "otp_invalid" && code != "000000" {
		t.Fatalf("reset wrong code: %d %s", r.Status, r.Raw)
	}
	r = f.must(200, "POST", "/v1/auth/password/reset", map[string]any{"phone": phone, "code": code, "password": "third-password-3"})
	if r.data()["is_new_user"] != false {
		t.Fatalf("reset of existing account: %s", r.Raw)
	}
	if r := d.do("POST", "/v1/auth/refresh", nil); r.Status != 401 {
		t.Fatalf("session survived reset: %d", r.Status)
	}
	e.client().must(200, "POST", "/v1/auth/password/login", map[string]any{"phone": phone, "password": "third-password-3"})
	// Код одноразовый.
	if r := e.client().do("POST", "/v1/auth/password/reset", map[string]any{"phone": phone, "code": code, "password": "fourth-password-4"}); r.errCode() != "otp_invalid" {
		t.Fatalf("reset code reuse: %d %s", r.Status, r.Raw)
	}

	// Сотрудник не задаёт пароль через клиентский маршрут.
	if r := e.client().do("POST", "/v1/auth/password", map[string]any{"password": "whatever-123"}); r.Status != 401 {
		t.Fatalf("anon set password: %d", r.Status)
	}

	// Удаление аккаунта стирает пароль: по нему больше не войти.
	f.token = r.data()["access_token"].(string)
	f.must(204, "DELETE", "/v1/me", map[string]any{"confirm": true})
	if r := e.client().do("POST", "/v1/auth/password/login", map[string]any{"phone": phone, "password": "third-password-3"}); r.Status != 401 {
		t.Fatalf("deleted account password login: %d %s", r.Status, r.Raw)
	}
}

// Сброс пароля для незнакомого номера — это регистрация: ответ не выдаёт, есть ли аккаунт.
func TestClientPassword_ResetUnknownPhoneRegisters(t *testing.T) {
	e := setup(t)
	const phone = "+998901313131"
	c := e.client()
	c.must(202, "POST", "/v1/auth/otp/request", map[string]any{"phone": phone})
	r := c.must(200, "POST", "/v1/auth/password/reset", map[string]any{"phone": phone, "code": e.sms.last(phone), "password": "brand-new-pass"})
	if r.data()["is_new_user"] != true {
		t.Fatalf("reset for unknown phone must register: %s", r.Raw)
	}
	c.token = r.data()["access_token"].(string)
	if me := c.must(200, "GET", "/v1/me", nil).data(); me["has_password"] != true || me["registration_complete"] != false {
		t.Fatalf("me: %v", me)
	}
}
