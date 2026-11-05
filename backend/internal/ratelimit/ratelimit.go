// Package ratelimit — лимиты запросов в Redis (фиксированное окно, атомарно через Lua).
// Работает одинаково на любом числе инстансов API.
package ratelimit

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/httpx"
)

var script = redis.NewScript(`
local n = redis.call('INCR', KEYS[1])
if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return {n, redis.call('PTTL', KEYS[1])}
`)

type Limiter struct {
	rdb *redis.Client
	// FailOpen: если Redis недоступен — пропускать (true) или отклонять (false).
	FailOpen bool
}

func New(rdb *redis.Client) *Limiter { return &Limiter{rdb: rdb, FailOpen: true} }

type Rule struct {
	Name   string
	Limit  int
	Window time.Duration
}

// Allow увеличивает счётчик и возвращает, разрешён ли запрос, и через сколько повторить.
func (l *Limiter) Allow(ctx context.Context, rule Rule, subject string) (bool, time.Duration, error) {
	key := "rl:" + rule.Name + ":" + subject
	res, err := script.Run(ctx, l.rdb, []string{key}, rule.Window.Milliseconds()).Int64Slice()
	if err != nil {
		return l.FailOpen, 0, err
	}
	return res[0] <= int64(rule.Limit), time.Duration(res[1]) * time.Millisecond, nil
}

// ByIP — middleware с лимитом по IP клиента.
func (l *Limiter) ByIP(rule Rule) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.Check(w, r, rule, httpx.ClientIP(r.Context())) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ByIPExcept — ByIP, но запросы, для которых skip вернул true, лимит не считают
// (серверный рендеринг сайта, см. config.InternalAPIToken).
func (l *Limiter) ByIPExcept(rule Rule, skip func(*http.Request) bool) func(http.Handler) http.Handler {
	byIP := l.ByIP(rule)
	return func(next http.Handler) http.Handler {
		limited := byIP(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip(r) {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}

// Check применяет правило; при превышении сам пишет 429 и возвращает false.
func (l *Limiter) Check(w http.ResponseWriter, r *http.Request, rule Rule, subject string) bool {
	ok, retry, err := l.Allow(r.Context(), rule, subject)
	if err != nil {
		httpx.LoggerFrom(r.Context()).Warn("rate limiter unavailable", slog.String("err", err.Error()))
	}
	if !ok {
		if retry > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
		}
		httpx.Fail(w, r, httpx.ErrTooMany)
	}
	return ok
}
