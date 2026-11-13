package mail

import (
	"bufio"
	"context"
	"net"
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
