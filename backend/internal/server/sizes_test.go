package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/boutique-market/backend/internal/orders"
	"github.com/boutique-market/backend/internal/outbox"
	"github.com/boutique-market/backend/internal/telegram"
)

func stockOf(t *testing.T, e *env, productID, size string) (stock int, inStock bool) {
	t.Helper()
	err := e.d.DB.QueryRow(context.Background(), `SELECT ps.stock, p.in_stock FROM product_sizes ps JOIN products p ON p.id = ps.product_id
		WHERE ps.product_id = $1 AND ps.size = $2`, productID, size).Scan(&stock, &inStock)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestSizesStockCartCheckoutAndClientCancel(t *testing.T) {
	f := newOrderFixture(t)
	e := f.e

	// магазин задаёт размеры: «m» нормализуется в «M»
	r := f.storeA.must(200, "PATCH", "/v1/store/products/"+f.prodA, map[string]any{
		"sizes": []map[string]any{{"size": " m ", "stock": 2}, {"size": "L", "stock": 0}}})
	sizes := r.data()["sizes"].([]any)
	if len(sizes) != 2 || sizes[0].(map[string]any)["size"] != "M" || sizes[0].(map[string]any)["stock"].(float64) != 2 {
		t.Fatalf("store sizes: %v", sizes)
	}
	if r := f.storeA.do("PATCH", "/v1/store/products/"+f.prodA, map[string]any{
		"sizes": []map[string]any{{"size": "S", "stock": 1}, {"size": "s", "stock": 1}}}); r.Status != 422 {
		t.Fatalf("duplicate sizes accepted: %d %s", r.Status, r.Raw)
	}
	// покупатель видит наличие размера, но не точный остаток
	pub := e.client().must(200, "GET", "/v1/products/"+f.prodA, nil).data()
	ps := pub["sizes"].([]any)
	m := ps[0].(map[string]any)
	if _, leak := m["stock"]; leak || m["in_stock"] != true || m["low"] != true || ps[1].(map[string]any)["in_stock"] != false {
		t.Fatalf("public sizes: %v", ps)
	}

	c := e.client()
	c.loginClient("+998901112233")
	if r := c.do("PUT", "/v1/me/cart/"+f.prodA, map[string]any{"qty": 1}); r.errCode() != "size_required" {
		t.Fatalf("no size: %s", r.Raw)
	}
	if r := c.do("PUT", "/v1/me/cart/"+f.prodA+"?size=L", map[string]any{"qty": 1}); r.errCode() != "out_of_stock" {
		t.Fatalf("size L: %s", r.Raw)
	}
	if r := c.do("PUT", "/v1/me/cart/"+f.prodA+"?size=XL", map[string]any{"qty": 1}); r.errCode() != "size_unknown" {
		t.Fatalf("size XL: %s", r.Raw)
	}
	if r := c.do("PUT", "/v1/me/cart/"+f.prodA+"?size=m", map[string]any{"qty": 3}); r.errCode() != "not_enough_stock" {
		t.Fatalf("qty > stock: %s", r.Raw)
	}
	if r := c.do("PUT", "/v1/me/cart/"+f.prodA2+"?size=M", map[string]any{"qty": 1}); r.errCode() != "size_unknown" {
		t.Fatalf("size on product without sizes: %s", r.Raw)
	}
	cart := c.must(200, "PUT", "/v1/me/cart/"+f.prodA+"?size=m", map[string]any{"qty": 2}).data()
	ci := cart["items"].([]any)[0].(map[string]any)
	if ci["size"] != "M" || ci["available"] != true || cart["can_checkout"] != true {
		t.Fatalf("cart: %v", cart)
	}
	c.must(200, "PUT", "/v1/me/cart/"+f.prodA2, map[string]any{"qty": 1})

	// без размера заказ не оформляется
	if r := c.doH("POST", "/v1/orders", checkoutBody(item(f.prodA, 1)), idem("size-missing-1")); r.errCode() != "products_unavailable" ||
		!strings.Contains(r.Raw, "size_required") {
		t.Fatalf("checkout without size: %s", r.Raw)
	}
	withSize := map[string]any{"product_id": f.prodA, "size": "M", "qty": 2}
	r = c.mustH(201, "POST", "/v1/orders", checkoutBody(withSize, item(f.prodA2, 1)), idem("size-order-1"))
	orderID := r.data()["orders"].([]any)[0].(map[string]any)["id"].(string)
	if st, in := stockOf(t, e, f.prodA, "M"); st != 0 || in {
		t.Fatalf("stock after order: %d in_stock=%v", st, in)
	}
	if got := c.must(200, "GET", "/v1/me/cart", nil).data()["items"].([]any); len(got) != 0 {
		t.Fatalf("cart not cleared: %v", got)
	}
	// второй покупатель не купит то, чего уже нет
	c2 := e.client()
	c2.loginClient("+998901112244")
	if r := c2.doH("POST", "/v1/orders", checkoutBody(withSize), idem("size-order-2")); r.errCode() != "products_unavailable" {
		t.Fatalf("oversold: %s", r.Raw)
	}

	d := c.must(200, "GET", "/v1/orders/"+orderID, nil).data()
	if d["can_cancel"] != true {
		t.Fatalf("can_cancel: %v", d["can_cancel"])
	}
	var sawSize bool
	for _, it := range d["items"].([]any) {
		if it.(map[string]any)["size"] == "M" {
			sawSize = true
		}
	}
	if !sawSize {
		t.Fatalf("order items: %v", d["items"])
	}
	// чужой заказ отменить нельзя
	if r := c2.do("POST", "/v1/orders/"+orderID+"/cancel", map[string]any{}); r.Status != 404 {
		t.Fatalf("foreign cancel: %d", r.Status)
	}
	// клиент отменяет — остаток возвращается, магазин получает уведомление
	d = c.must(200, "POST", "/v1/orders/"+orderID+"/cancel", map[string]any{"reason": "передумал"}).data()
	if d["status"] != "cancelled" || d["can_cancel"] == true {
		t.Fatalf("cancelled: %v %v", d["status"], d["can_cancel"])
	}
	if st, in := stockOf(t, e, f.prodA, "M"); st != 2 || !in {
		t.Fatalf("stock not restored: %d %v", st, in)
	}
	var n int
	_ = e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM notifications n JOIN users u ON u.id = n.user_id
		WHERE u.store_id = $1 AND n.kind = 'order_cancelled'`, f.storeAID).Scan(&n)
	if n != 1 {
		t.Fatalf("store cancel notifications: %d", n)
	}
	// после начала сборки отмена клиентом невозможна
	r = c.mustH(201, "POST", "/v1/orders", checkoutBody(map[string]any{"product_id": f.prodA, "size": "M", "qty": 1}), idem("size-order-3"))
	id2 := r.data()["orders"].([]any)[0].(map[string]any)["id"].(string)
	f.storeA.must(200, "POST", "/v1/store/orders/"+id2+"/status", map[string]any{"status": "confirmed"})
	f.storeA.must(200, "POST", "/v1/store/orders/"+id2+"/status", map[string]any{"status": "assembling"})
	if r := c.do("POST", "/v1/orders/"+id2+"/cancel", nil); r.errCode() != "cannot_cancel" {
		t.Fatalf("cancel after assembling: %s", r.Raw)
	}
	// отказ магазина тоже возвращает остаток
	f.storeA.must(200, "POST", "/v1/store/orders/"+id2+"/status", map[string]any{"status": "cancelled", "reason": "брак"})
	if st, _ := stockOf(t, e, f.prodA, "M"); st != 2 {
		t.Fatalf("stock after store cancel: %d", st)
	}
	// убрать размеры — товар снова управляется флажком
	f.storeA.must(200, "PATCH", "/v1/store/products/"+f.prodA, map[string]any{"sizes": []any{}, "in_stock": true})
	c.must(200, "PUT", "/v1/me/cart/"+f.prodA, map[string]any{"qty": 1})
}

// fakeTelegram — Bot API, который запоминает вызовы.
type fakeTelegram struct {
	mu    sync.Mutex
	calls []map[string]any
	paths []string
}

func (f *fakeTelegram) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.calls = append(f.calls, body)
	f.paths = append(f.paths, r.URL.Path)
	f.mu.Unlock()
	_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
}

func (f *fakeTelegram) find(method string) []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []map[string]any
	for i, p := range f.paths {
		if strings.HasSuffix(p, "/"+method) {
			out = append(out, f.calls[i])
		}
	}
	return out
}

const tgSecret = "test-secret-0123456789"

func TestTelegramBotLinkConfirmAndReminders(t *testing.T) {
	fake := &fakeTelegram{}
	api := httptest.NewServer(fake)
	t.Cleanup(api.Close)
	bot := &telegram.Bot{Token: "123:ABC", BaseURL: api.URL}
	f := newOrderFixture(t, func(d *Deps) {
		d.Telegram = bot
		d.Cfg.TelegramBotUsername = "boutique_test_bot"
		d.Cfg.TelegramWebhookSecret = tgSecret
		d.Cfg.SiteURL = "https://shop.test"
		d.Cfg.OrderSMS = true
	})
	e := f.e
	ctx := context.Background()
	sms := &smsLog{}
	worker := &outbox.Worker{DB: e.d.DB, Log: slog.New(slog.NewJSONHandler(io.Discard, nil)), Telegram: bot, SMS: sms}
	flush := func() {
		t.Helper()
		if _, err := worker.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	hook := func(secret string, upd map[string]any) int {
		b, _ := json.Marshal(upd)
		req, _ := http.NewRequest("POST", e.srv.URL+"/v1/telegram/webhook", strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	const chat = 555001

	st := f.storeA.must(200, "GET", "/v1/store/telegram", nil).data()
	if st["available"] != true || st["connected"] != false {
		t.Fatalf("status: %v", st)
	}
	link := f.storeA.must(200, "POST", "/v1/store/telegram/link", nil).data()["url"].(string)
	code := link[strings.Index(link, "start=")+6:]
	if !strings.HasPrefix(link, "https://t.me/boutique_test_bot?start=") || len(code) < 20 {
		t.Fatalf("link: %s", link)
	}
	start := map[string]any{"message": map[string]any{"chat": map[string]any{"id": chat, "type": "private"}, "text": "/start " + code}}
	if s := hook("wrong-secret-000000000", start); s != 404 {
		t.Fatalf("bad secret accepted: %d", s)
	}
	if s := hook(tgSecret, start); s != 200 {
		t.Fatalf("webhook: %d", s)
	}
	if f.storeA.must(200, "GET", "/v1/store/telegram", nil).data()["connected"] != true {
		t.Fatal("store not connected")
	}
	// код одноразовый
	hook(tgSecret, map[string]any{"message": map[string]any{"chat": map[string]any{"id": 999}, "text": "/start " + code}})
	var linked int64
	_ = e.d.DB.QueryRow(ctx, `SELECT telegram_chat_id FROM stores WHERE id = $1`, f.storeAID).Scan(&linked)
	if linked != chat {
		t.Fatalf("chat id: %d", linked)
	}

	c := e.client()
	c.loginClient("+998901119900")
	r := c.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 1), item(f.prodB, 1)), idem("tg-order-1"))
	var orderA string
	for _, o := range r.data()["orders"].([]any) {
		if o.(map[string]any)["store_id"] == f.storeAID {
			orderA = o.(map[string]any)["id"].(string)
		}
	}
	flush()
	sent := fake.find("sendMessage")
	var newOrder map[string]any
	for _, m := range sent {
		if strings.Contains(m["text"].(string), "Новый заказ") {
			newOrder = m
		}
	}
	if newOrder == nil || newOrder["chat_id"].(float64) != chat {
		t.Fatalf("no new-order message: %v", sent)
	}
	if strings.Contains(newOrder["text"].(string), "+998901119900") || strings.Contains(newOrder["text"].(string), "Навои") {
		t.Fatalf("customer PII in telegram: %s", newOrder["text"])
	}
	kb, _ := json.Marshal(newOrder["reply_markup"])
	if !strings.Contains(string(kb), orders.CallbackConfirm+orderA) || !strings.Contains(string(kb), "https://shop.test/store/orders/"+orderA) {
		t.Fatalf("keyboard: %s", kb)
	}
	// магазин B бот не подключал — ему ничего не уходит
	var queued int
	_ = e.d.DB.QueryRow(ctx, `SELECT count(*) FROM outbound_messages WHERE channel = 'telegram' AND recipient <> $1`, "555001").Scan(&queued)
	if queued != 1 { // только ответ на повторный /start из чата 999
		t.Fatalf("unexpected telegram recipients: %d", queued)
	}

	cb := func(chatID int, data string) {
		hook(tgSecret, map[string]any{"callback_query": map[string]any{"id": "cb1", "data": data,
			"message": map[string]any{"message_id": 10, "text": "Новый заказ", "chat": map[string]any{"id": chatID}}}})
	}
	cb(777, orders.CallbackConfirm+orderA) // чужой чат
	if st := c.must(200, "GET", "/v1/orders/"+orderA, nil).data()["status"]; st != "pending" {
		t.Fatalf("foreign chat confirmed order: %v", st)
	}
	cb(chat, orders.CallbackConfirm+orderA)
	if st := c.must(200, "GET", "/v1/orders/"+orderA, nil).data()["status"]; st != "confirmed" {
		t.Fatalf("status after telegram confirm: %v", st)
	}
	if len(fake.find("editMessageText")) != 1 {
		t.Fatal("message not marked as confirmed")
	}
	// SMS клиенту о подтверждении
	flush()
	if len(sms.sent) != 1 || sms.sent[0].phone != "+998901119900" || !strings.Contains(sms.sent[0].text, "подтверждён") {
		t.Fatalf("sms: %+v", sms.sent)
	}

	// напоминание о неподтверждённом заказе — один раз
	r = c.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA2, 1)), idem("tg-order-2"))
	id2 := r.data()["orders"].([]any)[0].(map[string]any)["id"].(string)
	_, _ = e.d.DB.Exec(ctx, `UPDATE orders SET created_at = now() - interval '40 minutes' WHERE id = $1`, id2)
	for i, want := range []int64{1, 0} {
		n, err := orders.RemindPending(ctx, e.d.DB, "https://shop.test", 30*time.Minute, time.Now())
		if err != nil || n != want {
			t.Fatalf("remind #%d: %d %v", i, n, err)
		}
	}
	// напоминание уходит в Telegram
	_ = e.d.DB.QueryRow(ctx, `SELECT count(*) FROM outbound_messages WHERE sent_at IS NULL`).Scan(&queued)
	if queued == 0 {
		t.Fatal("reminder not queued")
	}
	flush()
	_ = e.d.DB.QueryRow(ctx, `SELECT count(*) FROM outbound_messages WHERE sent_at IS NULL AND failed_at IS NULL`).Scan(&queued)
	if queued != 0 {
		t.Fatalf("unsent after flush: %d", queued)
	}

	f.storeA.must(204, "DELETE", "/v1/store/telegram", nil)
	if f.storeA.must(200, "GET", "/v1/store/telegram", nil).data()["connected"] != false {
		t.Fatal("unlink failed")
	}
}

type smsLog struct {
	sent []struct{ phone, text string }
}

func (s *smsLog) SendText(_ context.Context, phone, text string) error {
	s.sent = append(s.sent, struct{ phone, text string }{phone, text})
	return nil
}

func TestOutboxRetriesAndDropsPermanent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if err := outbox.ToTelegramChat(ctx, e.d.DB, 42, outbox.Message{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	var calls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
			return
		}
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`))
	}))
	t.Cleanup(api.Close)
	var dropped string
	w := &outbox.Worker{DB: e.d.DB, Log: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Telegram:    &telegram.Bot{Token: "x", BaseURL: api.URL},
		OnPermanent: func(_ context.Context, _, rcpt string) { dropped = rcpt }}
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var attempts int
	var nextAt time.Time
	_ = e.d.DB.QueryRow(ctx, `SELECT attempts, next_at FROM outbound_messages`).Scan(&attempts, &nextAt)
	if attempts != 1 || time.Until(nextAt) < 5*time.Second {
		t.Fatalf("retry not scheduled: %d %v", attempts, nextAt)
	}
	_, _ = e.d.DB.Exec(ctx, `UPDATE outbound_messages SET next_at = now()`)
	if _, err := w.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var failed bool
	_ = e.d.DB.QueryRow(ctx, `SELECT failed_at IS NOT NULL FROM outbound_messages`).Scan(&failed)
	if !failed || dropped != "42" {
		t.Fatalf("permanent error: failed=%v dropped=%q", failed, dropped)
	}
}
