package server

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/tryon"
)

func jpegOf(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type tryonFixture struct {
	*orderFixture
	mem          *media.Memory
	cli          *client
	hat, top     string
	top2         string
	photos       map[string]any
	keys         []string
	worker       *tryon.Worker
	providerFail func(tryon.Request) error
}

func newTryonFixture(t *testing.T, mods ...func(*Deps)) *tryonFixture {
	of := newOrderFixture(t, mods...)
	f := &tryonFixture{orderFixture: of, mem: of.e.d.Storage.(*media.Memory)}
	ctx := context.Background()
	mk := func(zone string) string {
		var id uuid.UUID
		if err := of.e.d.DB.QueryRow(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price, status)
			VALUES ($1, (SELECT id FROM categories WHERE zone = $2 LIMIT 1), $3, 'Вещь', 'D', 'Acme', 100000, 'published')
			RETURNING id`, of.storeAID, zone, uuid.NewString()).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for _, a := range []string{"front", "back"} {
			key := "pub/u/" + uuid.NewString() + "/product/" + strings.Repeat("a", 32) + ".jpg"
			_ = f.mem.Put(ctx, key, jpegOf(t, 40, 60, color.RGBA{200, 0, 0, 255}), "image/jpeg")
			if _, err := of.e.d.DB.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, $2, $3)`, id, a, key); err != nil {
				t.Fatal(err)
			}
		}
		return id.String()
	}
	f.hat, f.top, f.top2 = mk("headwear"), mk("top"), mk("top")

	f.cli = of.e.client()
	f.cli.loginClient("+998901112233")
	f.cli.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "female", "pd_consent": true})
	f.photos = map[string]any{}
	for _, a := range tryon.Angles {
		key := f.cli.must(201, "POST", "/v1/uploads?purpose=body", pngFile(t)).data()["key"].(string)
		f.photos[a] = key
		f.keys = append(f.keys, key)
	}
	f.worker = &tryon.Worker{DB: of.e.d.DB, Store: f.mem, Log: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Timeout: 10 * time.Second, MaxAttempts: 2,
		Provider: tryon.Mock{Fail: func(r tryon.Request) error {
			if f.providerFail != nil {
				return f.providerFail(r)
			}
			return nil
		}}}
	return f
}

func (f *tryonFixture) runOnce(t *testing.T) {
	t.Helper()
	did, err := f.worker.RunOnce(context.Background())
	if err != nil || !did {
		t.Fatalf("worker: did=%v err=%v", did, err)
	}
}

func (f *tryonFixture) body(products ...string) map[string]any {
	return map[string]any{"product_ids": products, "photos": f.photos}
}

func (f *tryonFixture) balance(t *testing.T) float64 {
	return f.cli.must(200, "GET", "/v1/me", nil).data()["token_balance"].(float64)
}

func (f *tryonFixture) grant(t *testing.T, n int) {
	t.Helper()
	if _, err := f.e.d.DB.Exec(context.Background(), `
		WITH u AS (UPDATE users SET token_balance = token_balance + $2 WHERE phone = $1 RETURNING id, token_balance)
		INSERT INTO token_ledger (user_id, op, amount, balance_after, reason, actor_id)
		SELECT id, 'admin_adjustment', $2, token_balance, 'test', id FROM u`, "+998901112233", n); err != nil {
		t.Fatal(err)
	}
}

