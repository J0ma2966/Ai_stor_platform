package server

import (
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
