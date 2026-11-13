// Package server собирает HTTP-роутер из модулей.
package server

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/admin"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/catalogadmin"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/mail"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/notify"
	"github.com/boutique-market/backend/internal/orders"
	"github.com/boutique-market/backend/internal/payments"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/promotions"
	"github.com/boutique-market/backend/internal/ratelimit"
	"github.com/boutique-market/backend/internal/reviews"
	"github.com/boutique-market/backend/internal/shop"
	"github.com/boutique-market/backend/internal/stores"
	"github.com/boutique-market/backend/internal/telegram"
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
	Mail    mail.Sender // nil — письма с кодами пишутся в лог (dev/test)
	// GoogleCertsURL — подмена адреса ключей Google в тестах; пусто — настоящий.
	GoogleCertsURL string
	Views          *products.ViewCounter // nil — просмотры не считаются (тесты)
	// UserPurge — очистка данных клиента в других модулях при удалении аккаунта.
	UserPurge []users.PurgeFunc
	Fitting   *tryon.FittingCounter // nil — счётчик «В примерочную» отключён
	Telegram  *telegram.Bot         // nil — бот магазинов выключен
}

func New(d Deps) http.Handler {
	signer := auth.NewSigner(d.Cfg.JWTSecret, d.Cfg.AccessTTL)
	limiter := ratelimit.New(d.Redis)
	mailer := d.Mail
	if mailer == nil {
		mailer = mail.LogSender{Log: d.Log}
	}
	authSvc := &auth.Service{
		DB: d.DB, Redis: d.Redis, Signer: signer, SMS: d.SMS, RefreshTTL: d.Cfg.RefreshTTL,
		OTP:      auth.NewOTP(d.Redis, d.Cfg.OTPPepper, d.Cfg.OTPTTL, d.Cfg.OTPCooldown, d.Cfg.OTPMaxTries),
		EmailOTP: auth.NewOTP(d.Redis, d.Cfg.OTPPepper, 10*time.Minute, time.Minute, 5),
		Mail:     mailer,
	}
	if len(d.Cfg.GoogleClientIDs) > 0 {
		authSvc.Google = &auth.GoogleVerifier{ClientIDs: d.Cfg.GoogleClientIDs, CertsURL: d.GoogleCertsURL}
	}
	telegramLogin := d.Telegram != nil && d.Cfg.TelegramBotUsername != ""
	if telegramLogin {
		authSvc.TelegramBot = d.Cfg.TelegramBotUsername
	}
	revoker := auth.NewRevoker(d.Redis, d.Cfg.AccessTTL)
	authH := &auth.Handlers{Svc: authSvc, Limiter: limiter, AllowedPrefixes: d.Cfg.OTPAllowedPrefixes,
		CookieDomain: d.Cfg.CookieDomain, CookieSecure: d.Cfg.CookieSecure}
	usersH := &users.Handlers{DB: d.DB, Revoker: revoker, Purge: d.UserPurge}
	productsH := &products.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL, Views: d.Views, Media: d.Storage, Limiter: limiter}
	shopH := &shop.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	storesH := &stores.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	notifyH := &notify.Handlers{DB: d.DB}
	reviewsH := &reviews.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	promoH := &promotions.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	catalogH := &catalogadmin.Handlers{DB: d.DB, PublicBase: d.Cfg.PublicMediaURL}
	mediaH := &media.Handlers{Store: d.Storage, Limiter: limiter, Redis: d.Redis, PublicBase: d.Cfg.PublicMediaURL,
		MaxBytes: int64(d.Cfg.MaxUploadMB) << 20, Sem: media.NewSem(max(d.Cfg.UploadConcurrency, 1))}
	if !d.Cfg.TryOnEnabled() {
		mediaH.Disabled = map[media.Purpose]bool{media.PurposeBodyPhoto: true}
	}
	adminH := &admin.Handlers{DB: d.DB, Revoker: revoker}
	ordersH := &orders.Handlers{DB: d.DB, Media: d.Storage, Limiter: limiter, PublicBase: d.Cfg.PublicMediaURL,
		SiteURL: d.Cfg.SiteURL, OrderSMS: d.Cfg.OrderSMS, AllowedPrefixes: d.Cfg.OTPAllowedPrefixes}
	telegramH := &telegram.Handlers{DB: d.DB, Bot: d.Telegram, BotUsername: d.Cfg.TelegramBotUsername,
		Secret: d.Cfg.TelegramWebhookSecret, Orders: ordersH, Login: authSvc}
	tryonH := &tryon.Handlers{DB: d.DB, Store: d.Storage, Limiter: limiter, Redis: d.Redis, PublicBase: d.Cfg.PublicMediaURL,
		Retention: RetentionFrom(d.Cfg), MaxActive: d.Cfg.TryOn.MaxActive, Provider: d.Cfg.TryOn.Provider, Fitting: d.Fitting}
	// Удаление аккаунта стирает и биометрию (фото тела, результаты примерок).
	usersH.Purge = append(usersH.Purge, func(ctx context.Context, tx pgx.Tx, userID uuid.UUID) error {
		return tryon.PurgeUser(ctx, tx, tryonH.Store, userID)
	})
	// Удаление аккаунта стирает почту, пароль и привязки Telegram и Google.
	usersH.Purge = append(usersH.Purge, auth.PurgeIdentities)
	// Отзывы удалённого клиента убираются, средние оценки товаров пересчитываются.
	usersH.Purge = append(usersH.Purge, reviews.PurgeUser)
	usersH.SMSAuth = d.Cfg.SMSAuthEnabled
	paymentsH, err := payments.New(d.DB, d.Cfg, limiter)
	if err != nil {
		panic("payments config: " + err.Error()) // неверный CLICK_/PAYME_ALLOWED_IPS — ошибка конфигурации
	}

	r := chi.NewRouter()
	r.Use(httpx.RequestContext(d.Log, d.Cfg.TrustedProxy))
	r.Use(httpx.AccessLog)
	r.Use(httpx.Recover)
	r.Use(httpx.SecurityHeaders)
	r.Use(httpx.CORS(d.Cfg.CORSOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", ready(d))

	r.Route("/v1", func(r chi.Router) {
		// Вебхуки Click/Payme: без пользовательского токена и общего лимита по IP
		// (все запросы идут с нескольких IP провайдера); защищены подписью/ключом кассы.
		r.Group(func(r chi.Router) {
			r.Use(timeout(15 * time.Second))
			paymentsH.WebhookRoutes(r)
			// Telegram-бот магазинов; защищён секретом в заголовке.
			r.Post("/telegram/webhook", telegramH.Webhook)
		})
		r.Group(func(r chi.Router) {
			// Сайт рендерит страницы на сервере: все его запросы приходят с одного IP,
			// поэтому с общим секретом общий лимит по IP к ним не применяется.
			r.Use(limiter.ByIPExcept(ratelimit.Rule{Name: "global_ip", Limit: 600, Window: time.Minute}, internalRequest(d.Cfg.InternalAPIToken)))
			// JSON браузерам и приложениям уходит сжатым (страница каталога: 22 КБ → около 3 КБ),
			// даже если прокси перед API сам не сжимает. Сайту при серверном рендеринге
			// сжатие не нужно: он рядом, это была бы лишняя работа процессора с обеих сторон.
			r.Use(compressExcept(internalRequest(d.Cfg.InternalAPIToken)))
			r.Use(auth.Authenticate(signer, revoker))

			// Загрузка файлов — отдельно: медленный мобильный интернет, свой таймаут 60 с.
			r.With(timeout(60*time.Second), auth.RequireRole(auth.RoleClient, auth.RoleStore, auth.RoleAdmin)).
				Post("/uploads", mediaH.Upload)

			r.Group(func(r chi.Router) {
				r.Use(timeout(15 * time.Second))

				r.Route("/auth", func(r chi.Router) {
					authH.Routes(r)
					// какие способы входа показывать клиенту (сайт и приложение)
					r.Get("/methods", authMethods(d.Cfg, telegramLogin))
					if d.Cfg.SMSAuthEnabled {
						authH.SMSRoutes(r)
					}
					r.Route("/password", authH.PasswordRoutes(d.Cfg.SMSAuthEnabled))
					if d.Cfg.EmailAuthEnabled {
						r.Route("/email", authH.EmailRoutes)
					}
					if authSvc.Google != nil {
						r.Route("/google", authH.GoogleRoutes)
					}
					if telegramLogin {
						r.Route("/telegram", authH.TelegramRoutes)
					}
				})
				productsH.PublicRoutes(r)
				reviewsH.PublicRoutes(r)
				storesH.PublicRoutes(r)
				catalogH.PublicRoutes(r)
				paymentsH.PublicRoutes(r)
				if d.Cfg.TryOnEnabled() {
					tryonH.PublicRoutes(r)
				} else {
					r.Post("/fitting/adds", tryon.Disabled)
				}

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
					// Отзывы и оценки: только после доставки заказа с товаром.
					r.With(auth.RequireRole(auth.RoleClient)).Route("/reviews", reviewsH.ClientRoutes)
					// Покупка токенов: создание платежа и его статус.
					r.With(auth.RequireRole(auth.RoleClient)).Group(paymentsH.ClientRoutes)
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
					storesH.StoreRoutes(r)
					// Окно доставки магазина: только заказы своего магазина.
					r.Route("/orders", ordersH.StoreRoutes)
					r.Route("/telegram", telegramH.StoreRoutes)
					r.Route("/reviews", reviewsH.StoreRoutes)
					// Заявки на рекламу своих товаров.
					r.Route("/promotions", promoH.StoreRoutes)
				})

				r.Route("/admin", func(r chi.Router) {
					r.Use(auth.RequireRole(auth.RoleAdmin))
					adminH.Routes(r)
					catalogH.AdminRoutes(r)
					r.Route("/orders", ordersH.AdminRoutes)
					paymentsH.AdminRoutes(r)
					r.Route("/tryon", tryonH.AdminRoutes)
					r.Route("/reviews", reviewsH.AdminRoutes)
					r.Route("/promotions", promoH.AdminRoutes)
				})
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

// InternalTokenHeader — заголовок с общим секретом сайта и API (config.InternalAPIToken).
const InternalTokenHeader = "X-Internal-Token"

func internalRequest(token string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		got := r.Header.Get(InternalTokenHeader)
		return token != "" && got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
	}
}

// compressExcept сжимает JSON-ответы (gzip, если клиент его принимает), кроме запросов,
// для которых skip вернул true.
func compressExcept(skip func(*http.Request) bool) func(http.Handler) http.Handler {
	compress := middleware.Compress(5, "application/json")
	return func(next http.Handler) http.Handler {
		compressed := compress(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skip(r) {
				next.ServeHTTP(w, r)
				return
			}
			compressed.ServeHTTP(w, r)
		})
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

// authMethods: GET /v1/auth/methods — способы входа клиента. Google client_id и
// имя бота публичные (их всё равно видно на странице входа).
func authMethods(c config.Config, telegram bool) http.HandlerFunc {
	body := map[string]any{"email": c.EmailAuthEnabled, "sms": c.SMSAuthEnabled, "google": nil, "telegram": nil}
	if len(c.GoogleClientIDs) > 0 {
		body["google"] = map[string]any{"client_id": c.GoogleClientIDs[0]}
	}
	if telegram {
		body["telegram"] = map[string]any{"bot_username": c.TelegramBotUsername}
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=300")
		httpx.JSON(w, http.StatusOK, body)
	}
}
