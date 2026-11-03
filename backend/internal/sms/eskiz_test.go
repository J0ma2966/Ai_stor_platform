package sms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeEskiz struct {
	logins   atomic.Int32
	sends    atomic.Int32
	valid    atomic.Value // текущий действующий токен
	lastForm atomic.Value
	sendCode int
}

func (f *fakeEskiz) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("email") != "shop@example.uz" || r.PostForm.Get("password") != "secret" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"Invalid credentials"}`))
			return
		}
		n := f.logins.Add(1)
		tok := "tok-" + string(rune('0'+n))
		f.valid.Store(tok)
		_, _ = w.Write([]byte(`{"message":"token_generated","data":{"token":"` + tok + `"},"token_type":"bearer"}`))
	})
	mux.HandleFunc("POST /message/sms/send", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+f.valid.Load().(string) {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"status":"token-invalid"}`))
			return
		}
		_ = r.ParseForm()
		f.lastForm.Store(r.PostForm)
		f.sends.Add(1)
		if f.sendCode != 0 {
			w.WriteHeader(f.sendCode)
			_, _ = w.Write([]byte(`{"message":"Bu matn moderatsiyadan o'tmagan"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"4385062","message":"Waiting for SMS provider","status":"waiting"}`))
	})
	return mux
}

func newEskiz(t *testing.T, f *fakeEskiz) *Eskiz {
	f.valid.Store("")
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	return &Eskiz{BaseURL: srv.URL, Email: "shop@example.uz", Password: "secret", From: "4546", Template: "Kod: %s", HTTP: srv.Client()}
}

func TestEskiz_SendAndTokenReuse(t *testing.T) {
	f := &fakeEskiz{}
	e := newEskiz(t, f)
	for range 3 {
		if err := e.SendOTP(context.Background(), "+998901234567", "123456"); err != nil {
			t.Fatal(err)
		}
	}
	if f.logins.Load() != 1 || f.sends.Load() != 3 {
		t.Fatalf("logins %d sends %d", f.logins.Load(), f.sends.Load())
	}
	form := f.lastForm.Load().(url.Values)
	if form["mobile_phone"][0] != "998901234567" || form["message"][0] != "Kod: 123456" || form["from"][0] != "4546" {
		t.Fatalf("form: %v", form)
	}
}

func TestEskiz_ReloginOnExpiredToken(t *testing.T) {
	f := &fakeEskiz{}
	e := newEskiz(t, f)
	if err := e.SendOTP(context.Background(), "+998901234567", "111111"); err != nil {
		t.Fatal(err)
	}
	f.valid.Store("rotated-elsewhere") // токен истёк на стороне Eskiz
	f.logins.Store(5)                  // следующий логин выдаст новый токен
	if err := e.SendOTP(context.Background(), "+998901234567", "222222"); err != nil {
		t.Fatal(err)
	}
	if f.logins.Load() != 6 {
		t.Fatalf("expected re-login, logins=%d", f.logins.Load())
	}
}

func TestEskiz_Errors(t *testing.T) {
	f := &fakeEskiz{sendCode: 400}
	e := newEskiz(t, f)
	err := e.SendOTP(context.Background(), "+998901234567", "123456")
	if !errors.Is(err, ErrSend) || strings.Contains(err.Error(), "123456") || strings.Contains(err.Error(), "901234567") {
		t.Fatalf("err = %v", err)
	}
	bad := newEskiz(t, &fakeEskiz{})
	bad.Password = "wrong"
	if err := bad.SendOTP(context.Background(), "+998901234567", "123456"); err == nil || !strings.Contains(err.Error(), "login failed") {
		t.Fatalf("login err = %v", err)
	}
}

func TestEskiz_FixedTestTemplate(t *testing.T) {
	f := &fakeEskiz{}
	e := newEskiz(t, f)
	e.Template = "Bu Eskiz dan test"
	if err := e.SendOTP(context.Background(), "+998901234567", "123456"); err != nil {
		t.Fatal(err)
	}
	if m := f.lastForm.Load().(url.Values)["message"][0]; m != "Bu Eskiz dan test" {
		t.Fatalf("message %q", m)
	}
}