func TestTryOnHappyPath(t *testing.T) {
	f := newTryonFixture(t)
	c := f.cli
	ctx := context.Background()
	q := c.must(200, "GET", "/v1/tryon/quote", nil).data()
	if q["cost"].(float64) != 20 || q["balance"].(float64) != 20 || q["enough"] != true || q["consent"] != false {
		t.Fatalf("quote: %v", q)
	}
	if r := c.doH("POST", "/v1/tryon/generations", f.body(f.top), idem("gen-00000001")); r.Status != 409 || r.errCode() != "tryon_consent_required" {
		t.Fatalf("consent must be required: %d %s", r.Status, r.Raw)
	}
	c.must(204, "POST", "/v1/tryon/consent", nil)
	if r := c.do("POST", "/v1/tryon/generations", f.body(f.top)); r.Status != 400 || r.errCode() != "idempotency_key_required" {
		t.Fatalf("idempotency key required: %d", r.Status)
	}
	// одна вещь на зону
	if r := c.doH("POST", "/v1/tryon/generations", f.body(f.top, f.top2), idem("gen-00000002")); r.Status != 422 {
		t.Fatalf("two tops must be rejected: %d %s", r.Status, r.Raw)
	}
	body := f.body(f.hat, f.top)
	body["save_photos"] = true
	body["body"] = map[string]any{"height_cm": 170, "weight_kg": 60.5, "params": map[string]any{"waist_cm": 70}}
	r1 := c.doH("POST", "/v1/tryon/generations", body, idem("gen-00000003"))
	if r1.Status != 202 || r1.data()["status"] != "queued" {
		t.Fatalf("create: %d %s", r1.Status, r1.Raw)
	}
	// повтор с тем же ключом — тот же ответ, без второго списания
	r2 := c.doH("POST", "/v1/tryon/generations", body, idem("gen-00000003"))
	if r2.Status != 202 || r2.data()["id"] != r1.data()["id"] {
		t.Fatalf("idempotent replay: %d %s", r2.Status, r2.Raw)
	}
	if b := f.balance(t); b != 0 {
		t.Fatalf("balance after charge = %v, want 0", b)
	}
	id := r1.data()["id"].(string)

	f.runOnce(t)
	// токенов больше нет
	if r := c.doH("POST", "/v1/tryon/generations", f.body(f.top), idem("gen-00000004")); r.Status != 402 || r.errCode() != "insufficient_tokens" {
		t.Fatalf("insufficient: %d %s", r.Status, r.Raw)
	}
	g := c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data()
	if g["status"] != "succeeded" || g["result_url"] == nil || len(g["products"].([]any)) != 2 || len(g["photos"].([]any)) != 3 {
		t.Fatalf("generation: %v", g)
	}
	if bd := g["body"].(map[string]any); bd["height_cm"].(float64) != 170 {
		t.Fatalf("body snapshot: %v", bd)
	}
	var resultKey string
	_ = f.e.d.DB.QueryRow(ctx, `SELECT result_key FROM generations WHERE id = $1`, id).Scan(&resultKey)
	if !media.IsBiometricKey(resultKey) || !f.mem.Has(resultKey) {
		t.Fatalf("result must be stored as biometric object: %q", resultKey)
	}
	img, _, err := image.DecodeConfig(bytes.NewReader(f.mem.Objs[resultKey]))
	if err != nil || img.Width <= img.Height {
		t.Fatalf("result must be one wide composite of 3 views: %v %v", img, err)
	}
	// фото сохранены в профиле
	if ph := c.must(200, "GET", "/v1/tryon/photos", nil).Body["data"].([]any); len(ph) != 3 {
		t.Fatalf("saved photos: %v", ph)
	}
	// сохранить образ
	if c.must(200, "POST", "/v1/tryon/generations/"+id+"/save", nil).data()["saved"] != true {
		t.Fatal("save failed")
	}
	if l := c.must(200, "GET", "/v1/tryon/generations?saved=true", nil).Body["data"].([]any); len(l) != 1 {
		t.Fatalf("saved list: %v", l)
	}
	// чужой клиент не видит генерацию и не может использовать чужие фото
	other := f.e.client()
	other.loginClient("+998909998877")
	other.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "male", "pd_consent": true})
	other.must(204, "POST", "/v1/tryon/consent", nil)
	if r := other.do("GET", "/v1/tryon/generations/"+id, nil); r.Status != 404 {
		t.Fatalf("foreign generation visible: %d", r.Status)
	}
	if r := other.doH("POST", "/v1/tryon/generations", f.body(f.top), idem("gen-other-001")); r.Status != 422 {
		t.Fatalf("foreign photo keys accepted: %d %s", r.Status, r.Raw)
	}
	// магазин и админ не имеют доступа к клиентскому API; админ видит журнал без картинок
	if r := f.storeA.do("GET", "/v1/tryon/generations", nil); r.Status != 403 {
		t.Fatalf("store access: %d", r.Status)
	}
	adm := f.adm.must(200, "GET", "/v1/admin/tryon/generations", nil).Body["data"].([]any)
	if len(adm) != 1 || strings.Contains(f.adm.do("GET", "/v1/admin/tryon/generations", nil).Raw, "result_url") {
		t.Fatalf("admin list: %v", adm)
	}
	if st := f.adm.must(200, "GET", "/v1/admin/tryon/stats", nil).data(); st["succeeded_24h"].(float64) != 1 {
		t.Fatalf("stats: %v", st)
	}
	// удаление из истории удаляет результат; сохранённые фото остаются
	c.must(204, "DELETE", "/v1/tryon/generations/"+id, nil)
	if f.mem.Has(resultKey) {
		t.Fatal("result object must be deleted")
	}
	for _, k := range f.keys {
		if !f.mem.Has(k) {
			t.Fatal("saved photo must survive generation delete")
		}
	}
	if r := c.do("GET", "/v1/tryon/generations/"+id, nil); r.Status != 404 {
		t.Fatalf("deleted generation visible: %d", r.Status)
	}
	// удалить сохранённое фото
	c.must(204, "DELETE", "/v1/tryon/photos/front", nil)
	if f.mem.Has(f.keys[0]) {
		t.Fatal("deleted photo object must be removed")
	}
	// отзыв согласия удаляет всю биометрию
	c.must(204, "DELETE", "/v1/tryon/consent", nil)
	for _, k := range f.keys {
		if f.mem.Has(k) {
			t.Fatal("revoking consent must purge photos")
		}
	}
}

