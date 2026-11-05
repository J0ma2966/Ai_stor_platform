// Package outbox — очередь исходящих сообщений (Telegram магазину, SMS клиенту).
//
// Сообщение пишется в той же транзакции, что и событие (новый заказ, смена
// статуса), поэтому не теряется и не уходит, если транзакция откатилась.
// Worker забирает сообщения с FOR UPDATE SKIP LOCKED (можно несколько
// инстансов API) и повторяет отправку с нарастающей паузой.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ChannelTelegram = "telegram"
	ChannelSMS      = "sms"

	MaxAttempts = 8
)

type Execer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Button — кнопка под сообщением Telegram: ссылка (URL) или действие бота (Data).
type Button struct {
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
	Data string `json:"data,omitempty"`
}

// Message — содержимое сообщения. Для SMS используется только Text.
type Message struct {
	Text    string     `json:"text"`
	Buttons [][]Button `json:"buttons,omitempty"`
}

// ToStoreTelegram ставит сообщение в чат магазина, если магазин подключил бота.
func ToStoreTelegram(ctx context.Context, q Execer, storeID uuid.UUID, m Message) error {
	_, err := q.Exec(ctx, `INSERT INTO outbound_messages (channel, recipient, payload)
		SELECT 'telegram', telegram_chat_id::text, $2 FROM stores WHERE id = $1 AND telegram_chat_id IS NOT NULL`, storeID, m)
	return err
}

// ToTelegramChat ставит сообщение в конкретный чат.
func ToTelegramChat(ctx context.Context, q Execer, chatID int64, m Message) error {
	_, err := q.Exec(ctx, `INSERT INTO outbound_messages (channel, recipient, payload) VALUES ('telegram', $1, $2)`,
		strconv.FormatInt(chatID, 10), m)
	return err
}

// ToSMS ставит SMS на номер в формате E.164.
func ToSMS(ctx context.Context, q Execer, phone, text string) error {
	if phone == "" || text == "" {
		return nil
	}
	_, err := q.Exec(ctx, `INSERT INTO outbound_messages (channel, recipient, payload) VALUES ('sms', $1, $2)`,
		phone, Message{Text: text})
	return err
}

// TelegramSender отправляет сообщение в чат.
type TelegramSender interface {
	Send(ctx context.Context, chatID int64, m Message) error
}

// SMSSender отправляет произвольный текст на телефон.
type SMSSender interface {
	SendText(ctx context.Context, phone, text string) error
}

// ErrPermanent — повторять бесполезно (бот заблокирован, чат удалён).
var ErrPermanent = errors.New("outbox: permanent failure")

type Worker struct {
	DB       *pgxpool.Pool
	Log      *slog.Logger
	Telegram TelegramSender // nil — канал выключен, сообщения ждут
	SMS      SMSSender      // nil — канал выключен
	// OnPermanent вызывается при необратимой ошибке Telegram (например, отвязать чат).
	OnPermanent func(ctx context.Context, channel, recipient string)
}

type row struct {
	id        int64
	channel   string
	recipient string
	payload   []byte
	attempts  int
}

// Переменные, а не константы, — чтобы тест мог ускорить время.
var (
	// leaseTTL — на сколько воркер «занимает» пачку сообщений.
	leaseTTL = 2 * time.Minute
	// sendTimeout — предел на отправку одного сообщения.
	sendTimeout = 15 * time.Second
)

// Backoff — пауза перед повтором n (с 1): 10 с, 30 с, 1.5 мин, … не больше часа.
func Backoff(n int) time.Duration {
	d := 10 * time.Second
	for i := 1; i < n; i++ {
		d *= 3
		if d > time.Hour {
			return time.Hour
		}
	}
	return d
}

