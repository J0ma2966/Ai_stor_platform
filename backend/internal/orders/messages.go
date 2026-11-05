package orders

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/outbox"
	"github.com/boutique-market/backend/internal/platform/db"
)

// Тексты для Telegram магазина и SMS клиенту. В Telegram уходят только номер
// заказа, товары и сумма — без телефона и адреса клиента: их магазин видит
// в окне доставки на сайте (ПД не покидают платформу без нужды).

// Callback-данные кнопок Telegram: «confirm:<id заказа>».
const CallbackConfirm = "confirm:"

type msgLine struct {
	title string
	size  string
	qty   int
	price int64
}

// FormatSum: 1500000 тийин → «15 000 сум».
func FormatSum(tiyin int64) string {
	s := fmt.Sprintf("%d", tiyin/100)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	return b.String() + " сум"
}

func orderURL(site string, id uuid.UUID) string {
	site = strings.TrimRight(site, "/")
	if !strings.HasPrefix(site, "https://") {
		return "" // Telegram принимает только публичные https-ссылки
	}
	return site + "/store/orders/" + id.String()
}

func withOpenButton(m outbox.Message, site string, id uuid.UUID) outbox.Message {
	if u := orderURL(site, id); u != "" {
		m.Buttons = append(m.Buttons, []outbox.Button{{Text: "Открыть заказ", URL: u}})
	}
	return m
}

func newOrderMessage(site string, id uuid.UUID, number int64, lines []msgLine, total int64, timeComment string) outbox.Message {
	var b strings.Builder
	fmt.Fprintf(&b, "🛍 Новый заказ №%d\n\n", number)
	for _, l := range lines {
		b.WriteString("• " + l.title)
		if l.size != "" {
			b.WriteString(", размер " + l.size)
		}
		fmt.Fprintf(&b, " × %d — %s\n", l.qty, FormatSum(l.price*int64(l.qty)))
	}
	fmt.Fprintf(&b, "\nИтого: %s", FormatSum(total))
	if timeComment != "" {
		b.WriteString("\nКогда доставить: " + timeComment)
	}
	b.WriteString("\n\nПодтвердите заказ, чтобы клиент знал, что вы его получили.")
	m := outbox.Message{Text: b.String(), Buttons: [][]outbox.Button{{{Text: "✅ Подтвердить", Data: CallbackConfirm + id.String()}}}}
	return withOpenButton(m, site, id)
}

func cancelledMessage(site string, id uuid.UUID, number int64, reason string) outbox.Message {
	text := fmt.Sprintf("❌ Клиент отменил заказ №%d.", number)
	if reason != "" {
		text += "\nПричина: " + reason
	}
	text += "\nТовары вернулись в остатки."
	return withOpenButton(outbox.Message{Text: text}, site, id)
}

func reminderMessage(site string, id uuid.UUID, number int64, waited time.Duration) outbox.Message {
	text := fmt.Sprintf("⏰ Заказ №%d ждёт подтверждения уже %d мин. Клиент ждёт ответа.", number, int(waited.Minutes()))
	m := outbox.Message{Text: text, Buttons: [][]outbox.Button{{{Text: "✅ Подтвердить", Data: CallbackConfirm + id.String()}}}}
	return withOpenButton(m, site, id)
}

// statusSMS — текст SMS клиенту на его языке. Для остальных статусов SMS не
// отправляется. Тексты нужно согласовать в кабинете Eskiz точно в таком виде
// (docs/NOTIFICATIONS.md).
func statusSMS(number int64, to Status, lang string) string {
	uz := map[Status]string{
		StatusConfirmed:  "Buyurtma #%d do'kon tomonidan tasdiqlandi.",
		StatusInDelivery: "Buyurtma #%d yetkazib berishga topshirildi.",
		StatusRejected:   "Buyurtma #%d do'kon tomonidan bekor qilindi.",
		StatusCancelled:  "Buyurtma #%d do'kon tomonidan bekor qilindi.",
	}
	ru := map[Status]string{
		StatusConfirmed:  "Заказ #%d подтверждён магазином.",
		StatusInDelivery: "Заказ #%d передан в доставку.",
		StatusRejected:   "Заказ #%d отменён магазином.",
		StatusCancelled:  "Заказ #%d отменён магазином.",
	}
	t := ru
	if lang == "uz" {
		t = uz
	}
	if f, ok := t[to]; ok {
		return fmt.Sprintf(f, number)
	}
	return ""
}

// RemindPending — один раз напоминает магазину в Telegram о заказе, который не
// подтверждён дольше after. Возвращает число напоминаний.
func RemindPending(ctx context.Context, pool *pgxpool.Pool, site string, after time.Duration, now time.Time) (n int64, err error) {
	err = db.Tx(ctx, pool, func(tx pgx.Tx) error {
		n, err = remindPending(ctx, tx, site, after, now)
		return err
	})
	return n, err
}

func remindPending(ctx context.Context, tx pgx.Tx, site string, after time.Duration, now time.Time) (int64, error) {
	rows, err := tx.Query(ctx, `UPDATE orders o SET store_reminded_at = $2
		FROM stores s
		WHERE s.id = o.store_id AND s.telegram_chat_id IS NOT NULL
		  AND o.status = 'pending' AND o.store_reminded_at IS NULL AND o.created_at < $1
		RETURNING o.id, o.number, o.store_id, o.created_at`, now.Add(-after), now)
	if err != nil {
		return 0, err
	}
	type due struct {
		id, store uuid.UUID
		number    int64
		created   time.Time
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.number, &d.store, &d.created); err != nil {
			rows.Close()
			return 0, err
		}
		list = append(list, d)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, d := range list {
		if err := outbox.ToStoreTelegram(ctx, tx, d.store, reminderMessage(site, d.id, d.number, now.Sub(d.created))); err != nil {
			return 0, err
		}
	}
	return int64(len(list)), nil
}
