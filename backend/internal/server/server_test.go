package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/admin"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/testutil"
	"github.com/boutique-market/backend/internal/tokens"
)

type captureSMS struct {
	mu    sync.Mutex
	codes map[string]string
}

func (c *captureSMS) SendOTP(_ context.Context, phone, code string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.codes[phone] = code
	return nil
}

func (c *captureSMS) last(phone string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.codes[phone]
}

type env struct {
	t   *testing.T
	srv *httptest.Server
	sms *captureSMS
	mr  *miniredis.Miniredis
	d   Deps
}

func setup(t *testing.T, mods ...func(*Deps)) *env {
	pool := testutil.DB(t)
	mr := miniredis.RunT(t)
	sms := &captureSMS{codes: map[string]string{}}
	d := Deps{
		Cfg: config.Config{
			Env: "test", JWTSecret: []byte(strings.Repeat("k", 32)), AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour,
			OTPTTL: 5 * time.Minute, OTPCooldown: time.Minute, OTPMaxTries: 5, OTPPepper: []byte("pepper-pepper-pepper"),
			MaxUploadMB: 5, CookieSecure: false, OTPAllowedPrefixes: []string{"+998"},
			PublicMediaURL: "https://cdn.test", UploadConcurrency: 2, EmailAuthEnabled: true,
			PaymentReturnURL: "https://shop.test/account/balance",
			ClickServiceID:   "777", ClickMerchantID: "555", ClickSecretKey: testClickSecret,
			PaymeMerchantID: "pm-merchant", PaymeKey: testPaymeKey, PaymeTest: true, PaymeAccountKey: "payment_id",
		},
		Log: slog.New(slog.NewJSONHandler(io.Discard, nil)), DB: pool,
		Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}), Storage: media.NewMemory(), SMS: sms,
	}
	for _, m := range mods {
		m(&d)
	}
	srv := httptest.NewServer(New(d))
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, sms: sms, mr: mr, d: d}
}

// client — HTTP-клиент со своей cookie-jar (отдельное «устройство»).
type client struct {
	e     *env
	http  *http.Client
	token string
}

func (e *env) client() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, http: &http.Client{Jar: jar}}
}

type resp struct {
	Status int
	Body   map[string]any
	Raw    string
}

func (r resp) data() map[string]any { m, _ := r.Body["data"].(map[string]any); return m }
func (r resp) errCode() string {
	m, _ := r.Body["error"].(map[string]any)
	s, _ := m["code"].(string)
	return s
}

func (c *client) do(method, path string, body any) resp {
	c.e.t.Helper()
	var rd io.Reader
	ct := "application/json"
	switch b := body.(type) {
	case nil:
	case []byte:
		rd, ct = bytes.NewReader(b), "image/png"
	default:
		buf, _ := json.Marshal(b)
		rd = bytes.NewReader(buf)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, rd)
	req.Header.Set("Content-Type", ct)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	out := resp{Status: res.StatusCode, Raw: string(raw)}
	_ = json.Unmarshal(raw, &out.Body)
	return out
}

func (c *client) must(status int, method, path string, body any) resp {
	c.e.t.Helper()
	r := c.do(method, path, body)
	if r.Status != status {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, r.Status, status, r.Raw)
	}
	return r
}

func (c *client) loginClient(phone string) resp {
	c.e.t.Helper()
	c.e.mr.Del("otp:cd:" + phone)
	c.must(202, "POST", "/v1/auth/otp/request", map[string]any{"phone": phone})
	r := c.must(200, "POST", "/v1/auth/otp/verify", map[string]any{"phone": phone, "code": c.e.sms.last(phone)})
	c.token = r.data()["access_token"].(string)
	return r
}

