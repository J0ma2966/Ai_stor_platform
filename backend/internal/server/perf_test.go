package server

import (
	"compress/gzip"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Серверный рендеринг сайта идёт с одного IP: с общим секретом он не упирается
// в общий лимит 600/мин, а без секрета (или с неверным) лимит действует как обычно.
func TestInternalTokenSkipsGlobalIPLimit(t *testing.T) {
	token := strings.Repeat("s", 40)
	e := setup(t, func(d *Deps) { d.Cfg.InternalAPIToken = token })
	c := e.client()
	if r := c.do("GET", "/v1/categories", nil); r.Status != 200 {
		t.Fatalf("first request: %d", r.Status)
	}
	// исчерпать лимит адреса 127.0.0.1
	for _, k := range e.mr.Keys() {
		if strings.HasPrefix(k, "rl:global_ip:") {
			_ = e.mr.Set(k, "600")
		}
	}
	if r := c.do("GET", "/v1/categories", nil); r.Status != 429 {
		t.Fatalf("over limit without token: %d", r.Status)
	}
	if r := c.doH("GET", "/v1/categories", nil, map[string]string{InternalTokenHeader: "wrong" + token}); r.Status != 429 {
		t.Fatalf("wrong token: %d", r.Status)
	}
	if r := c.doH("GET", "/v1/categories", nil, map[string]string{InternalTokenHeader: token}); r.Status != 200 {
		t.Fatalf("internal token: %d", r.Status)
	}
}

// JSON сжимается для браузеров и приложений; серверному рендерингу сайта (общий секрет)
// и клиентам без поддержки gzip ответ уходит как есть.
func TestJSONResponsesAreGzippedExceptInternal(t *testing.T) {
	token := strings.Repeat("s", 40)
	e := setup(t, func(d *Deps) { d.Cfg.InternalAPIToken = token })
	get := func(headers map[string]string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", e.srv.URL+"/v1/categories", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		// свой транспорт: стандартный сам распаковывает gzip и скрывает заголовок
		res, err := (&http.Transport{DisableCompression: true}).RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		return res
	}

	res := get(map[string]string{"Accept-Encoding": "gzip"})
	if res.StatusCode != 200 || res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("browser: status %d, encoding %q", res.StatusCode, res.Header.Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(zr).Decode(&body); err != nil || len(body.Data) == 0 {
		t.Fatalf("gzipped body: %v, %d categories", err, len(body.Data))
	}
	if cc := res.Header.Get("Cache-Control"); cc != "public, max-age=300" {
		t.Fatalf("cache header lost: %q", cc)
	}

	if res := get(nil); res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("no Accept-Encoding: got %q", res.Header.Get("Content-Encoding"))
	}
	if res := get(map[string]string{"Accept-Encoding": "gzip", InternalTokenHeader: token}); res.StatusCode != 200 || res.Header.Get("Content-Encoding") != "" {
		t.Fatalf("internal: status %d, encoding %q", res.StatusCode, res.Header.Get("Content-Encoding"))
	}
}
