package payments

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/testutil"
)

func TestParseSums(t *testing.T) {
	cases := map[string]int64{"15000": 1500000, "15000.0": 1500000, "15000.00": 1500000, "15000.5": 1500050, "1.05": 105, "0.01": 1}
	for in, want := range cases {
		got, ok := parseSums(in)
		if !ok || got != want {
			t.Errorf("parseSums(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "-1", "1.001", "1e3", "abc", "1,00", "0", ".5", "1.2.3"} {
		if _, ok := parseSums(bad); ok {
			t.Errorf("parseSums(%q) accepted", bad)
		}
	}
	if s := formatSums(1500005); s != "15000.05" {
		t.Errorf("formatSums = %s", s)
	}
}

func TestIPAllow(t *testing.T) {
	a, err := parseAllow([]string{"185.8.212.0/24", "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.allows("185.8.212.77") || !a.allows("10.0.0.1") || a.allows("10.0.0.2") || a.allows("garbage") {
		t.Fatal("allow list mismatch")
	}
	if !(ipAllow(nil)).allows("1.2.3.4") {
		t.Fatal("empty list must allow")
	}
	if _, err := parseAllow([]string{"nope"}); err == nil {
		t.Fatal("expected error")
	}
}

// ---------- интеграционные ----------

type fixture struct {
	pool   *pgxpool.Pool
	userID uuid.UUID
	tariff uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	pool := testutil.DB(t)
	ctx := context.Background()
	f := &fixture{pool: pool}
	if err := pool.QueryRow(ctx, `INSERT INTO users (role, phone) VALUES ('client', '+998901112233') RETURNING id`).Scan(&f.userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO tariffs (name_ru, name_uz, tokens, price, is_published)
		VALUES ('Старт', 'Start', 50, 1500000, true) RETURNING id`).Scan(&f.tariff); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) payment(t *testing.T, provider string) uuid.UUID {
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO payments (user_id, tariff_id, provider, amount, tokens)
		VALUES ($1, $2, $3, 1500000, 50) RETURNING id`, f.userID, f.tariff, provider).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) balance(t *testing.T) int64 {
	var b int64
	if err := f.pool.QueryRow(context.Background(), `SELECT token_balance FROM users WHERE id = $1`, f.userID).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func (f *fixture) status(t *testing.T, id uuid.UUID) string {
	var s string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM payments WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fixture) ledgerCount(t *testing.T, id uuid.UUID) int {
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM token_ledger WHERE reference_type = 'payment' AND reference_id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// ---------- Click ----------

const clickSecret = "click-secret"

type clickCall struct {
	h       *Click
	payment uuid.UUID
}

func (c clickCall) do(t *testing.T, complete bool, v url.Values) clickResp {
	t.Helper()
	if v.Get("sign_string") == "" {
		parts := []string{v.Get("click_trans_id"), v.Get("service_id"), clickSecret, v.Get("merchant_trans_id")}
		if complete {
			parts = append(parts, v.Get("merchant_prepare_id"))
		}
		parts = append(parts, v.Get("amount"), v.Get("action"), v.Get("sign_time"))
		v.Set("sign_string", clickSign(parts...))
	}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(v.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	if complete {
		c.h.Complete(w, r)
	} else {
		c.h.Prepare(w, r)
	}
	var resp clickResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json %s", w.Body.String())
	}
	return resp
}

func clickForm(clickTxn int64, pid uuid.UUID, amount string, action string) url.Values {
	return url.Values{
		"click_trans_id": {strconv.FormatInt(clickTxn, 10)}, "service_id": {"777"}, "click_paydoc_id": {"1"},
		"merchant_trans_id": {pid.String()}, "amount": {amount}, "action": {action}, "error": {"0"},
		"error_note": {"Success"}, "sign_time": {"2026-11-02 12:00:00"},
	}
}

func TestClick_FullFlowCreditsOnce(t *testing.T) {
	f := newFixture(t)
	pid := f.payment(t, ProviderClick)
	c := clickCall{h: &Click{DB: f.pool, ServiceID: "777", SecretKey: clickSecret}, payment: pid}

	prep := c.do(t, false, clickForm(1001, pid, "15000.00", "0"))
	if prep.Error != 0 || prep.MerchantPrepareID == nil {
		t.Fatalf("prepare: %+v", prep)
	}
	again := c.do(t, false, clickForm(1001, pid, "15000.00", "0"))
	if again.Error != 0 || *again.MerchantPrepareID != *prep.MerchantPrepareID {
		t.Fatalf("repeat prepare must be idempotent: %+v", again)
	}
	v := clickForm(1001, pid, "15000", "1")
	v.Set("merchant_prepare_id", strconv.FormatInt(*prep.MerchantPrepareID, 10))
	done := c.do(t, true, v)
	if done.Error != 0 || done.MerchantConfirmID == nil {
		t.Fatalf("complete: %+v", done)
	}
	if f.balance(t) != 50 || f.status(t, pid) != "paid" {
		t.Fatalf("balance %d status %s", f.balance(t), f.status(t, pid))
	}
	// повтор Complete: успех, но без второго зачисления
	v.Del("sign_string")
	if r := c.do(t, true, v); r.Error != 0 {
		t.Fatalf("repeat complete: %+v", r)
	}
	if f.balance(t) != 50 || f.ledgerCount(t, pid) != 1 {
		t.Fatalf("double credit: balance %d", f.balance(t))
	}
	// новый Prepare по оплаченному платежу
	if r := c.do(t, false, clickForm(1002, pid, "15000", "0")); r.Error != clickAlreadyPaid {
		t.Fatalf("prepare after paid: %+v", r)
	}
}

func TestClick_Rejections(t *testing.T) {
	f := newFixture(t)
	pid := f.payment(t, ProviderClick)
	c := clickCall{h: &Click{DB: f.pool, ServiceID: "777", SecretKey: clickSecret}, payment: pid}

	bad := clickForm(2001, pid, "15000", "0")
	bad.Set("sign_string", strings.Repeat("0", 32))
	if r := c.do(t, false, bad); r.Error != clickSignFailed {
		t.Fatalf("bad sign: %+v", r)
	}
	if r := c.do(t, false, clickForm(2001, pid, "14999", "0")); r.Error != clickBadAmount {
		t.Fatalf("bad amount: %+v", r)
	}
	if r := c.do(t, false, clickForm(2001, uuid.New(), "15000", "0")); r.Error != clickOrderNotFound {
		t.Fatalf("unknown payment: %+v", r)
	}
	if r := c.do(t, false, clickForm(2001, pid, "15000", "1")); r.Error != clickActionNotFound {
		t.Fatalf("wrong action: %+v", r)
	}
	// платёж Payme нельзя оплатить через Click
	if r := c.do(t, false, clickForm(2001, f.payment(t, ProviderPayme), "15000", "0")); r.Error != clickOrderNotFound {
		t.Fatalf("foreign provider: %+v", r)
	}
	// выключенный провайдер (пустой секрет) отклоняет всё, даже подпись с пустым ключом
	off := clickCall{h: &Click{DB: f.pool, ServiceID: "777"}}
	if r := off.do(t, false, clickForm(2001, pid, "15000", "0")); r.Error != clickSignFailed {
		t.Fatalf("disabled: %+v", r)
	}
	// Complete без Prepare
	v := clickForm(2001, pid, "15000", "1")
	v.Set("merchant_prepare_id", "999999")
	if r := c.do(t, true, v); r.Error != clickTxnNotFound {
		t.Fatalf("complete unknown: %+v", r)
	}
	if f.balance(t) != 0 {
		t.Fatal("nothing must be credited")
	}
}

func TestClick_CompleteWithProviderErrorDeclines(t *testing.T) {
	f := newFixture(t)
	pid := f.payment(t, ProviderClick)
	c := clickCall{h: &Click{DB: f.pool, ServiceID: "777", SecretKey: clickSecret}}
	prep := c.do(t, false, clickForm(3001, pid, "15000", "0"))
	v := clickForm(3001, pid, "15000", "1")
	v.Set("merchant_prepare_id", strconv.FormatInt(*prep.MerchantPrepareID, 10))
	v.Set("error", "-5017")
	if r := c.do(t, true, v); r.Error != clickTxnCancelled {
		t.Fatalf("complete with error: %+v", r)
	}
	if f.status(t, pid) != "declined" || f.balance(t) != 0 {
		t.Fatalf("status %s", f.status(t, pid))
	}
	v.Set("error", "0")
	v.Del("sign_string")
	if r := c.do(t, true, v); r.Error != clickTxnCancelled {
		t.Fatalf("complete after cancel: %+v", r)
	}
}

func TestClick_UnparsableErrorFieldIsFailure(t *testing.T) {
	f := newFixture(t)
	pid := f.payment(t, ProviderClick)
	c := clickCall{h: &Click{DB: f.pool, ServiceID: "777", SecretKey: clickSecret}}
	prep := c.do(t, false, clickForm(3501, pid, "15000", "0"))
	v := clickForm(3501, pid, "15000", "1")
	v.Set("merchant_prepare_id", strconv.FormatInt(*prep.MerchantPrepareID, 10))
	v.Set("error", "")
	if r := c.do(t, true, v); r.Error != clickTxnCancelled || f.balance(t) != 0 {
		t.Fatalf("empty error field must not credit: %+v", r)
	}
}

func TestClick_ConcurrentCompleteCreditsOnce(t *testing.T) {
	f := newFixture(t)
	pid := f.payment(t, ProviderClick)
	c := clickCall{h: &Click{DB: f.pool, ServiceID: "777", SecretKey: clickSecret}}
	prep := c.do(t, false, clickForm(4001, pid, "15000", "0"))
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := clickForm(4001, pid, "15000", "1")
			v.Set("merchant_prepare_id", strconv.FormatInt(*prep.MerchantPrepareID, 10))
			if r := c.do(t, true, v); r.Error != 0 {
				t.Errorf("complete: %+v", r)
			}
		}()
	}
	wg.Wait()
	if f.balance(t) != 50 || f.ledgerCount(t, pid) != 1 {
		t.Fatalf("balance %d", f.balance(t))
	}
}

// ---------- Payme ----------

const paymeKey = "payme-test-key"

type paymeClient struct {
	h *Payme
}

func (p paymeClient) call(t *testing.T, auth string, method string, params any) (map[string]any, *rpcError) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 42, "method": method, "params": params})
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	if auth == "" {
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte("Paycom:"+paymeKey))
	}
	r.Header.Set("Authorization", auth)
	w := httptest.NewRecorder()
	p.h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("http %d", w.Code)
	}
	var resp struct {
		ID     int            `json:"id"`
		Result map[string]any `json:"result"`
		Error  *rpcError      `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != 42 {
		t.Fatalf("id not echoed: %s", w.Body.String())
	}
	return resp.Result, resp.Error
}

func code(e *rpcError) int {
	if e == nil {
		return 0
	}
	return e.Code
}

func newPayme(f *fixture, now *time.Time) paymeClient {
	h := &Payme{DB: f.pool, Key: paymeKey, AccountKey: "payment_id"}
	if now != nil {
		h.Now = func() time.Time { return *now }
	}
	return paymeClient{h: h}
}

func acc(id uuid.UUID) map[string]any { return map[string]any{"payment_id": id.String()} }

func TestPayme_Auth(t *testing.T) {
	f := newFixture(t)
	p := newPayme(f, nil)
	for _, a := range []string{
		"Basic " + base64.StdEncoding.EncodeToString([]byte("Paycom:wrong")),
		"Basic " + base64.StdEncoding.EncodeToString([]byte("Other:"+paymeKey)),
		"Bearer x", "Basic !!!",
	} {
		if _, e := p.call(t, a, "CheckTransaction", map[string]any{"id": "x"}); code(e) != rpcAuth {
			t.Errorf("%q: got %v", a, e)
		}
	}
	off := paymeClient{h: &Payme{DB: f.pool, AccountKey: "payment_id"}}
	if _, e := off.call(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("Paycom:")), "CheckTransaction", map[string]any{"id": "x"}); code(e) != rpcAuth {
		t.Errorf("disabled payme must reject: %v", e)
	}
	if _, e := p.call(t, "", "Nope", map[string]any{}); code(e) != rpcMethod {
		t.Errorf("unknown method: %v", e)
	}
}

func TestPayme_FullFlow(t *testing.T) {
	f := newFixture(t)
	now := time.Now()
	p := newPayme(f, &now)
	pid := f.payment(t, ProviderPayme)

	if _, e := p.call(t, "", "CheckPerformTransaction", map[string]any{"amount": 1500000, "account": acc(pid)}); e != nil {
		t.Fatalf("check perform: %v", e)
	}
	if _, e := p.call(t, "", "CheckPerformTransaction", map[string]any{"amount": 100, "account": acc(pid)}); code(e) != paymeBadAmount {
		t.Fatalf("wrong amount: %v", e)
	}
	if _, e := p.call(t, "", "CheckPerformTransaction", map[string]any{"amount": 1500000, "account": acc(uuid.New())}); code(e) != paymeNoOrder {
		t.Fatalf("unknown account: %v", e)
	}
	create := map[string]any{"id": "pm-1", "time": now.UnixMilli(), "amount": 1500000, "account": acc(pid)}
	res, e := p.call(t, "", "CreateTransaction", create)
	if e != nil || res["state"].(float64) != 1 {
		t.Fatalf("create: %v %v", res, e)
	}
	res2, e := p.call(t, "", "CreateTransaction", create)
	if e != nil || res2["transaction"] != res["transaction"] || res2["create_time"] != res["create_time"] {
		t.Fatalf("repeat create must be idempotent: %v %v", res2, e)
	}
	// вторая транзакция на тот же платёж
	if _, e := p.call(t, "", "CreateTransaction", map[string]any{"id": "pm-2", "time": now.UnixMilli(), "amount": 1500000, "account": acc(pid)}); code(e) != paymeOrderBusy {
		t.Fatalf("busy: %v", e)
	}
	perf, e := p.call(t, "", "PerformTransaction", map[string]any{"id": "pm-1"})
	if e != nil || perf["state"].(float64) != 2 || perf["perform_time"].(float64) == 0 {
		t.Fatalf("perform: %v %v", perf, e)
	}
	perf2, e := p.call(t, "", "PerformTransaction", map[string]any{"id": "pm-1"})
	if e != nil || perf2["perform_time"] != perf["perform_time"] {
		t.Fatalf("repeat perform: %v %v", perf2, e)
	}
	if f.balance(t) != 50 || f.ledgerCount(t, pid) != 1 || f.status(t, pid) != "paid" {
		t.Fatalf("balance %d", f.balance(t))
	}
	if _, e := p.call(t, "", "CancelTransaction", map[string]any{"id": "pm-1", "reason": 5}); code(e) != paymeCantCancel {
		t.Fatalf("cancel performed: %v", e)
	}
	chk, e := p.call(t, "", "CheckTransaction", map[string]any{"id": "pm-1"})
	if e != nil || chk["state"].(float64) != 2 || chk["reason"] != nil {
		t.Fatalf("check: %v %v", chk, e)
	}
	if _, e := p.call(t, "", "CheckPerformTransaction", map[string]any{"amount": 1500000, "account": acc(pid)}); code(e) != paymeOrderDone {
		t.Fatalf("check perform paid: %v", e)
	}
	st, e := p.call(t, "", "GetStatement", map[string]any{"from": now.Add(-time.Hour).UnixMilli(), "to": now.Add(time.Hour).UnixMilli()})
	if e != nil || len(st["transactions"].([]any)) != 1 {
		t.Fatalf("statement: %v %v", st, e)
	}
	if _, e := p.call(t, "", "CheckTransaction", map[string]any{"id": "missing"}); code(e) != paymeTxnMissing {
		t.Fatalf("missing: %v", e)
	}
}

func TestPayme_CancelAndTimeout(t *testing.T) {
	f := newFixture(t)
	now := time.Now()
	p := newPayme(f, &now)

	// отмена созданной транзакции
	pid := f.payment(t, ProviderPayme)
	if _, e := p.call(t, "", "CreateTransaction", map[string]any{"id": "c-1", "time": now.UnixMilli(), "amount": 1500000, "account": acc(pid)}); e != nil {
		t.Fatal(e)
	}
	res, e := p.call(t, "", "CancelTransaction", map[string]any{"id": "c-1", "reason": 3})
	if e != nil || res["state"].(float64) != -1 || res["cancel_time"].(float64) == 0 {
		t.Fatalf("cancel: %v %v", res, e)
	}
	res2, e := p.call(t, "", "CancelTransaction", map[string]any{"id": "c-1", "reason": 3})
	if e != nil || res2["cancel_time"] != res["cancel_time"] {
		t.Fatalf("repeat cancel: %v %v", res2, e)
	}
	if _, e := p.call(t, "", "PerformTransaction", map[string]any{"id": "c-1"}); code(e) != paymeCantDo {
		t.Fatalf("perform cancelled: %v", e)
	}
	if f.status(t, pid) != "cancelled" {
		t.Fatal("payment must be cancelled")
	}

	// таймаут 12 ч: Perform отменяет транзакцию, и отмена сохраняется
	pid2 := f.payment(t, ProviderPayme)
	if _, e := p.call(t, "", "CreateTransaction", map[string]any{"id": "t-1", "time": now.UnixMilli(), "amount": 1500000, "account": acc(pid2)}); e != nil {
		t.Fatal(e)
	}
	now = now.Add(13 * time.Hour)
	if _, e := p.call(t, "", "PerformTransaction", map[string]any{"id": "t-1"}); code(e) != paymeCantDo {
		t.Fatalf("expired perform: %v", e)
	}
	chk, _ := p.call(t, "", "CheckTransaction", map[string]any{"id": "t-1"})
	if chk["state"].(float64) != -1 || chk["reason"].(float64) != 4 {
		t.Fatalf("timeout cancel not persisted: %v", chk)
	}
	if f.balance(t) != 0 {
		t.Fatal("nothing credited")
	}
}

func TestPayme_ConcurrentPerformCreditsOnce(t *testing.T) {
	f := newFixture(t)
	p := newPayme(f, nil)
	pid := f.payment(t, ProviderPayme)
	if _, e := p.call(t, "", "CreateTransaction", map[string]any{"id": "r-1", "time": time.Now().UnixMilli(), "amount": 1500000, "account": acc(pid)}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := p.call(t, "", "PerformTransaction", map[string]any{"id": "r-1"}); e != nil {
				t.Errorf("perform: %v", e)
			}
		}()
	}
	wg.Wait()
	if f.balance(t) != 50 || f.ledgerCount(t, pid) != 1 {
		t.Fatalf("balance %d", f.balance(t))
	}
}

func TestPayme_ConcurrentCreateOneWins(t *testing.T) {
	f := newFixture(t)
	p := newPayme(f, nil)
	pid := f.payment(t, ProviderPayme)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := p.call(t, "", "CreateTransaction", map[string]any{"id": fmt.Sprintf("cc-%d", i), "time": time.Now().UnixMilli(), "amount": 1500000, "account": acc(pid)})
			if e == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if e.Code != paymeOrderBusy {
				t.Errorf("unexpected: %v", e)
			}
		}()
	}
	wg.Wait()
	if ok != 1 {
		t.Fatalf("%d transactions created, want 1", ok)
	}
}

func TestPayme_FiscalDetail(t *testing.T) {
	f := newFixture(t)
	p := newPayme(f, nil)
	p.h.IKPU, p.h.PackageCode = "10899002001000000", "1500128"
	pid := f.payment(t, ProviderPayme)
	res, e := p.call(t, "", "CheckPerformTransaction", map[string]any{"amount": 1500000, "account": acc(pid)})
	if e != nil {
		t.Fatal(e)
	}
	items := res["detail"].(map[string]any)["items"].([]any)
	if items[0].(map[string]any)["code"] != "10899002001000000" || items[0].(map[string]any)["price"].(float64) != 1500000 {
		t.Fatalf("detail: %v", res)
	}
}

func TestExpirePending(t *testing.T) {
	f := newFixture(t)
	old := f.payment(t, ProviderClick)
	fresh := f.payment(t, ProviderClick)
	busy := f.payment(t, ProviderPayme)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `UPDATE payments SET created_at = now() - interval '2 days' WHERE id = ANY($1)`, []uuid.UUID{old, busy}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO payment_transactions (payment_id, provider, external_id, state, amount) VALUES ($1, 'payme', 'b-1', 1, 1500000)`, busy); err != nil {
		t.Fatal(err)
	}
	// брошенная подготовка Click старше суток тоже закрывается
	clickOld := f.payment(t, ProviderClick)
	if _, err := f.pool.Exec(ctx, `UPDATE payments SET created_at = now() - interval '2 days' WHERE id = $1`, clickOld); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO payment_transactions (payment_id, provider, external_id, state, amount, created_at)
		VALUES ($1, 'click', '5001', 1, 1500000, now() - interval '2 days')`, clickOld); err != nil {
		t.Fatal(err)
	}
	n, err := ExpirePending(ctx, f.pool, 24*time.Hour)
	if err != nil || n != 2 || f.status(t, clickOld) != "cancelled" {
		t.Fatalf("expired %d, %v", n, err)
	}
	if f.status(t, old) != "cancelled" || f.status(t, fresh) != "pending" || f.status(t, busy) != "pending" {
		t.Fatal("wrong rows expired")
	}
}
