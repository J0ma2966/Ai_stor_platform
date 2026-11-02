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
	"github.com/boutique-market/backend/internal/catalogadmin"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/notify"
	"github.com/boutique-market/backend/internal/orders"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/ratelimit"
	"github.com/boutique-market/backend/internal/shop"
	"github.com/boutique-market/backend/internal/stores"
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
	// UserPurge — очистка данных клиента в других модулях при удалении аккаунта.
	UserPurge []users.PurgeFunc
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
	usersH := &users.Handlers{DB: d.DB, Revoker: revoker, Purge: d.UserPurge}
	productsH := &products.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL, Views: d.Views, Media: d.Storage}
	shopH := &shop.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	storesH := &stores.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	notifyH := &notify.Handlers{DB: d.DB}
	catalogH := &catalogadmin.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	mediaH := &media.Handlers{Store: d.Storage, Limiter: limiter, Redis: d.Redis, PublicBase: d.Cfg.PublicMediaURL,
		MaxBytes: int64(d.Cfg.MaxUploadMB) << 20, Sem: media.NewSem(max(d.Cfg.UploadConcurrency, 1))}
	adminH := &admin.Handlers{DB: d.DB, Revoker: revoker}
	ordersH := &orders.Handlers{DB: d.DB, Media: d.Storage, Limiter: limiter, PublicBase: d.Cfg.PublicMediaURL}

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
			storesH.PublicRoutes(r)
			catalogH.PublicRoutes(r)

			r.Group(func(r chi.Router) {
				r.Use(auth.RequireRole(auth.RoleClient, auth.RoleStore, auth.RoleAdmin))
				r.Route("/me", func(r chi.Router) {
					usersH.Routes(r)
					r.Route("/notifications", notifyH.Routes)
					r.With(auth.RequireRole(auth.RoleClient)).Route("/cart", shopH.CartRoutes)
					r.With(auth.RequireRole(auth.RoleClient)).Route("/favorites", shopH.FavoriteRoutes)
				})
				// Заказы клиента: оформление, список, статусы.
				r.With(auth.RequireRole(auth.RoleClient)).Route("/orders", ordersH.ClientRoutes)
			})

			r.Route("/store", func(r chi.Router) {
				r.Use(auth.RequireRole(auth.RoleStore))
				productsH.StoreRoutes(r)
				storesH.StoreRoutes(r)
				// Окно доставки магазина: только заказы своего магазина.
				r.Route("/orders", ordersH.StoreRoutes)
			})

			r.Route("/admin", func(r chi.Router) {
				r.Use(auth.RequireRole(auth.RoleAdmin))
				adminH.Routes(r)
				catalogH.AdminRoutes(r)
				r.Route("/orders", ordersH.AdminRoutes)
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
