package server

// Security regression tests owned by the testing/security thread.
// Each test maps to a threat in the threat model (docs/SECURITY.md).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/boutique-market/backend/internal/admin"
)

// publicRoutes — routes reachable without a token. Anything else must answer 401
// to an anonymous caller. A new public route must be added here consciously.
var publicRoutes = map[string]bool{
	"GET /healthz":                     true,
	"GET /readyz":                      true,
	"POST /v1/auth/otp/request":        true,
	"POST /v1/auth/otp/verify":         true,
	"POST /v1/auth/login":              true,
	"POST /v1/auth/refresh":            true,
	"POST /v1/auth/logout":             true,
	"POST /v1/auth/password/login":     true,
	"POST /v1/auth/password/reset":     true,
	"GET /v1/auth/email/status":        true, // ВРЕМЕННО: вход по почте на время тестирования
	"POST /v1/auth/email/register":     true,
	"POST /v1/auth/email/login":        true,
	"GET /v1/categories":               true,
	"GET /v1/products":                 true,
	"GET /v1/products/{id}":            true,
	"GET /v1/tariffs":                  true,
	"POST /v1/fitting/adds":            true, // счётчик «В примерочную»: работает и без входа, лимит по IP + дедуп
	"POST /v1/payments/click/prepare":  true, // provider webhooks: authenticated by signature, not by user token
	"POST /v1/payments/click/complete": true,
	"POST /v1/payments/payme":          true,
	"POST /v1/telegram/webhook":        true, // Telegram: secret header, 404 without it
	"GET /v1/stores":                   true,
	"GET /v1/stores/{slug}":            true,
	"GET /v1/platform":                 true,
}

// rolePrefixes — which roles may reach a route prefix. Every other authenticated role must get 403.
var rolePrefixes = []struct {
	prefix string
	roles  map[string]bool
}{
	{"/v1/admin/", map[string]bool{"admin": true}},
	{"/v1/store/", map[string]bool{"store": true}},
}

