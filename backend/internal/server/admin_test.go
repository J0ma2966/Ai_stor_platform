package server

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/boutique-market/backend/internal/admin"
)

func newAdmin(t *testing.T, e *env) *client {
	t.Helper()
	if _, err := admin.CreateAdmin(context.Background(), e.d.DB, "root", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	adm := e.client()
	adm.token = adm.must(200, "POST", "/v1/auth/login", map[string]any{"login": "root", "password": "admin-password-123"}).data()["access_token"].(string)
	return adm
}

func list(r resp) []map[string]any {
	arr, _ := r.Body["data"].([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, v := range arr {
		m, _ := v.(map[string]any)
		out = append(out, m)
	}
	return out
}

func TestAdminPanelReadEndpoints(t *testing.T) {
	e := setup(t)
	adm := newAdmin(t, e)

	ids := map[string]string{}
	for _, slug := range []string{"aaa", "bbb", "ccc"} {
		ids[slug] = adm.must(201, "POST", "/v1/admin/stores", map[string]any{"slug": slug, "name": "Бутик " + strings.ToUpper(slug), "login": slug + "-shop", "password": "store-password-1"}).data()["store_id"].(string)
	}
	adm.must(200, "POST", "/v1/admin/stores/"+ids["aaa"]+"/status", map[string]any{"status": "active"})
	adm.must(200, "POST", "/v1/admin/stores/"+ids["bbb"]+"/status", map[string]any{"status": "active"})
	adm.must(200, "POST", "/v1/admin/stores/"+ids["bbb"]+"/status", map[string]any{"status": "blocked", "reason": "жалобы"})

	cl := e.client()
	cl.loginClient("+998901112233")
	cl.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "female", "pd_consent": true, "lang": "ru"})
	me := cl.must(200, "GET", "/v1/me", nil).data()
	clientID := me["id"].(string)

	// --- статистика
	st := adm.must(200, "GET", "/v1/admin/stats", nil).data()
	stores := st["stores"].(map[string]any)
	if stores["total"].(float64) != 3 || stores["active"].(float64) != 1 || stores["blocked"].(float64) != 1 || stores["pending_activation"].(float64) != 1 {
		t.Fatalf("store stats: %v", stores)
	}
	if u := st["users"].(map[string]any); u["clients"].(float64) != 1 || u["clients_new_7d"].(float64) != 1 {
		t.Fatalf("user stats: %v", u)
	}
	if bs := st["orders"].(map[string]any)["by_status"].(map[string]any); len(bs) != 7 {
		t.Fatalf("by_status must list all statuses: %v", bs)
	}
	if tk := st["tokens"].(map[string]any); tk["issued_30d"].(float64) != 20 {
		t.Fatalf("token stats: %v", tk)
	}

	// --- список магазинов: фильтры, поиск, пагинация
	all := list(adm.must(200, "GET", "/v1/admin/stores", nil))
	if len(all) != 3 || all[0]["slug"] != "ccc" || all[0]["login"] != "ccc-shop" {
		t.Fatalf("stores list: %v", all)
	}
	if got := list(adm.must(200, "GET", "/v1/admin/stores?status=blocked", nil)); len(got) != 1 || got[0]["slug"] != "bbb" {
		t.Fatalf("blocked filter: %v", got)
	}
	if got := list(adm.must(200, "GET", "/v1/admin/stores?q="+url.QueryEscape("бутик a"), nil)); len(got) != 1 || got[0]["slug"] != "aaa" {
		t.Fatalf("search: %v", got)
	}
	// % не должен работать как шаблон
	if got := list(adm.must(200, "GET", "/v1/admin/stores?q=%25", nil)); len(got) != 0 {
		t.Fatalf("LIKE wildcard leaked: %v", got)
	}
	p1 := adm.must(200, "GET", "/v1/admin/stores?limit=2", nil)
	cur, _ := p1.Body["next_cursor"].(string)
	if len(list(p1)) != 2 || cur == "" {
		t.Fatalf("page1: %v", p1.Raw)
	}
	p2 := adm.must(200, "GET", "/v1/admin/stores?limit=2&cursor="+cur, nil)
	if got := list(p2); len(got) != 1 || got[0]["slug"] != "aaa" || p2.Body["next_cursor"] != nil {
		t.Fatalf("page2: %v", p2.Raw)
	}
	if r := adm.do("GET", "/v1/admin/stores?status=nope", nil); r.Status != 400 || r.errCode() != "bad_filter" {
		t.Fatalf("bad filter: %d %s", r.Status, r.Raw)
	}
	if r := adm.do("GET", "/v1/admin/stores?cursor=@@", nil); r.Status != 400 || r.errCode() != "bad_cursor" {
		t.Fatalf("bad cursor: %d %s", r.Status, r.Raw)
	}

	// --- карточка магазина с историей действий
	d := adm.must(200, "GET", "/v1/admin/stores/"+ids["bbb"], nil).data()
	hist := d["history"].([]any)
	if d["status"] != "blocked" || len(hist) != 3 || hist[0].(map[string]any)["action"] != "store.status" {
		t.Fatalf("store detail: %v", d)
	}
	if hist[0].(map[string]any)["actor_login"] != "root" {
		t.Fatalf("actor login: %v", hist[0])
	}
	adm.must(404, "GET", "/v1/admin/stores/00000000-0000-0000-0000-000000000000", nil)
	adm.must(404, "GET", "/v1/admin/stores/not-a-uuid", nil)

	// --- пользователи
	if got := list(adm.must(200, "GET", "/v1/admin/users?role=client", nil)); len(got) != 1 || got[0]["phone"] != "+998901112233" || got[0]["token_balance"].(float64) != 20 || got[0]["registration_complete"] != true {
		t.Fatalf("clients: %v", got)
	}
	if got := list(adm.must(200, "GET", "/v1/admin/users?role=store&q=bbb", nil)); len(got) != 1 || got[0]["store_name"] != "Бутик BBB" {
		t.Fatalf("store users: %v", got)
	}
	if got := list(adm.must(200, "GET", "/v1/admin/users?q=1112", nil)); len(got) != 1 {
		t.Fatalf("phone search: %v", got)
	}
	adm.must(200, "POST", "/v1/admin/users/"+clientID+"/tokens", map[string]any{"amount": 5, "reason": "компенсация"})
	adm.must(200, "POST", "/v1/admin/users/"+clientID+"/status", map[string]any{"status": "blocked", "reason": "спам"})
	if got := list(adm.must(200, "GET", "/v1/admin/users?status=blocked", nil)); len(got) != 1 || got[0]["id"] != clientID {
		t.Fatalf("blocked users: %v", got)
	}
	ud := adm.must(200, "GET", "/v1/admin/users/"+clientID, nil).data()
	ledger := ud["ledger"].([]any)
	if ud["token_balance"].(float64) != 25 || len(ledger) != 2 || ledger[0].(map[string]any)["op"] != "admin_adjustment" {
		t.Fatalf("user detail: %v", ud)
	}
	if h := ud["history"].([]any); len(h) != 2 {
		t.Fatalf("user history: %v", h)
	}

	// --- журнал аудита
	a := list(adm.must(200, "GET", "/v1/admin/audit", nil))
	if len(a) < 8 || a[0]["action"] != "user.status" {
		t.Fatalf("audit: %v", a)
	}
	if got := list(adm.must(200, "GET", "/v1/admin/audit?action=store.", nil)); len(got) != 6 {
		t.Fatalf("audit prefix filter: %d", len(got))
	}
	if got := list(adm.must(200, "GET", "/v1/admin/audit?action=tokens.adjust&entity_id="+clientID, nil)); len(got) != 1 {
		t.Fatalf("audit exact filter: %v", got)
	}
	ap := adm.must(200, "GET", "/v1/admin/audit?limit=3", nil)
	c, _ := ap.Body["next_cursor"].(string)
	ap2 := list(adm.must(200, "GET", "/v1/admin/audit?limit=3&cursor="+c, nil))
	if len(ap2) != 3 || ap2[0]["id"].(float64) >= list(ap)[2]["id"].(float64) {
		t.Fatalf("audit paging: %v", ap2)
	}
	adm.must(400, "GET", "/v1/admin/audit?actor_id=x", nil)

	// --- не-админ не видит ничего
	st2 := e.client()
	adm.must(200, "POST", "/v1/admin/stores/"+ids["ccc"]+"/status", map[string]any{"status": "active"})
	st2.token = st2.must(200, "POST", "/v1/auth/login", map[string]any{"login": "ccc-shop", "password": "store-password-1"}).data()["access_token"].(string)
	for _, p := range []string{"/v1/admin/stats", "/v1/admin/stores", "/v1/admin/users", "/v1/admin/audit", "/v1/admin/users/" + clientID} {
		st2.must(403, "GET", p, nil)
	}
}

func TestAdminResetStorePassword(t *testing.T) {
	e := setup(t)
	adm := newAdmin(t, e)
	sid := adm.must(201, "POST", "/v1/admin/stores", map[string]any{"slug": "pw", "name": "PW", "login": "pw-shop", "password": "store-password-1"}).data()["store_id"].(string)
	adm.must(200, "POST", "/v1/admin/stores/"+sid+"/status", map[string]any{"status": "active"})
	st := e.client()
	st.token = st.must(200, "POST", "/v1/auth/login", map[string]any{"login": "pw-shop", "password": "store-password-1"}).data()["access_token"].(string)
	st.must(200, "GET", "/v1/store/orders", nil)

	adm.must(422, "POST", "/v1/admin/stores/"+sid+"/password", map[string]any{"password": "short"})
	adm.must(404, "POST", "/v1/admin/stores/00000000-0000-0000-0000-000000000000/password", map[string]any{"password": "new-password-123"})
	adm.must(204, "POST", "/v1/admin/stores/"+sid+"/password", map[string]any{"password": "new-password-123"})

	// старая сессия и старый пароль больше не работают
	if r := st.do("GET", "/v1/store/orders", nil); r.Status != 401 {
		t.Fatalf("old session still works: %d", r.Status)
	}
	if r := st.do("POST", "/v1/auth/refresh", nil); r.Status != 401 {
		t.Fatalf("old refresh still works: %d", r.Status)
	}
	e.client().must(401, "POST", "/v1/auth/login", map[string]any{"login": "pw-shop", "password": "store-password-1"})
	e.client().must(200, "POST", "/v1/auth/login", map[string]any{"login": "pw-shop", "password": "new-password-123"})

	a := list(adm.must(200, "GET", "/v1/admin/audit?action=store.password_reset", nil))
	if len(a) != 1 || strings.Contains(fmtAny(a[0]["data"]), "new-password") {
		t.Fatalf("audit: %v", a)
	}
}

func fmtAny(v any) string { b, _ := json.Marshal(v); return string(b) }
