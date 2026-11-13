// Package mail отправляет клиенту письма с кодом подтверждения (регистрация по
// почте и восстановление пароля). MAIL_PROVIDER=smtp — любой SMTP-сервис
// (Яндекс 360, Google Workspace, Mailgun, SendGrid…); log — код в лог, только dev/test.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// Purpose — зачем отправлен код (меняет тему и текст письма).
type Purpose string

const (
	PurposeRegister Purpose = "register"
	PurposeReset    Purpose = "reset"
)

// Sender отправляет код на почту.
type Sender interface {
	SendCode(ctx context.Context, to, code string, purpose Purpose, lang string) error
}

// LogSender пишет код в лог. ТОЛЬКО для dev/test — config.Load запрещает его в prod.
type LogSender struct{ Log *slog.Logger }

func (s LogSender) SendCode(_ context.Context, to, code string, purpose Purpose, _ string) error {
	s.Log.Warn("DEV EMAIL CODE (never in prod)", slog.String("to", maskEmail(to)), slog.String("purpose", string(purpose)), slog.String("code", code))
	return nil
}

func maskEmail(e string) string {
	at := strings.LastIndexByte(e, '@')
	if at <= 1 {
		return "…" + e[max(at, 0):]
	}
	return e[:1] + "…" + e[at:]
}

// SMTP — отправка через SMTP с обязательным шифрованием: порт 465 — TLS сразу,
// иначе STARTTLS (без него письмо не отправляется, пароль не уходит открытым текстом).
type SMTP struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string // "Boutique <no-reply@shop.uz>"
	SiteName string // в теме письма; пусто — «Boutique»
	Timeout  time.Duration
}

func (s *SMTP) SendCode(ctx context.Context, to, code string, purpose Purpose, lang string) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("mail: bad MAIL_FROM: %w", err)
	}
	rcpt, err := mail.ParseAddress(to)
	if err != nil || strings.ContainsAny(to, "\r\n") {
		return errors.New("mail: bad recipient")
	}
	subject, body := Compose(code, purpose, lang, s.siteName())
	msg := buildMessage(from, rcpt.Address, subject, body)

	timeout := s.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	tlsCfg := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	d := &net.Dialer{}
	var conn net.Conn
	if s.Port == 465 {
		conn, err = (&tls.Dialer{NetDialer: d, Config: tlsCfg}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mail: connect: %w", err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("mail: hello: %w", err)
	}
	defer c.Close()
	if s.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("mail: server does not support STARTTLS")
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("mail: starttls: %w", err)
		}
	}
	if s.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.User, s.Password, s.Host)); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return fmt.Errorf("mail: from: %w", err)
	}
	if err := c.Rcpt(rcpt.Address); err != nil {
		return fmt.Errorf("mail: rcpt: %w", err)
	}
	wc, err := c.Data()
	if err != nil {
		return fmt.Errorf("mail: data: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		return fmt.Errorf("mail: write: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("mail: send: %w", err)
	}
	return c.Quit()
}

func (s *SMTP) siteName() string {
	if s.SiteName == "" {
		return "Boutique"
	}
	return s.SiteName
}

// Compose — тема и текст письма на языке клиента (ru по умолчанию).
func Compose(code string, purpose Purpose, lang, site string) (subject, body string) {
	if lang == "uz" {
		if purpose == PurposeReset {
			return site + ": parolni tiklash kodi " + code,
				"Parolni tiklash uchun kod: " + code + "\r\n\r\nKod 10 daqiqa amal qiladi. Agar siz so'ramagan bo'lsangiz, bu xatni e'tiborsiz qoldiring.\r\n"
		}
		return site + ": tasdiqlash kodi " + code,
			"Ro'yxatdan o'tish uchun kod: " + code + "\r\n\r\nKod 10 daqiqa amal qiladi. Agar siz so'ramagan bo'lsangiz, bu xatni e'tiborsiz qoldiring.\r\n"
	}
	if purpose == PurposeReset {
		return site + ": код для восстановления пароля " + code,
			"Код для восстановления пароля: " + code + "\r\n\r\nКод действует 10 минут. Если вы не запрашивали код, просто проигнорируйте это письмо.\r\n"
	}
	return site + ": код подтверждения " + code,
		"Код для регистрации: " + code + "\r\n\r\nКод действует 10 минут. Если вы не регистрировались, просто проигнорируйте это письмо.\r\n"
}

func buildMessage(from *mail.Address, to, subject, body string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from.String() + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("Auto-Submitted: auto-generated\r\n\r\n")
	b.WriteString(body)
	return []byte(b.String())
}