func walkRoutes(t *testing.T, h http.Handler) []string {
	t.Helper()
	var out []string
	err := chi.Walk(h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.ReplaceAll(route, "/*/", "/")
		route = strings.TrimSuffix(route, "/")
		if route == "" {
			route = "/"
		}
		out = append(out, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func fill(route string) string {
	// replace {param} with a random-looking uuid so handlers get past parsing
	for {
		i := strings.Index(route, "{")
		if i < 0 {
			return route
		}
		j := strings.Index(route[i:], "}")
		route = route[:i] + "00000000-0000-4000-8000-000000000001" + route[i+j+1:]
	}
}

// T1: anonymous caller never reaches a protected handler; every route is either
// explicitly public or rejects anonymous access with 401.
func TestSecurity_EveryRouteRequiresAuthUnlessPublic(t *testing.T) {
	e := setup(t)
	anon := e.client()
	routes := walkRoutes(t, e.srv.Config.Handler)
	if len(routes) < 10 {
		t.Fatalf("route walk found too few routes: %v", routes)
	}
	for _, rt := range routes {
		if publicRoutes[rt] {
			continue
		}
		method, path, _ := strings.Cut(rt, " ")
		if method == http.MethodOptions || method == http.MethodHead {
			continue
		}
		r := anon.do(method, fill(path), map[string]any{})
		if r.Status != http.StatusUnauthorized {
			t.Errorf("%s: anonymous got %d, want 401 (add to publicRoutes only if it is meant to be public)", rt, r.Status)
		}
	}
}

// T1/T2: role isolation across /admin and /store for all three roles.
func TestSecurity_RoleIsolationMatrix(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := admin.CreateAdmin(ctx, e.d.DB, "root", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	adm := e.client()
	adm.token = adm.must(200, "POST", "/v1/auth/login", map[string]any{"login": "root", "password": "admin-password-123"}).data()["access_token"].(string)
	sid := adm.must(201, "POST", "/v1/admin/stores", map[string]any{"slug": "iso", "name": "Iso", "login": "iso", "password": "store-password-1"}).data()["store_id"].(string)
	adm.must(200, "POST", "/v1/admin/stores/"+sid+"/status", map[string]any{"status": "active"})
	st := e.client()
	st.token = st.must(200, "POST", "/v1/auth/login", map[string]any{"login": "iso", "password": "store-password-1"}).data()["access_token"].(string)
	cl := e.client()
	cl.loginClient("+998901110000")

	callers := map[string]*client{"admin": adm, "store": st, "client": cl}
	for _, rt := range walkRoutes(t, e.srv.Config.Handler) {
		method, path, _ := strings.Cut(rt, " ")
		for _, pr := range rolePrefixes {
			if !strings.HasPrefix(path+"/", pr.prefix) {
				continue
			}
			for role, c := range callers {
				if pr.roles[role] {
					continue
				}
				if r := c.do(method, fill(path), map[string]any{}); r.Status != http.StatusForbidden {
					t.Errorf("%s as %s: got %d, want 403", rt, role, r.Status)
				}
			}
		}
	}
}

// T6: SMS pumping. Only Uzbek numbers may request an OTP, otherwise anyone can
// make us send paid SMS to premium international numbers.
func TestSecurity_OTPOnlyUzbekNumbers(t *testing.T) {
	e := setup(t)
	c := e.client()
	for _, phone := range []string{"+447700900123", "+12025550123", "+88213000000"} {
		if r := c.do("POST", "/v1/auth/otp/request", map[string]any{"phone": phone}); r.Status != http.StatusUnprocessableEntity {
			t.Errorf("OTP for %s: got %d, want 422", phone, r.Status)
		}
	}
	if got := e.sms.last("+447700900123"); got != "" {
		t.Error("SMS was sent to a non-UZ number")
	}
}

// T6: brute force. 5 wrong codes burn the code; the right code after that is rejected.
func TestSecurity_OTPBurnsAfterMaxTries(t *testing.T) {
	e := setup(t)
	c := e.client()
	const phone = "+998901112233"
	c.must(202, "POST", "/v1/auth/otp/request", map[string]any{"phone": phone})
	right := e.sms.last(phone)
	wrong := "000000"
	if right == wrong {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		if r := c.do("POST", "/v1/auth/otp/verify", map[string]any{"phone": phone, "code": wrong}); r.Status != 401 {
			t.Fatalf("wrong code %d: %d", i, r.Status)
		}
	}
	if r := c.do("POST", "/v1/auth/otp/verify", map[string]any{"phone": phone, "code": right}); r.Status != 401 {
		t.Fatalf("code must be burned after 5 wrong tries, got %d", r.Status)
	}
}

// T10: a blocked store must lose access immediately, not when its 15-minute access token expires.
func TestSecurity_BlockedStoreLosesAccessImmediately(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	if _, err := admin.CreateAdmin(ctx, e.d.DB, "root", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	adm := e.client()
	adm.token = adm.must(200, "POST", "/v1/auth/login", map[string]any{"login": "root", "password": "admin-password-123"}).data()["access_token"].(string)
	sid := adm.must(201, "POST", "/v1/admin/stores", map[string]any{"slug": "bad", "name": "Bad", "login": "bad", "password": "store-password-1"}).data()["store_id"].(string)
	adm.must(200, "POST", "/v1/admin/stores/"+sid+"/status", map[string]any{"status": "active"})
	st := e.client()
	st.token = st.must(200, "POST", "/v1/auth/login", map[string]any{"login": "bad", "password": "store-password-1"}).data()["access_token"].(string)
	st.must(200, "GET", "/v1/store/products", nil)

	adm.must(200, "POST", "/v1/admin/stores/"+sid+"/status", map[string]any{"status": "blocked", "reason": "fraud"})
	if r := st.do("GET", "/v1/store/products", nil); r.Status != http.StatusForbidden && r.Status != http.StatusUnauthorized {
		t.Fatalf("blocked store still has access with its old access token: %d", r.Status)
	}
}

// T7: refresh cookie flags.
func TestSecurity_RefreshCookieFlags(t *testing.T) {
	e := setup(t)
	e.d.Cfg.CookieSecure = true
	srv := New(e.d)
	c := e.client()
	_ = c
	req, _ := http.NewRequest("POST", "/v1/auth/otp/request", strings.NewReader(`{"phone":"+998901119999"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	req, _ = http.NewRequest("POST", "/v1/auth/otp/verify", strings.NewReader(`{"phone":"+998901119999","code":"`+e.sms.last("+998901119999")+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	sc := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"HttpOnly", "Secure", "SameSite=Lax", "Path=/v1/auth"} {
		if !strings.Contains(sc, want) {
			t.Errorf("refresh cookie missing %s: %q", want, sc)
		}
	}
}

// T7: security headers on every response, including errors.
func TestSecurity_HeadersOnErrors(t *testing.T) {
	e := setup(t)
	r, err := http.Get(e.srv.URL + "/v1/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	for h, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Strict-Transport-Security": "max-age=",
		"Content-Security-Policy":   "default-src 'none'",
		"Cache-Control":             "no-store",
	} {
		if !strings.Contains(r.Header.Get(h), want) {
			t.Errorf("%s = %q, want %q", h, r.Header.Get(h), want)
		}
	}
}

// T7: CORS only for allow-listed origins.
func TestSecurity_CORSRejectsUnknownOrigin(t *testing.T) {
	e := setup(t)
	req, _ := http.NewRequest("OPTIONS", e.srv.URL+"/v1/me", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "GET")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unknown origin got CORS allow header")
	}
}

// T9: oversized JSON bodies are rejected before parsing.
func TestSecurity_LargeJSONBodyRejected(t *testing.T) {
	e := setup(t)
	c := e.client()
	big := strings.Repeat("a", 2<<20)
	r := c.do("POST", "/v1/auth/otp/request", map[string]any{"phone": big})
	if r.Status != http.StatusRequestEntityTooLarge && r.Status != http.StatusBadRequest && r.Status != http.StatusUnprocessableEntity {
		t.Fatalf("2MB JSON accepted with status %d", r.Status)
	}
}
