package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/tryon"
)

// fakeOpenAI — поддельный POST /v1/images/edits. mode задаёт ответ на следующий запрос.
type fakeOpenAI struct {
	mu       sync.Mutex
	mode     string // ok | 500 | moderation | empty | badb64 | slow
	requests int
	images   []int    // число картинок в каждом запросе
	prompts  []string // промпт каждого запроса
	result   []byte
	srv      *httptest.Server
}

func newFakeOpenAI(t *testing.T) *fakeOpenAI {
	f := &fakeOpenAI{mode: "ok", result: jpegOf(t, 1536, 1024, color.RGBA{10, 120, 200, 255})}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" || r.Header.Get("Authorization") != "Bearer sk-test" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.requests++
		f.images = append(f.images, len(r.MultipartForm.File["image[]"]))
		f.prompts = append(f.prompts, r.FormValue("prompt"))
		mode := f.mode
		f.mu.Unlock()
		for k, want := range map[string]string{"model": "gpt-image-2.5-sunburst", "quality": "high", "size": "1536x1024", "output_format": "jpeg", "n": "1"} {
			if r.FormValue(k) != want {
				http.Error(w, `{"error":{"message":"bad `+k+`"}}`, http.StatusBadRequest)
				return
			}
		}
		switch mode {
		case "500":
			http.Error(w, `{"error":{"message":"server error","type":"server_error"}}`, http.StatusInternalServerError)
		case "moderation":
			http.Error(w, `{"error":{"message":"Your request was rejected by the safety system","code":"moderation_blocked"}}`, http.StatusBadRequest)
		case "empty":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "badb64":
			_, _ = w.Write([]byte(`{"data":[{"b64_json":"!!!not-base64"}]}`))
		case "slow":
			time.Sleep(700 * time.Millisecond) // дольше таймаута клиента в тесте
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data":  []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(f.result)}},
				"usage": map[string]any{"input_tokens": 1000, "output_tokens": 6000},
			})
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeOpenAI) set(mode string) { f.mu.Lock(); f.mode = mode; f.mu.Unlock() }
func (f *fakeOpenAI) count() int      { f.mu.Lock(); defer f.mu.Unlock(); return f.requests }
func (f *fakeOpenAI) last() (int, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.images[len(f.images)-1], f.prompts[len(f.prompts)-1]
}

type openAIFixture struct {
	*tryonFixture
	ai            *fakeOpenAI
	bottom, shoes string
}

func newOpenAIFixture(t *testing.T) *openAIFixture {
	f := newTryonFixture(t, func(d *Deps) { d.Cfg.TryOn.Provider = "openai" })
	ai := newFakeOpenAI(t)
	f.worker.Provider = &tryon.OpenAI{APIKey: "sk-test", BaseURL: ai.srv.URL, Model: "gpt-image-2.5-sunburst",
		Quality: "high", Size: "1536x1024", Format: "jpeg", HTTP: &http.Client{Timeout: 300 * time.Millisecond}}
	of := &openAIFixture{tryonFixture: f, ai: ai}
	of.bottom, of.shoes = of.product(t, "bottom"), of.product(t, "shoes")
	f.grant(t, 1000)
	return of
}

func (f *openAIFixture) product(t *testing.T, zone string) string {
	ctx := context.Background()
	var id uuid.UUID
	if err := f.e.d.DB.QueryRow(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price, status)
		VALUES ($1, (SELECT id FROM categories WHERE zone = $2 LIMIT 1), $3, 'Вещь', 'D', 'Acme', 100000, 'published')
		RETURNING id`, f.storeAID, zone, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	key := "pub/u/" + uuid.NewString() + "/product/" + strings.Repeat("b", 32) + ".jpg"
	_ = f.mem.Put(ctx, key, jpegOf(t, 40, 60, color.RGBA{0, 200, 0, 255}), "image/jpeg")
	if _, err := f.e.d.DB.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, 'front', $2)`, id, key); err != nil {
		t.Fatal(err)
	}
	return id.String()
}

func (f *openAIFixture) gen(t *testing.T, id string) map[string]any {
	return f.cli.must(200, "GET", "/v1/tryon/generations/"+id, nil).data()
}

