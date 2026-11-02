package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

type ctxKey int

const (
	ctxRequestID ctxKey = iota
	ctxLogger
	ctxClientIP
)

func RequestID(ctx context.Context) string { s, _ := ctx.Value(ctxRequestID).(string); return s }
func ClientIP(ctx context.Context) string  { s, _ := ctx.Value(ctxClientIP).(string); return s }

func LoggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxLogger).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

var validReqID = func(s string) bool {
	if len(s) == 0 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// RequestContext проставляет request_id (принимает X-Request-ID от балансировщика,
// если он корректный), IP клиента и логгер с этими полями.
func RequestContext(base *slog.Logger, trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if !validReqID(id) {
				var b [12]byte
				_, _ = rand.Read(b[:])
				id = hex.EncodeToString(b[:])
			}
			ip := clientIP(r, trustProxy)
			w.Header().Set("X-Request-ID", id)
			ctx := context.WithValue(r.Context(), ctxRequestID, id)
			ctx = context.WithValue(ctx, ctxClientIP, ip)
			ctx = context.WithValue(ctx, ctxLogger, base.With(slog.String("request_id", id)))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// берём ближайший к нам адрес, добавленный доверенным балансировщиком
			parts := strings.Split(xff, ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total", Help: "HTTP requests by route and status.",
	}, []string{"method", "route", "status"})
	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "http_request_duration_seconds", Help: "HTTP request latency.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "route"})
)

func init() { prometheus.MustRegister(httpRequests, httpDuration) }

// AccessLog пишет одну JSON-строку на запрос и метрики Prometheus.
// В лог не попадают тело, query string и заголовки (там могут быть ПД и токены).
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		d := time.Since(start)
		httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(sw.status)).Inc()
		httpDuration.WithLabelValues(r.Method, route).Observe(d.Seconds())
		lvl := slog.LevelInfo
		if sw.status >= 500 {
			lvl = slog.LevelError
		}
		LoggerFrom(r.Context()).Log(r.Context(), lvl, "http",
			slog.String("method", r.Method), slog.String("route", route), slog.String("path", r.URL.Path),
			slog.Int("status", sw.status), slog.Int("bytes", sw.bytes),
			slog.Duration("dur", d), slog.String("ip", ClientIP(r.Context())))
	})
}

// Recover перехватывает панику, логирует стек и отвечает 500.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				LoggerFrom(r.Context()).Error("panic", slog.Any("panic", p), slog.String("stack", string(debug.Stack())))
				Fail(w, r, ErrInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// SecurityHeaders — строгие заголовки для JSON API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// CORS разрешает только явно перечисленные origin, с cookie.
func CORS(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Request-ID")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