func TestClientRegistrationAndWelcomeBonusOnce(t *testing.T) {
	e := setup(t)
	c := e.client()
	const phone = "+998901234567"

	c.must(202, "POST", "/v1/auth/otp/request", map[string]any{"phone": phone})
	if r := c.do("POST", "/v1/auth/otp/request", map[string]any{"phone": phone}); r.Status != 429 || r.errCode() != "otp_cooldown" {
		t.Fatalf("cooldown not enforced: %d %s", r.Status, r.Raw)
	}
	if r := c.do("POST", "/v1/auth/otp/verify", map[string]any{"phone": phone, "code": "999999x"}); r.Status != 422 {
		t.Fatalf("bad code format: %d", r.Status)
	}
	r := c.must(200, "POST", "/v1/auth/otp/verify", map[string]any{"phone": phone, "code": e.sms.last(phone)})
	if r.data()["is_new_user"] != true {
		t.Fatal("expected new user")
	}
	c.token = r.data()["access_token"].(string)

	me := c.must(200, "GET", "/v1/me", nil).data()
	if me["registration_complete"] != false || me["token_balance"].(float64) != 0 {
		t.Fatalf("unexpected me before registration: %v", me)
	}
	if r := c.do("POST", "/v1/me/complete-registration", map[string]any{"gender": "female"}); r.Status != 422 {
		t.Fatalf("consent must be required: %d", r.Status)
	}
	for i := 0; i < 3; i++ {
		c.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "female", "pd_consent": true})
	}
	me = c.must(200, "GET", "/v1/me", nil).data()
	if me["token_balance"].(float64) != 20 || me["registration_complete"] != true {
		t.Fatalf("welcome bonus must be exactly 20: %v", me)
	}
	// повторный вход — снова не новый и без бонуса
	c2 := e.client()
	if c2.loginClient(phone).data()["is_new_user"] != false {
		t.Fatal("second login must not be new")
	}
	hist := c.must(200, "GET", "/v1/me/tokens", nil)
	if items := hist.Body["data"].([]any); len(items) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(items))
	}
	c.must(200, "PATCH", "/v1/me", map[string]any{"gender": "male", "height_cm": 180, "body_params": map[string]any{"chest": 96}})
	if r := c.do("PATCH", "/v1/me", map[string]any{"token_balance": 9999}); r.Status != 400 {
		t.Fatalf("mass assignment must be rejected: %d", r.Status)
	}
}

func TestRefreshRotationAndReuseDetection(t *testing.T) {
	e := setup(t)
	c := e.client()
	c.loginClient("+998907777777")

	// украденная копия текущего refresh-cookie
	u, _ := http.NewRequest("GET", e.srv.URL+"/v1/auth/refresh", nil)
	stolen := c.http.Jar.Cookies(u.URL)
	if len(stolen) != 1 {
		t.Fatalf("refresh cookie not set: %v", stolen)
	}
	c.must(200, "POST", "/v1/auth/refresh", nil) // легитимная ротация

	attacker := e.client()
	attacker.http.Jar.SetCookies(u.URL, stolen)
	if r := attacker.do("POST", "/v1/auth/refresh", nil); r.Status != 401 {
		t.Fatalf("reused refresh must fail: %d", r.Status)
	}
	// вся семья отозвана — легитимный клиент тоже должен перелогиниться
	if r := c.do("POST", "/v1/auth/refresh", nil); r.Status != 401 {
		t.Fatalf("family must be revoked after reuse: %d", r.Status)
	}
}

func TestAccessControl(t *testing.T) {
	e := setup(t)
	anon := e.client()
	if r := anon.do("GET", "/v1/me", nil); r.Status != 401 {
		t.Fatalf("anon /me: %d", r.Status)
	}
	anon.token = "garbage"
	if r := anon.do("GET", "/v1/products", nil); r.Status != 401 || r.errCode() != "token_invalid" {
		t.Fatalf("invalid token must be 401: %d", r.Status)
	}
	c := e.client()
	c.loginClient("+998903333333")
	for _, path := range []string{"/v1/admin/stores", "/v1/store/products"} {
		if r := c.do("POST", path, map[string]any{}); r.Status != 403 {
			t.Fatalf("client %s: %d", path, r.Status)
		}
	}
	if r := c.do("POST", "/v1/uploads?purpose=product", pngFile(t)); r.Status != 422 {
		t.Fatalf("client must not upload product images: %d", r.Status)
	}
	r := c.do("GET", "/v1/nope", nil)
	if r.Status != 404 || r.Body["request_id"] == "" {
		t.Fatalf("404 envelope: %s", r.Raw)
	}
}

func pngFile(t *testing.T) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 20, 30)))
	return b.Bytes()
}

