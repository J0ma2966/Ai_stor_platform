package server

import (
	"context"
	"crypto/md5" // #nosec G501 -- протокол Click
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boutique-market/backend/internal/admin"
)

const (
	testClickSecret = "click-secret-test"
	testPaymeKey    = "payme-key-test"
)

func (e *env) admin() *client {
	e.t.Helper()
	if _, err := admin.CreateAdmin(context.Background(), e.d.DB, "root", "admin-password-123"); err != nil {
		e.t.Fatal(err)
	}
	adm := e.client()
	adm.token = adm.must(200, "POST", "/v1/auth/login", map[string]any{"login": "root", "password": "admin-password-123"}).data()["access_token"].(string)
	return adm
}

func (e *env) clickHook(path string, v url.Values) map[string]any {
	e.t.Helper()
	parts := v.Get("click_trans_id") + v.Get("service_id") + testClickSecret + v.Get("merchant_trans_id")
	if path == "complete" {
		parts += v.Get("merchant_prepare_id")
	}
	sum := md5.Sum([]byte(parts + v.Get("amount") + v.Get("action") + v.Get("sign_time"))) // #nosec G401
	v.Set("sign_string", hex.EncodeToString(sum[:]))
	res, err := http.PostForm(e.srv.URL+"/v1/payments/click/"+path, v)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return out
}

func (e *env) payme(method string, params map[string]any) map[string]any {
	e.t.Helper()
	body, _ := json.Marshal(map[string]any{"id": 1, "method": method, "params": params})
	req, _ := http.NewRequest("POST", e.srv.URL+"/v1/payments/payme", strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("Paycom:"+testPaymeKey)))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

