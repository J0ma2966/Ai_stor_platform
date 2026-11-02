package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sample struct {
	Phone string `json:"phone" validate:"required,e164"`
	Qty   int    `json:"qty" validate:"min=1,max=99"`
}

func TestDecode(t *testing.T) {
	cases := map[string]struct {
		body string
		code string
	}{
		"ok":            {`{"phone":"+998901234567","qty":1}`, ""},
		"unknown field": {`{"phone":"+998901234567","qty":1,"role":"admin"}`, "invalid_json"},
		"bad phone":     {`{"phone":"12345","qty":1}`, "validation_failed"},
		"trailing":      {`{"phone":"+998901234567","qty":1}{}`, "invalid_json"},
		"empty":         {``, "invalid_json"},
		"too big":       {`{"phone":"` + strings.Repeat("1", 2<<20) + `"}`, "payload_too_large"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body))
			var s sample
			err := Decode(httptest.NewRecorder(), r, &s)
			if c.code == "" {
				if err != nil {
					t.Fatalf("unexpected %v", err)
				}
				return
			}
			if e := AsError(err); e == nil || e.Code != c.code {
				t.Fatalf("got %v, want %s", err, c.code)
			}
		})
	}
}

func TestFailHidesInternalErrors(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	Fail(w, r, errString("pq: password authentication failed for user secret"))
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("internal error leaked: %d %s", w.Code, w.Body.String())
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestRequestIDSanitized(t *testing.T) {
	h := RequestContext(nil_logger(), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Request-ID", "bad\nheader injection")
	h.ServeHTTP(w, r)
	if id := w.Header().Get("X-Request-ID"); id == "" || strings.ContainsAny(id, "\n ") {
		t.Fatalf("request id not sanitized: %q", id)
	}
}