func (f *tryonFixture) create(t *testing.T, key string, products ...string) string {
	t.Helper()
	r := f.cli.doH("POST", "/v1/tryon/generations", f.body(products...), idem(key))
	if r.Status != 202 {
		t.Fatalf("create: %d %s", r.Status, r.Raw)
	}
	return r.data()["id"].(string)
}

func TestTryOnFailureRefundsTokens(t *testing.T) {
	f := newTryonFixture(t)
	c := f.cli
	ctx := context.Background()
	c.must(204, "POST", "/v1/tryon/consent", nil)

	// постоянная ошибка (нет человека на фото) — сразу failed и возврат токенов
	f.providerFail = func(tryon.Request) error { return &tryon.PermanentError{Code: tryon.CodePhotoRejected, Msg: "no pose"} }
	id := f.create(t, "fail-perm-01", f.top)
	if b := f.balance(t); b != 0 {
		t.Fatalf("charged balance %v", b)
	}
	f.runOnce(t)
	g := c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data()
	if g["status"] != "failed" || g["error_code"] != tryon.CodePhotoRejected || g["refunded"] != true {
		t.Fatalf("permanent failure: %v", g)
	}
	if b := f.balance(t); b != 20 {
		t.Fatalf("refund: balance %v, want 20", b)
	}

	// временная ошибка: повтор с задержкой, после MaxAttempts — failed и возврат
	f.providerFail = func(tryon.Request) error { return errors.New("503 upstream") }
	id = f.create(t, "fail-temp-01", f.top)
	f.runOnce(t)
	g = c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data()
	if g["status"] != "queued" {
		t.Fatalf("temporary failure must be retried: %v", g)
	}
	if did, _ := f.worker.RunOnce(ctx); did {
		t.Fatal("retry must wait for backoff")
	}
	_, _ = f.e.d.DB.Exec(ctx, `UPDATE generations SET next_run_at = now() WHERE id = $1`, id)
	f.runOnce(t)
	g = c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data()
	if g["status"] != "failed" || g["error_code"] != tryon.CodeProviderFailure || f.balance(t) != 20 {
		t.Fatalf("exhausted retries: %v balance=%v", g, f.balance(t))
	}
	// в журнале: 2 списания, 2 возврата — каждый ровно один раз
	var charges, refunds int
	_ = f.e.d.DB.QueryRow(ctx, `SELECT count(*) FILTER (WHERE op = 'generation_charge'), count(*) FILTER (WHERE op = 'generation_refund')
		FROM token_ledger`).Scan(&charges, &refunds)
	if charges != 2 || refunds != 2 {
		t.Fatalf("ledger: charges=%d refunds=%d", charges, refunds)
	}

	// упавший воркер: задание «running» с истёкшей арендой подхватывается снова
	f.providerFail = nil
	id = f.create(t, "stale-lease-1", f.top)
	_, _ = f.e.d.DB.Exec(ctx, `UPDATE generations SET status = 'running', attempts = 1, lease_until = now() - interval '1 second' WHERE id = $1`, id)
	f.runOnce(t)
	if g = c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data(); g["status"] != "succeeded" {
		t.Fatalf("stale lease not recovered: %v", g)
	}
	// фото удалено до запуска — failed без повторов, токены возвращены
	f.grant(t, 20)
	id = f.create(t, "photo-gone-1", f.top)
	_ = f.mem.Delete(ctx, f.keys[1])
	f.runOnce(t)
	if g = c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data(); g["error_code"] != tryon.CodePhotoMissing || g["refunded"] != true {
		t.Fatalf("missing photo: %v", g)
	}
}