// Полный путь покупки токенов: админ заводит тариф → клиент видит его и создаёт
// платёж → провайдер подтверждает → баланс пополнен ровно один раз.
func TestTokenPurchase_ClickAndPayme(t *testing.T) {
	e := setup(t)
	adm := e.admin()
	tariff := adm.must(201, "POST", "/v1/admin/tariffs", map[string]any{
		"name_ru": "Старт", "name_uz": "Start", "tokens": 50, "price": 1500000, "is_published": true}).data()
	hidden := adm.must(201, "POST", "/v1/admin/tariffs", map[string]any{
		"name_ru": "Скрытый", "name_uz": "Yashirin", "tokens": 5, "price": 100000}).data()
	if r := adm.do("POST", "/v1/admin/tariffs", map[string]any{"name_ru": "x"}); r.Status != 422 {
		t.Fatalf("incomplete tariff: %d", r.Status)
	}

	anon := e.client()
	pub := anon.must(200, "GET", "/v1/tariffs", nil).Body["data"].([]any)
	if len(pub) != 1 || pub[0].(map[string]any)["id"] != tariff["id"] {
		t.Fatalf("public tariffs: %v", pub)
	}

	cl := e.client()
	cl.loginClient("+998901230000")
	if r := cl.do("POST", "/v1/payments", map[string]any{"tariff_id": hidden["id"], "provider": "click"}); r.errCode() != "tariff_not_found" {
		t.Fatalf("unpublished tariff must not be purchasable: %s", r.Raw)
	}
	if r := cl.do("POST", "/v1/payments", map[string]any{"tariff_id": tariff["id"], "provider": "cash"}); r.Status != 422 {
		t.Fatalf("unknown provider: %d", r.Status)
	}

	// Click, с Idempotency-Key: повтор возвращает тот же платёж
	key := map[string]string{"Idempotency-Key": "buy-click-0001"}
	p1 := cl.mustH(201, "POST", "/v1/payments", map[string]any{"tariff_id": tariff["id"], "provider": "click"}, key).data()
	p1b := cl.mustH(200, "POST", "/v1/payments", map[string]any{"tariff_id": tariff["id"], "provider": "click"}, key).data()
	if p1["id"] != p1b["id"] {
		t.Fatal("idempotency key must return the same payment")
	}
	if r := cl.doH("POST", "/v1/payments", map[string]any{"tariff_id": tariff["id"], "provider": "payme"}, key); r.errCode() != "idempotency_key_reused" {
		t.Fatalf("reused key: %s", r.Raw)
	}
	cu, _ := url.Parse(p1["checkout_url"].(string))
	if cu.Host != "my.click.uz" || cu.Query().Get("amount") != "15000.00" || cu.Query().Get("transaction_param") != p1["id"] ||
		!strings.Contains(cu.Query().Get("return_url"), "payment="+p1["id"].(string)) {
		t.Fatalf("click checkout url: %s", cu)
	}
	form := url.Values{"click_trans_id": {"9001"}, "service_id": {"777"}, "click_paydoc_id": {"1"},
		"merchant_trans_id": {p1["id"].(string)}, "amount": {"15000"}, "action": {"0"}, "error": {"0"},
		"error_note": {"Success"}, "sign_time": {"2026-11-02 12:00:00"}}
	prep := e.clickHook("prepare", form)
	if prep["error"].(float64) != 0 {
		t.Fatalf("prepare: %v", prep)
	}
	form.Set("action", "1")
	form.Set("merchant_prepare_id", strings.TrimSuffix(strings.TrimSuffix(jsonNum(prep["merchant_prepare_id"]), ".0"), ".00"))
	for range 2 {
		if done := e.clickHook("complete", form); done["error"].(float64) != 0 {
			t.Fatalf("complete: %v", done)
		}
	}
	if st := cl.must(200, "GET", "/v1/payments/"+p1["id"].(string), nil).data(); st["status"] != "paid" || st["checkout_url"] != nil {
		t.Fatalf("payment after click: %v", st)
	}

	// Payme
	p2 := cl.must(201, "POST", "/v1/payments", map[string]any{"tariff_id": tariff["id"], "provider": "payme"}).data()
	pu := p2["checkout_url"].(string)
	dec, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(pu, "https://checkout.test.paycom.uz/"))
	if !strings.Contains(string(dec), "m=pm-merchant;ac.payment_id="+p2["id"].(string)+";a=1500000") {
		t.Fatalf("payme checkout: %s", dec)
	}
	acc := map[string]any{"payment_id": p2["id"]}
	if r := e.payme("CheckPerformTransaction", map[string]any{"amount": 1500000, "account": acc}); r["result"] == nil {
		t.Fatalf("check perform: %v", r)
	}
	if r := e.payme("CreateTransaction", map[string]any{"id": "pm-777", "time": time.Now().UnixMilli(), "amount": 1500000, "account": acc}); r["result"] == nil {
		t.Fatalf("create: %v", r)
	}
	for range 2 {
		if r := e.payme("PerformTransaction", map[string]any{"id": "pm-777"}); r["result"] == nil {
			t.Fatalf("perform: %v", r)
		}
	}
	me := cl.must(200, "GET", "/v1/me", nil).data()
	if me["token_balance"].(float64) != 100 {
		t.Fatalf("balance must be 100 (2×50): %v", me["token_balance"])
	}
	mine := cl.must(200, "GET", "/v1/payments", nil).Body["data"].([]any)
	if len(mine) != 2 {
		t.Fatalf("my payments: %d", len(mine))
	}

	// чужой платёж не виден
	other := e.client()
	other.loginClient("+998901230001")
	other.must(404, "GET", "/v1/payments/"+p1["id"].(string), nil)
	if l := other.must(200, "GET", "/v1/payments", nil).Body["data"].([]any); len(l) != 0 {
		t.Fatal("foreign payments leaked")
	}
	// админ: фильтры
	paid := adm.must(200, "GET", "/v1/admin/payments?status=paid&provider=payme", nil).Body["data"].([]any)
	if len(paid) != 1 || paid[0].(map[string]any)["id"] != p2["id"] {
		t.Fatalf("admin filter: %v", paid)
	}
	adm.must(400, "GET", "/v1/admin/payments?status=weird", nil)
	adm.must(200, "PATCH", "/v1/admin/tariffs/"+tariff["id"].(string), map[string]any{"price": 2000000, "is_published": false})
	if l := anon.must(200, "GET", "/v1/tariffs", nil).Body["data"].([]any); len(l) != 0 {
		t.Fatal("unpublished tariff still public")
	}
	// снимок цены: уже оплаченный платёж не изменился
	if st := cl.must(200, "GET", "/v1/payments/"+p2["id"].(string), nil).data(); st["amount"].(float64) != 1500000 {
		t.Fatalf("payment amount changed: %v", st)
	}
	runConsistencyCheck(t, e)
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// runConsistencyCheck выполняет scripts/consistency-check.sql (инварианты данных).
func runConsistencyCheck(t *testing.T, e *env) {
	t.Helper()
	sql, err := os.ReadFile("../../../scripts/consistency-check.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.d.DB.Exec(context.Background(), string(sql)); err != nil {
		t.Fatalf("consistency check: %v", err)
	}
}