func TestStoreLifecycleAndCatalog(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := admin.CreateAdmin(ctx, e.d.DB, "root", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	adm := e.client()
	adm.token = adm.must(200, "POST", "/v1/auth/login", map[string]any{"login": "root", "password": "admin-password-123"}).data()["access_token"].(string)

	createStore := func(slug, login string) string {
		r := adm.must(201, "POST", "/v1/admin/stores", map[string]any{"slug": slug, "name": "Бутик " + slug, "login": login, "password": "store-password-1"})
		return r.data()["store_id"].(string)
	}
	s1 := createStore("alpha", "alpha")
	s2 := createStore("beta", "beta")
	adm.must(409, "POST", "/v1/admin/stores", map[string]any{"slug": "alpha", "name": "x", "login": "other", "password": "store-password-1"})

	st := e.client()
	if r := st.do("POST", "/v1/auth/login", map[string]any{"login": "alpha", "password": "store-password-1"}); r.errCode() != "store_not_active" {
		t.Fatalf("pending store must not log in: %s", r.Raw)
	}
	if r := st.do("POST", "/v1/auth/login", map[string]any{"login": "alpha", "password": "wrong-password"}); r.errCode() != "bad_credentials" {
		t.Fatalf("wrong password: %s", r.Raw)
	}
	adm.must(200, "POST", "/v1/admin/stores/"+s1+"/status", map[string]any{"status": "active"})
	adm.must(200, "POST", "/v1/admin/stores/"+s2+"/status", map[string]any{"status": "active"})
	st.token = st.must(200, "POST", "/v1/auth/login", map[string]any{"login": "alpha", "password": "store-password-1"}).data()["access_token"].(string)
	other := e.client()
	other.token = other.must(200, "POST", "/v1/auth/login", map[string]any{"login": "beta", "password": "store-password-1"}).data()["access_token"].(string)

	cats := st.must(200, "GET", "/v1/categories", nil).Body["data"].([]any)
	catID := cats[0].(map[string]any)["id"].(string)

	images := map[string]string{}
	for _, a := range []string{"front", "side", "back"} {
		images[a] = st.must(201, "POST", "/v1/uploads?purpose=product", pngFile(t)).data()["key"].(string)
	}
	// чужие ключи изображений не принимаются
	foreign := map[string]string{"front": media.NewKey(uuid.New(), media.PurposeProductImage), "side": images["side"], "back": images["back"]}
	prod := map[string]any{"category_id": catID, "sku": "A-1", "title": "Пальто", "description": "Шерсть", "brand": "Acme", "price": 150000000, "in_stock": true, "images": foreign}
	if r := st.do("POST", "/v1/store/products", prod); r.Status != 422 {
		t.Fatalf("foreign image key accepted: %s", r.Raw)
	}
	prod["images"] = images
	pid := st.must(201, "POST", "/v1/store/products", prod).data()["id"].(string)
	st.must(409, "POST", "/v1/store/products", prod)

	if n := len(st.must(200, "GET", "/v1/products", nil).Body["data"].([]any)); n != 0 {
		t.Fatal("draft must not be public")
	}
	// другой магазин не может трогать чужой товар
	other.must(404, "PATCH", "/v1/store/products/"+pid+"/status", map[string]any{"status": "published"})
	if n := len(other.must(200, "GET", "/v1/store/products", nil).Body["data"].([]any)); n != 0 {
		t.Fatal("store sees foreign products")
	}
	st.must(200, "PATCH", "/v1/store/products/"+pid+"/status", map[string]any{"status": "published"})

	// автоартикул: пустой sku — сервер выдаёт следующий номер магазина,
	// пропуская номера, занятые вручную; у другого магазина своя нумерация
	autoProd := func(c *client, sku string) string {
		t.Helper()
		imgs := map[string]string{}
		for _, a := range []string{"front", "side", "back"} {
			imgs[a] = c.must(201, "POST", "/v1/uploads?purpose=product", pngFile(t)).data()["key"].(string)
		}
		p := map[string]any{"category_id": catID, "sku": sku, "title": "Шарф", "description": "Кашемир", "brand": "Acme", "price": 5000000, "in_stock": true, "images": imgs}
		return c.must(201, "POST", "/v1/store/products", p).data()["sku"].(string)
	}
	if got := autoProd(st, ""); got != "A-000001" {
		t.Fatalf("first auto sku = %q", got)
	}
	if got := autoProd(st, "  A-000002 "); got != "A-000002" { // вручную, с пробелами
		t.Fatalf("manual sku = %q", got)
	}
	if got := autoProd(st, ""); got != "A-000003" {
		t.Fatalf("auto sku must skip manual A-000002, got %q", got)
	}
	if got := autoProd(other, ""); got != "A-000001" {
		t.Fatalf("other store auto sku = %q", got)
	}

	anon := e.client()
	list := anon.must(200, "GET", "/v1/products?q=пальто&sort=price_asc", nil).Body["data"].([]any)
	if len(list) != 1 {
		t.Fatalf("published product not listed: %v", list)
	}
	card := anon.must(200, "GET", "/v1/products/"+pid, nil).data()
	if len(card["images"].(map[string]any)) != 3 {
		t.Fatalf("images missing: %v", card)
	}
	anon.must(400, "GET", "/v1/products?sort=drop_table", nil)

	// блокировка магазина: товары исчезают из каталога, сессии отозваны
	adm.must(200, "POST", "/v1/admin/stores/"+s1+"/status", map[string]any{"status": "blocked", "reason": "test"})
	if n := len(anon.must(200, "GET", "/v1/products", nil).Body["data"].([]any)); n != 0 {
		t.Fatal("blocked store products still public")
	}
	if r := st.do("POST", "/v1/auth/refresh", nil); r.Status != 401 && r.Status != 403 {
		t.Fatalf("blocked store refresh: %d", r.Status)
	}
	var n int
	_ = e.d.DB.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action LIKE 'store.%'`).Scan(&n)
	if n != 5 {
		t.Fatalf("audit entries: %d", n)
	}
}

func TestCatalogPagination(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var storeID, catID uuid.UUID
	_ = e.d.DB.QueryRow(ctx, `INSERT INTO stores (slug, name, status) VALUES ('shop', 'S', 'active') RETURNING id`).Scan(&storeID)
	_ = e.d.DB.QueryRow(ctx, `SELECT id FROM categories LIMIT 1`).Scan(&catID)
	for i := 0; i < 7; i++ {
		_, err := e.d.DB.Exec(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price, status)
			VALUES ($1, $2, $3, 'T', 'D', 'B', $4, 'published')`, storeID, catID, uuid.NewString(), 1000*(i%3+1))
		if err != nil {
			t.Fatal(err)
		}
	}
	anon := e.client()
	for _, sort := range []string{"new", "price_asc", "price_desc", "popular"} {
		seen := map[string]bool{}
		cursor := ""
		for page := 0; page < 10; page++ {
			r := anon.must(200, "GET", "/v1/products?limit=3&sort="+sort+"&cursor="+cursor, nil)
			for _, it := range r.Body["data"].([]any) {
				id := it.(map[string]any)["id"].(string)
				if seen[id] {
					t.Fatalf("%s: duplicate %s", sort, id)
				}
				seen[id] = true
			}
			c, _ := r.Body["next_cursor"].(string)
			if c == "" {
				break
			}
			cursor = c
		}
		if len(seen) != 7 {
			t.Fatalf("%s: saw %d of 7", sort, len(seen))
		}
	}
}