// RunOnce отправляет до 50 сообщений. Возвращает число обработанных.
func (w *Worker) RunOnce(ctx context.Context) (int, error) {
	var channels []string
	if w.Telegram != nil {
		channels = append(channels, ChannelTelegram)
	}
	if w.SMS != nil {
		channels = append(channels, ChannelSMS)
	}
	if len(channels) == 0 {
		return 0, nil
	}
	// Аренда: сообщение «занимается» на 2 минуты коротким UPDATE, потом отправляется
	// без открытой транзакции. Если процесс упал посреди отправки, через 2 минуты
	// сообщение заберёт другой воркер (возможен редкий дубль — лучше, чем потеря).
	leasedAt := time.Now()
	rows, err := w.DB.Query(ctx, `UPDATE outbound_messages SET next_at = now() + make_interval(secs => $2)
		WHERE id IN (SELECT id FROM outbound_messages
			WHERE sent_at IS NULL AND failed_at IS NULL AND next_at <= now() AND channel = ANY($1)
			ORDER BY next_at LIMIT 50 FOR UPDATE SKIP LOCKED)
		RETURNING id, channel, recipient, payload, attempts`, channels, leaseTTL.Seconds())
	if err != nil {
		return 0, err
	}
	batch, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		err := r.Scan(&x.id, &x.channel, &x.recipient, &x.payload, &x.attempts)
		return x, err
	})
	if err != nil {
		return 0, err
	}
	for i, m := range batch {
		// Пачка не успевает уложиться в аренду (провайдер отвечает медленно, до
		// sendTimeout на сообщение): остаток отдаём обратно, пока аренда не истекла,
		// иначе его же заберёт воркер другого инстанса API и сообщения уйдут дважды.
		if time.Since(leasedAt) > leaseTTL-2*sendTimeout {
			rest := make([]int64, 0, len(batch)-i)
			for _, r := range batch[i:] {
				rest = append(rest, r.id)
			}
			if _, err := w.DB.Exec(ctx, `UPDATE outbound_messages SET next_at = now()
				WHERE id = ANY($1) AND sent_at IS NULL AND failed_at IS NULL`, rest); err != nil {
				return 0, err
			}
			return i, nil
		}
		sendErr := w.send(ctx, m)
		attempts := m.attempts + 1
		switch {
		case sendErr == nil:
			_, err = w.DB.Exec(ctx, `UPDATE outbound_messages SET sent_at = now(), attempts = $2, last_error = NULL WHERE id = $1`, m.id, attempts)
		case errors.Is(sendErr, ErrPermanent) || attempts >= MaxAttempts:
			w.Log.Warn("outbox message dropped", slog.String("channel", m.channel), slog.Int64("id", m.id), slog.String("err", sendErr.Error()))
			_, err = w.DB.Exec(ctx, `UPDATE outbound_messages SET failed_at = now(), attempts = $2, last_error = $3 WHERE id = $1`,
				m.id, attempts, short(sendErr.Error()))
			if errors.Is(sendErr, ErrPermanent) && w.OnPermanent != nil {
				w.OnPermanent(ctx, m.channel, m.recipient)
			}
		default:
			_, err = w.DB.Exec(ctx, `UPDATE outbound_messages SET next_at = now() + make_interval(secs => $2), attempts = $3, last_error = $4 WHERE id = $1`,
				m.id, Backoff(attempts).Seconds(), attempts, short(sendErr.Error()))
		}
		if err != nil {
			return 0, err
		}
	}
	return len(batch), nil
}

func (w *Worker) send(ctx context.Context, m row) error {
	var msg Message
	if err := json.Unmarshal(m.payload, &msg); err != nil {
		return ErrPermanent
	}
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	switch m.channel {
	case ChannelTelegram:
		chatID, err := strconv.ParseInt(m.recipient, 10, 64)
		if err != nil {
			return ErrPermanent
		}
		return w.Telegram.Send(ctx, chatID, msg)
	case ChannelSMS:
		return w.SMS.SendText(ctx, m.recipient, msg.Text)
	}
	return ErrPermanent
}

// Run обрабатывает очередь каждые every, пока не отменён ctx.
func (w *Worker) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for {
				n, err := w.RunOnce(ctx)
				if err != nil {
					if ctx.Err() == nil {
						w.Log.Error("outbox", slog.String("err", err.Error()))
					}
					break
				}
				if n < 50 {
					break
				}
			}
		}
	}
}

func short(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
