// Package server собирает HTTP-роутер из модулей.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/admin"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/orders"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/ratelimit"
	"github.com/boutique-market/backend/internal/tryon"
	"github.com/boutique-market/backend/internal/users"
)

type Deps struct {
	Cfg     config.Config
	Log     *slog.Logger
	DB      *pgxpool.Pool
	Redis   *redis.Client
	Storage media.Storage
	SMS     auth.SMSSender
	Views   *products.ViewCounter // nil — просмотры не считаются (тесты)
	Fitting *tryon.FittingCounter // nil — счётчик «В примерочную» отключён
}

func New(d Deps) http.Handler {
	signer := auth.NewSigner(d.Cfg.JWTSecret, d.Cfg.AccessTTL)
	limiter := ratelimit.New(d.Redis)
	authSvc := &auth.Service{
		DB: d.DB, Signer: signer, SMS: d.SMS, RefreshTTL: d.Cfg.RefreshTTL,
		OTP: auth.NewOTP(d.Redis, d.Cfg.OTPPepper, d.Cfg.OTPTTL, d.Cfg.OTPCooldown, d.Cfg.OTPMaxTries),
	}
	revoker := auth.NewRevoker(d.Redis, d.Cfg.AccessTTL)
	authH := &auth.Handlers{Svc: authSvc, Limiter: limiter, AllowedPrefixes: d.Cfg.OTPAllowedPrefixes,
		CookieDomain: d.Cfg.CookieDomain, CookieSecure: d.Cfg.CookieSecure}
	usersH := &users.Handlers{DB: d.DB}
	productsH := &products.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL, Views: d.Views}
	mediaH := &media.Handlers{Store: d.Storage, Limiter: limiter, Redis: d.Redis, PublicBase: d.Cfg.PublicMediaURL,
		MaxBytes: int64(d.Cfg.MaxUploadMB) << 20, Sem: media.NewSem(max(d.Cfg.UploadConcurrency, 1))}
	if !d.Cfg.TryOnEnabled() {
		mediaH.Disabled = map[media.Purpose]bool{media.PurposeBodyPhoto: true}
	}
	adminH := &admin.Handlers{DB: d.DB, Revoker: revoker}
	ordersH := &orders.Handlers{DB: d.DB, Media: d.Storage, Limiter: limiter, PublicBase: d.Cfg.PublicMediaURL}
	tryonH := &tryon.Handlers{DB: d.DB, Store: d.Storage, Limiter: limiter, Redis: d.Redis, PublicBase: d.Cfg.PublicMediaURL,
		Retention: RetentionFrom(d.Cfg), MaxActive: d.Cfg.TryOn.MaxActive, Provider: d.Cfg.TryOn.Provider, Fitting: d.Fitting}

	r := chi.NewRouter()
	r.Use(httpx.RequestContext(d.Log, d.Cfg.TrustedProxy))
	r.Use(httpx.AccessLog)
	r.Use(httpx.Recover)
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.CORS(d.Cfg.CORSOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", ready(d))

	r.Route("/v1", func(r chi.Router) {
		r.Use(limiter.ByIP(ratelimit.Rule{Name: "global_ip", Limit: 600, Window: time.Minute}))
		r.Use(auth.Authenticate(signer, revoker))

		// Загрузка файлов — отдельно: медленный мобильный интернет, свой таймаут 60 с.
		r.With(timeout(60*time.Second), auth.RequireRole(auth.RoleClient, auth.RoleStore, auth.RoleAdmin)).
			Post("/uploads", mediaH.Upload)

		r.Group(func(r chi.Router) {
			r.Use(timeout(15 * time.Second))

			r.Route("/auth", authH.Routes)
			productsH.PublicRoutes(r)
			if d.Cfg.TryOnEnabled() {
				tryonH.PublicRoutes(r)
			} else {
				r.Post("/fitting/adds", tryon.Disabled)
			}

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireRole(auth.RoleClient, auth.RoleStore, auth.RoleAdmin))
				r.Route("/me", usersH.Routes)
				// Заказы клиента: оформление, список, статусы.
				r.With(auth.RequireRole(auth.RoleClient)).Route("/orders", ordersH.ClientRoutes)
				// AI-примерка: фото, генерации, сохранённые образы.
				if d.Cfg.TryOnEnabled() {
					r.With(auth.RequireRole(auth.RoleClient)).Route("/tryon", tryonH.ClientRoutes)
				} else {
					r.With(auth.RequireRole(auth.RoleClient)).HandleFunc("/tryon/*", tryon.Disabled)
				}
			})

			r.Route("/store", func(r chi.Router) {
				r.Use(auth.RequireRole(auth.RoleStore))
				productsH.StoreRoutes(r)
				// Окно доставки магазина: только заказы своего магазина.
				r.Route("/orders", ordersH.StoreRoutes)
			})

			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireRole(auth.RoleAdmin))
				adminH.Routes(r)
				r.Route("/orders", ordersH.AdminRoutes)
				r.Route("/tryon", tryonH.AdminRoutes)
			})
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { httpx.Fail(w, r, httpx.ErrNotFound) })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.Fail(w, r, httpx.NewError(http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed"))
	})
	return r
}

func ready(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		checks := map[string]string{"db": "ok", "redis": "ok"}
		code := http.StatusOK
		if err := d.DB.Ping(ctx); err != nil {
			checks["db"], code = "fail", http.StatusServiceUnavailable
		}
		if err := d.Redis.Ping(ctx).Err(); err != nil {
			checks["redis"], code = "fail", http.StatusServiceUnavailable
		}
		httpx.JSON(w, code, checks)
	}
}

// timeout отменяет контекст запроса (и SQL-запросы) после d.
func timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RetentionFrom — сроки хранения биометрии из конфигурации.
func RetentionFrom(c config.Config) tryon.Retention {
	return tryon.Retention{Photo: c.TryOn.PhotoTTL, SavedPhoto: c.TryOn.SavedPhotoTTL, Result: c.TryOn.ResultTTL, SavedLook: c.TryOn.SavedLookTTL}
}