func TestLedgerInvariants(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var uid uuid.UUID
	_ = e.d.DB.QueryRow(ctx, `INSERT INTO users (role, phone, token_balance) VALUES ('client', '+998905555555', 0) RETURNING id`).Scan(&uid)
	err := db.Tx(ctx, e.d.DB, func(tx pgx.Tx) error {
		_, err := tokens.Apply(ctx, tx, tokens.Entry{UserID: uid, Op: tokens.OpPurchase, Amount: 100, ReferenceType: "payment", ReferenceID: ptr(uuid.New())})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// 20 параллельных списаний по 20 при балансе 100 — успешно ровно 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := db.Tx(ctx, e.d.DB, func(tx pgx.Tx) error {
				_, err := tokens.Apply(ctx, tx, tokens.Entry{UserID: uid, Op: tokens.OpGenerationCharge, Amount: -20, ReferenceType: "generation", ReferenceID: ptr(uuid.New())})
				return err
			})
			if err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	var bal, sum int64
	_ = e.d.DB.QueryRow(ctx, `SELECT token_balance FROM users WHERE id = $1`, uid).Scan(&bal)
	_ = e.d.DB.QueryRow(ctx, `SELECT COALESCE(sum(amount), 0) FROM token_ledger WHERE user_id = $1`, uid).Scan(&sum)
	if ok != 5 || bal != 0 || sum != bal {
		t.Fatalf("ok=%d balance=%d ledger_sum=%d", ok, bal, sum)
	}
	// одна и та же оплата не зачисляется дважды
	ref := uuid.New()
	for i := 0; i < 2; i++ {
		err = db.Tx(ctx, e.d.DB, func(tx pgx.Tx) error {
			_, err := tokens.Apply(ctx, tx, tokens.Entry{UserID: uid, Op: tokens.OpPurchase, Amount: 50, ReferenceType: "payment", ReferenceID: &ref})
			return err
		})
	}
	if err != tokens.ErrDuplicate {
		t.Fatalf("duplicate credit must fail, got %v", err)
	}
	if _, err := e.d.DB.Exec(ctx, `UPDATE token_ledger SET amount = 1000000`); err == nil {
		t.Fatal("ledger must be append-only")
	}
	_, _ = e.d.DB.Exec(ctx, `INSERT INTO audit_log (action, entity_type) VALUES ('test', 'test')`)
	if _, err := e.d.DB.Exec(ctx, `DELETE FROM audit_log`); err == nil {
		t.Fatal("audit log must be append-only")
	}
	if _, err := e.d.DB.Exec(ctx, `TRUNCATE token_ledger`); err == nil {
		t.Fatal("ledger must not be truncatable")
	}
}