func TestTryOnActiveLimitAndQueueConcurrency(t *testing.T) {
	f := newTryonFixture(t)
	f.cli.must(204, "POST", "/v1/tryon/consent", nil)
	f.grant(t, 200)
	f.create(t, "active-0001", f.top)
	// MaxActive в тестовом конфиге = 1 (минимум)
	if r := f.cli.doH("POST", "/v1/tryon/generations", f.body(f.hat), idem("active-0002")); r.Status != 429 || r.errCode() != "too_many_active_generations" {
		t.Fatalf("active limit: %d %s", r.Status, r.Raw)
	}
	// два воркера параллельно не берут одно задание
	ctx := context.Background()
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func() { did, _ := f.worker.RunOnce(ctx); results <- did }()
	}
	got := 0
	for i := 0; i < 2; i++ {
		if <-results {
			got++
		}
	}
	if got != 1 {
		t.Fatalf("job processed %d times", got)
	}
}

func TestTryOnRetention(t *testing.T) {
	f := newTryonFixture(t)
	c := f.cli
	ctx := context.Background()
	c.must(204, "POST", "/v1/tryon/consent", nil)
	ret := tryon.Retention{Photo: 24 * time.Hour, SavedPhoto: 180 * 24 * time.Hour, Result: 30 * 24 * time.Hour, SavedLook: 365 * 24 * time.Hour}

	id := f.create(t, "retention-01", f.top)
	f.runOnce(t)
	// сохранить только фронтальное фото
	c.must(200, "PUT", "/v1/tryon/photos/front", map[string]any{"key": f.keys[0]})
	pool, store := f.e.d.DB, f.e.d.Storage
	now := time.Now()

	if n, _ := tryon.PurgePhotos(ctx, pool, store, ret, now); n != 0 {
		t.Fatal("photos purged too early")
	}
	n, err := tryon.PurgePhotos(ctx, pool, store, ret, now.Add(25*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("purge photos: n=%d err=%v", n, err)
	}
	if !f.mem.Has(f.keys[0]) || f.mem.Has(f.keys[1]) || f.mem.Has(f.keys[2]) {
		t.Fatal("unsaved photos must be deleted, saved one kept")
	}
	g := c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data()
	if len(g["photos"].([]any)) != 0 || g["result_url"] == nil {
		t.Fatalf("after photo purge: %v", g)
	}
	var key string
	_ = pool.QueryRow(ctx, `SELECT result_key FROM generations WHERE id = $1`, id).Scan(&key)
	if n, _ := tryon.PurgeResults(ctx, pool, store, ret, now.Add(31*24*time.Hour)); n != 1 || f.mem.Has(key) {
		t.Fatal("result must be purged after 30 days")
	}
	if g := c.must(200, "GET", "/v1/tryon/generations/"+id, nil).data(); g["result_url"] != nil {
		t.Fatalf("purged result still linked: %v", g)
	}
	// сохранённое фото без использования — удаляется через 180 дней
	if n, _ := tryon.PurgeSavedPhotos(ctx, pool, store, ret, now.Add(181*24*time.Hour)); n != 1 || f.mem.Has(f.keys[0]) {
		t.Fatal("stale saved photo must be purged")
	}
	// брошенная загрузка — удаляется через сутки
	orphan := c.must(201, "POST", "/v1/uploads?purpose=body", pngFile(t)).data()["key"].(string)
	f.mem.Mod[orphan] = now.Add(-25 * time.Hour)
	if n, err := tryon.PurgeOrphans(ctx, pool, store, ret, now); err != nil || n != 1 || f.mem.Has(orphan) {
		t.Fatalf("orphan: n=%d err=%v", n, err)
	}
}

func TestFittingAddCounter(t *testing.T) {
	f := newTryonFixture(t)
	fc := &tryon.FittingCounter{Redis: f.e.d.Redis, DB: f.e.d.DB, Log: f.worker.Log}
	f.e.d.Fitting = fc
	f.e.srv.Config.Handler = New(f.e.d)
	anon := f.e.client()
	for i := 0; i < 3; i++ {
		anon.must(204, "POST", "/v1/fitting/adds", map[string]any{"product_id": f.top})
	}
	f.cli.must(204, "POST", "/v1/fitting/adds", map[string]any{"product_id": f.top})
	var n int64
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT fitting_adds FROM products WHERE id = $1`, f.top).Scan(&n)
	if n != 0 {
		t.Fatalf("counter must be buffered in Redis, got %d in DB before flush", n)
	}
	if err := fc.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT fitting_adds FROM products WHERE id = $1`, f.top).Scan(&n)
	if n != 2 {
		t.Fatalf("fitting_adds = %d, want 2 (one per visitor per day)", n)
	}
}

