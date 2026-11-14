package mail

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestComposeAndMessage(t *testing.T) {
	subj, body := Compose("123456", PurposeRegister, "ru", "Boutique")
	if !strings.Contains(subj, "123456") || !strings.Contains(body, "123456") {
		t.Fatalf("ru: %s / %s", subj, body)
	}
	subj, _ = Compose("654321", PurposeReset, "uz", "Boutique")
	if !strings.Contains(subj, "tiklash") {
		t.Fatalf("uz reset: %s", subj)
	}
	from, _ := mail.ParseAddress("Boutique <no-reply@shop.uz>")
	msg := string(buildMessage(from, "a@b.uz", "Код подтверждения", "x"))
	if !strings.Contains(msg, "Subject: =?utf-8?q?") || !strings.Contains(msg, "To: a@b.uz\r\n") {
		t.Fatalf("message headers: %q", msg)
	}
	if maskEmail("tester@test.uz") != "t…@test.uz" {
		t.Fatal(maskEmail("tester@test.uz"))
	}
}

// Сервер без STARTTLS: письмо (и пароль SMTP) не должно уйти открытым текстом.
func TestSMTPRequiresTLS(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 16)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		rw := bufio.NewReadWriter(bufio.NewReader(c), bufio.NewWriter(c))
		rw.WriteString("220 test ESMTP\r\n")
		rw.Flush()
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				return
			}
			got <- strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "EHLO"):
				rw.WriteString("250-test\r\n250 AUTH PLAIN\r\n")
			case strings.HasPrefix(line, "QUIT"):
				rw.WriteString("221 bye\r\n")
				rw.Flush()
				return
			default:
				rw.WriteString("250 ok\r\n")
			}
			rw.Flush()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	s := &SMTP{Host: "127.0.0.1", Port: port, User: "u", Password: "secret", From: "no-reply@shop.uz", Timeout: 3 * time.Second}
	err = s.SendCode(context.Background(), "a@b.uz", "123456", PurposeRegister, "ru")
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("must refuse without STARTTLS: %v", err)
	}
	for {
		select {
		case l := <-got:
			if strings.HasPrefix(l, "AUTH") || strings.HasPrefix(l, "MAIL") {
				t.Fatalf("sent %q over plaintext", l)
			}
		default:
			return
		}
	}
}

func TestHTTPAPI(t *testing.T) {
	for _, p := range []string{"brevo", "resend"} {
		var body map[string]any
		var hdr http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hdr = r.Header.Clone()
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusCreated)
		}))
		s := &HTTPAPI{Provider: p, APIKey: "k-123", From: "Shop <no-reply@shop.uz>", Endpoint: srv.URL}
		if err := s.SendCode(context.Background(), "a@b.uz", "123456", PurposeRegister, "ru"); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		srv.Close()
		raw, _ := json.Marshal(body)
		if !strings.Contains(string(raw), "123456") || !strings.Contains(string(raw), "a@b.uz") || !strings.Contains(string(raw), "no-reply@shop.uz") {
			t.Fatalf("%s payload: %s", p, raw)
		}
		if hdr.Get("api-key") != "k-123" && hdr.Get("Authorization") != "Bearer k-123" {
			t.Fatalf("%s: no key header", p)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"sender not verified"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	err := (&HTTPAPI{Provider: "brevo", APIKey: "k", From: "a@shop.uz", Endpoint: srv.URL}).SendCode(context.Background(), "a@b.uz", "1", PurposeReset, "uz")
	if err == nil || !strings.Contains(err.Error(), "sender not verified") {
		t.Fatalf("want provider error, got %v", err)
	}
	if !KnownAPIProvider("resend") || KnownAPIProvider("smtp") {
		t.Fatal("KnownAPIProvider")
	}
}