func ptr[T any](v T) *T { return &v }

func TestViewsCountedInRedisAndFlushed(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	var storeID, catID, pid uuid.UUID
	_ = e.d.DB.QueryRow(ctx, `INSERT INTO stores (slug, name, status) VALUES ('views', 'V', 'active') RETURNING id`).Scan(&storeID)
	_ = e.d.DB.QueryRow(ctx, `SELECT id FROM categories LIMIT 1`).Scan(&catID)
	_ = e.d.DB.QueryRow(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price, status)
		VALUES ($1, $2, 'v1', 'T', 'D', 'B', 100, 'published') RETURNING id`, storeID, catID).Scan(&pid)
	_, _ = e.d.DB.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, 'front', $2)`, pid, media.NewKey(uuid.New(), media.PurposeProductImage))

	vc := &products.ViewCounter{Redis: e.d.Redis, DB: e.d.DB, Log: e.d.Log}
	e.d.Views = vc
	srv := httptest.NewServer(New(e.d))
	defer srv.Close()
	e.srv = srv
	anon := e.client()
	var first string
	for i := 0; i < 3; i++ {
		img := anon.must(200, "GET", "/v1/products/"+pid.String(), nil).data()["images"].(map[string]any)["front"].(string)
		if !strings.HasPrefix(img, "https://cdn.test/pub/") {
			t.Fatalf("product image must be a stable CDN URL: %s", img)
		}
		if first != "" && img != first {
			t.Fatal("image URL changes between requests (not cacheable)")
		}
		first = img
		// просмотр отмечает браузер отдельным запросом (GET кэшируется сайтом и не считается)
		anon.must(204, "POST", "/v1/products/"+pid.String()+"/view", nil)
	}
	anon.must(404, "POST", "/v1/products/not-a-uuid/view", nil)
	var n int64
	_ = e.d.DB.QueryRow(ctx, `SELECT views_count FROM products WHERE id = $1`, pid).Scan(&n)
	if n != 0 {
		t.Fatal("views must not be written per request")
	}
	if err := vc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	_ = e.d.DB.QueryRow(ctx, `SELECT views_count FROM products WHERE id = $1`, pid).Scan(&n)
	if n != 3 {
		t.Fatalf("views_count=%d, want 3", n)
	}
	if err := vc.Flush(ctx); err != nil { // пустой сброс — не ошибка
		t.Fatal(err)
	}
}

func TestBlockedClientLosesAccessImmediately(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, _ = admin.CreateAdmin(ctx, e.d.DB, "root", "admin-password-123")
	adm := e.client()
	adm.token = adm.must(200, "POST", "/v1/auth/login", map[string]any{"login": "root", "password": "admin-password-123"}).data()["access_token"].(string)
	c := e.client()
	uid := c.loginClient("+998904444444").data()["user"].(map[string]any)["id"].(string)
	c.must(200, "GET", "/v1/me", nil)
	adm.must(422, "POST", "/v1/admin/users/"+uid+"/status", map[string]any{"status": "blocked"})
	adm.must(200, "POST", "/v1/admin/users/"+uid+"/status", map[string]any{"status": "blocked", "reason": "fraud"})
	if r := c.do("GET", "/v1/me", nil); r.Status != 401 || r.errCode() != "token_revoked" {
		t.Fatalf("blocked client still has access: %d %s", r.Status, r.Raw)
	}
	if r := c.do("POST", "/v1/auth/refresh", nil); r.Status != 401 && r.Status != 403 {
		t.Fatalf("blocked client refresh: %d", r.Status)
	}
	c.token = ""
	c.e.mr.Del("otp:cd:+998904444444")
	c.must(202, "POST", "/v1/auth/otp/request", map[string]any{"phone": "+998904444444"})
	if r := c.do("POST", "/v1/auth/otp/verify", map[string]any{"phone": "+998904444444", "code": e.sms.last("+998904444444")}); r.errCode() != "account_blocked" {
		t.Fatalf("blocked client must not log in: %s", r.Raw)
	}
}