func (f *openAIFixture) ledger(t *testing.T) (charges, refunds int) {
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE op = 'generation_charge'),
		count(*) FILTER (WHERE op = 'generation_refund') FROM token_ledger`).Scan(&charges, &refunds)
	return
}

func TestTryOnOpenAIOneRequestPerGeneration(t *testing.T) {
	f := newOpenAIFixture(t)
	c := f.cli
	c.must(204, "POST", "/v1/tryon/consent", nil)

	// отдельное согласие на передачу фото в OpenAI обязательно
	q := c.must(200, "GET", "/v1/tryon/quote", nil).data()
	tr := q["transfer"].(map[string]any)
	if tr["required"] != true || tr["consent"] != false || !strings.Contains(tr["recipient"].(string), "OpenAI") {
		t.Fatalf("quote.transfer: %v", tr)
	}
	if r := c.doH("POST", "/v1/tryon/generations", f.body(f.top), idem("oa-consent-1")); r.Status != 409 || r.errCode() != "tryon_transfer_consent_required" {
		t.Fatalf("transfer consent must be required: %d %s", r.Status, r.Raw)
	}
	c.must(204, "POST", "/v1/tryon/consent/transfer", nil)
	if q := c.must(200, "GET", "/v1/tryon/quote", nil).data(); q["transfer"].(map[string]any)["consent"] != true {
		t.Fatalf("transfer consent not saved: %v", q)
	}

	cases := []struct {
		name     string
		products []string
		labels   []string // зоны, которые должны быть в промпте как референсы
		missing  string   // строка про отсутствующие зоны
	}{
		{"all four", []string{f.hat, f.top, f.bottom, f.shoes}, []string{"HEADWEAR", "TOP", "BOTTOM", "SHOES"}, ""},
		{"top only", []string{f.top}, []string{"TOP"}, "HEADWEAR, BOTTOM, SHOES"},
		{"top and bottom", []string{f.bottom, f.top}, []string{"TOP", "BOTTOM"}, "HEADWEAR, SHOES"},
		{"headwear and shoes", []string{f.shoes, f.hat}, []string{"HEADWEAR", "SHOES"}, "TOP, BOTTOM"},
	}
	for i, tc := range cases {
		before := f.ai.count()
		id := f.create(t, "oa-case-"+string(rune('a'+i))+"-0001", tc.products...)
		f.runOnce(t)
		if n := f.ai.count() - before; n != 1 {
			t.Fatalf("%s: %d OpenAI requests, want exactly 1", tc.name, n)
		}
		imgs, prompt := f.ai.last()
		if imgs != 3+len(tc.products) {
			t.Fatalf("%s: %d images in request, want %d", tc.name, imgs, 3+len(tc.products))
		}
		// порядок: 3 фото человека, затем вещи в порядке HEADWEAR, TOP, BOTTOM, SHOES
		for j, l := range tc.labels {
			if !strings.Contains(prompt, "Image "+string(rune('4'+j))+": "+l) {
				t.Fatalf("%s: image %d must be %s:\n%s", tc.name, 4+j, l, prompt)
			}
		}
		if tc.missing != "" && !strings.Contains(prompt, "No reference was provided for: "+tc.missing) {
			t.Fatalf("%s: missing categories not in prompt:\n%s", tc.name, prompt)
		}
		g := f.gen(t, id)
		if g["status"] != "succeeded" || g["result_url"] == nil || g["download_url"] == nil {
			t.Fatalf("%s: generation %v", tc.name, g)
		}
		var key string
		_ = f.e.d.DB.QueryRow(context.Background(), `SELECT result_key FROM generations WHERE id = $1`, id).Scan(&key)
		cfg, _, err := image.DecodeConfig(bytes.NewReader(f.mem.Objs[key]))
		// результат сохранён как есть (1536x1024): Compose дал бы высоту composeHeight+2*gap
		if err != nil || cfg.Width != 1536 || cfg.Height != 1024 {
			t.Fatalf("%s: stored result %dx%d (%v): must be OpenAI image without Compose", tc.name, cfg.Width, cfg.Height, err)
		}
		if !strings.Contains(g["download_url"].(string), "download=") {
			t.Fatalf("download url must force attachment: %v", g["download_url"])
		}
	}
	if ch, rf := f.ledger(t); ch != len(cases) || rf != 0 {
		t.Fatalf("ledger charges=%d refunds=%d", ch, rf)
	}

	// валидация до постановки в очередь: без вещей и без одного из фото
	if r := c.doH("POST", "/v1/tryon/generations", f.body(), idem("oa-noitems-01")); r.Status != 422 {
		t.Fatalf("no garments must be a validation error: %d %s", r.Status, r.Raw)
	}
	b := f.body(f.top)
	b["photos"] = map[string]any{"front": f.keys[0], "back": f.keys[2]}
	if r := c.doH("POST", "/v1/tryon/generations", b, idem("oa-nophoto-01")); r.Status != 422 {
		t.Fatalf("missing side photo must be a validation error: %d %s", r.Status, r.Raw)
	}
	if f.ai.count() != len(cases) {
		t.Fatal("invalid requests must not reach OpenAI")
	}
}

func TestTryOnOpenAIErrorsRetryAndRefund(t *testing.T) {
	f := newOpenAIFixture(t)
	ctx := context.Background()
	c := f.cli
	c.must(204, "POST", "/v1/tryon/consent", nil)
	c.must(204, "POST", "/v1/tryon/consent/transfer", nil)
	retryNow := func(id string) {
		_, _ = f.e.d.DB.Exec(ctx, `UPDATE generations SET next_run_at = now() WHERE id = $1`, id)
	}

	// 500, затем пустой ответ, затем таймаут — временные ошибки, повтор; токены не списываются повторно
	f.worker.MaxAttempts = 4
	f.ai.set("500")
	id := f.create(t, "oa-retry-0001", f.top)
	f.runOnce(t)
	if g := f.gen(t, id); g["status"] != "queued" {
		t.Fatalf("500 must be retried: %v", g)
	}
	f.ai.set("empty")
	retryNow(id)
	f.runOnce(t)
	if g := f.gen(t, id); g["status"] != "queued" {
		t.Fatalf("empty response must be retried: %v", g)
	}
	f.ai.set("slow")
	retryNow(id)
	f.runOnce(t)
	if g := f.gen(t, id); g["status"] != "queued" {
		t.Fatalf("timeout must be retried: %v", g)
	}
	f.ai.set("ok")
	retryNow(id)
	f.runOnce(t)
	if g := f.gen(t, id); g["status"] != "succeeded" || g["refunded"] != false {
		t.Fatalf("retry must succeed: %v", g)
	}
	if ch, rf := f.ledger(t); ch != 1 || rf != 0 {
		t.Fatalf("retries must not charge again: charges=%d refunds=%d", ch, rf)
	}

	// модерация — постоянная ошибка: failed без повторов, возврат токенов
	f.ai.set("moderation")
	before := f.ai.count()
	id = f.create(t, "oa-moder-0001", f.top)
	f.runOnce(t)
	if g := f.gen(t, id); g["status"] != "failed" || g["error_code"] != tryon.CodeContentBlocked || g["refunded"] != true {
		t.Fatalf("moderation: %v", g)
	}
	if f.ai.count()-before != 1 {
		t.Fatal("permanent error must not be retried")
	}

	// повреждённое изображение до исчерпания попыток — failed, возврат ровно один раз
	f.worker.MaxAttempts = 2
	f.ai.set("badb64")
	id = f.create(t, "oa-badimg-001", f.top)
	f.runOnce(t)
	retryNow(id)
	f.runOnce(t)
	if g := f.gen(t, id); g["status"] != "failed" || g["error_code"] != tryon.CodeProviderFailure || g["refunded"] != true {
		t.Fatalf("invalid image after retries: %v", g)
	}
	// задание, которое снова «всплыло» (истёкшая аренда), не возвращает токены второй раз
	_, _ = f.e.d.DB.Exec(ctx, `UPDATE generations SET status = 'running', lease_until = now() - interval '1 second' WHERE id = $1`, id)
	f.runOnce(t)
	if ch, rf := f.ledger(t); ch != 3 || rf != 2 {
		t.Fatalf("ledger: charges=%d refunds=%d, want 3/2", ch, rf)
	}

	// согласие на передачу отозвано, пока задание в очереди (смена провайдера и т.п.) —
	// фото не отправляются, токены возвращаются
	f.ai.set("ok")
	before = f.ai.count()
	id = f.create(t, "oa-noconsent-1", f.top)
	_, _ = f.e.d.DB.Exec(ctx, `UPDATE users SET tryon_transfer_provider = 'gemini' WHERE phone = '+998901112233'`)
	f.runOnce(t)
	if g := f.gen(t, id); g["status"] != "failed" || g["error_code"] != tryon.CodeTransferConsent || g["refunded"] != true {
		t.Fatalf("no transfer consent: %v", g)
	}
	if f.ai.count() != before {
		t.Fatal("photos must not be sent without transfer consent")
	}
	// отзыв согласия на передачу через API
	c.must(204, "DELETE", "/v1/tryon/consent/transfer", nil)
	if r := c.doH("POST", "/v1/tryon/generations", f.body(f.top), idem("oa-revoked-01")); r.errCode() != "tryon_transfer_consent_required" {
		t.Fatalf("revoked transfer consent: %d %s", r.Status, r.Raw)
	}
}