// TRYON_PROVIDER=off: примерка выключена целиком (запуск prod без неё).
func TestTryOn_Disabled(t *testing.T) {
	e := setup(t)
	e.d.Cfg.TryOn.Provider = "off"
	srv := httptest.NewServer(New(e.d))
	t.Cleanup(srv.Close)
	e.srv = srv

	c := e.client()
	c.loginClient("+998901114455")
	c.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "female", "pd_consent": true})
	for _, p := range []struct{ m, path string }{
		{"GET", "/v1/tryon/quote"},
		{"GET", "/v1/tryon/generations"},
		{"POST", "/v1/tryon/generations"},
		{"GET", "/v1/tryon/photos"},
	} {
		r := c.do(p.m, p.path, map[string]any{})
		if r.Status != http.StatusServiceUnavailable || r.errCode() != "tryon_disabled" {
			t.Errorf("%s %s: want 503 tryon_disabled, got %d %q", p.m, p.path, r.Status, r.errCode())
		}
	}
	if r := c.do("POST", "/v1/uploads?purpose=body", pngFile(t)); r.Status < 400 {
		t.Fatalf("body photo upload must be refused when try-on is off, got %d", r.Status)
	}
	if r := e.client().do("POST", "/v1/fitting/adds", map[string]any{"product_id": uuid.NewString()}); r.Status != http.StatusServiceUnavailable {
		t.Fatalf("fitting adds: want 503, got %d", r.Status)
	}
	// Остальной API работает.
	c.must(200, "GET", "/v1/me", nil)
}
