// Command api — HTTP API маркетплейса.
//
//	api                      запустить сервер (по умолчанию)
//	api migrate up|down|status
//	api create-admin <login> (пароль из ADMIN_PASSWORD)
//	api worker               только воркер AI-примерки (prod: отдельный процесс/контейнер)
//	api demo-data remove     удалить тестовые магазины, загруженные прежней версией
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/boutique-market/backend/internal/admin"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/demoseed"
	"github.com/boutique-market/backend/internal/janitor"
	"github.com/boutique-market/backend/internal/mail"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/notify"
	"github.com/boutique-market/backend/internal/orders"
	"github.com/boutique-market/backend/internal/outbox"
	"github.com/boutique-market/backend/internal/payments"
	"github.com/boutique-market/backend/internal/platform/cache"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/server"
	"github.com/boutique-market/backend/internal/sms"
	"github.com/boutique-market/backend/internal/telegram"
	"github.com/boutique-market/backend/internal/tryon"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	if err := run(log, os.Args[1:]); err != nil {
		log.Error("fatal", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run(log *slog.Logger, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// «api check-config»: только проверка настроек, без базы — ai-store вызывает её
	// перед перезапуском, чтобы неверный .env не останавливал работающий магазин.
	if len(args) > 0 && args[0] == "check-config" {
		fmt.Println("config ok")
		return nil
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	if len(args) > 0 {
		switch args[0] {
		case "migrate":
			// «api migrate» без аргументов = up; срез args[2:] при одном аргументе вызвал бы panic
			cmd, rest := "up", []string(nil)
			if len(args) > 1 {
				cmd, rest = args[1], args[2:]
			}
			return db.Migrate(ctx, pool, cmd, rest...)
		case "create-admin":
			if len(args) < 2 || os.Getenv("ADMIN_PASSWORD") == "" {
				return errors.New("usage: ADMIN_PASSWORD=... api create-admin <login>")
			}
			id, err := admin.CreateAdmin(ctx, pool, args[1], os.Getenv("ADMIN_PASSWORD"))
			if err == nil {
				fmt.Println("admin created:", id)
			}
			return err
		case "demo-data":
			return runDemoData(ctx, log, cfg, pool, args[1:])
		case "serve", "worker":
		default:
			return fmt.Errorf("unknown command %q", args[0])
		}
	}

	if cfg.AutoMigrate {
		if err := db.Migrate(ctx, pool, "up"); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	rdb, err := cache.Connect(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()

	store, err := newStorage(cfg)
	if err != nil {
		return err
	}
	provider, err := tryon.NewProvider(cfg.TryOn)
	if err != nil {
		return err
	}
	worker := &tryon.Worker{DB: pool, Store: store, Provider: provider, Log: log,
		Concurrency: cfg.TryOn.Workers, Timeout: cfg.TryOn.JobTimeout(), MaxAttempts: cfg.TryOn.MaxAttempts,
		Notify: func(ctx context.Context, tx pgx.Tx, userID uuid.UUID, kind string, data map[string]any) error {
			return notify.Send(ctx, tx, userID, kind, data)
		}}
	if provider == nil { // TRYON_PROVIDER=off
		if len(args) > 0 && args[0] == "worker" {
			return errors.New("AI try-on is turned off (TRYON_PROVIDER=off); worker is not needed")
		}
	} else if len(args) > 0 && args[0] == "worker" {
		// отдельный процесс: только очередь генераций, метрики на внутреннем порту
		internal := &http.Server{Addr: cfg.InternalAddr, Handler: internalMux(), ReadHeaderTimeout: 5 * time.Second}
		go func() { _ = internal.ListenAndServe() }()
		defer internal.Close()
		log.Info("worker started", slog.String("cfg", cfg.String()))
		worker.Run(ctx)
		return nil
	}
	if cfg.TryOn.InProcess && provider != nil {
		go worker.Run(ctx)
	}
	// config.Load уже запретил SMS_PROVIDER=log в prod (когда SMS вообще нужны)
	// и MAIL_PROVIDER=log при включённом входе по почте.
	var mailSender mail.Sender = mail.LogSender{Log: log}
	if mail.KnownAPIProvider(cfg.MailProvider) {
		mailSender = &mail.HTTPAPI{Provider: cfg.MailProvider, APIKey: cfg.MailAPIKey, From: cfg.MailFrom}
	} else if cfg.MailProvider == "smtp" {
		mailSender = &mail.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Password: cfg.SMTPPassword, From: cfg.MailFrom}
	}
	var smsSender auth.SMSSender = auth.LogSender{Log: log}
	if cfg.SMSProvider == "eskiz" {
		smsSender = &sms.Eskiz{BaseURL: cfg.EskizURL, Email: cfg.EskizEmail, Password: cfg.EskizPassword,
			From: cfg.EskizFrom, Template: cfg.SMSTemplate}
	}
	log.Info("providers", slog.String("sms", cfg.SMSProvider), slog.String("mail", cfg.MailProvider), slog.Bool("click", cfg.ClickEnabled()),
		slog.Bool("payme", cfg.PaymeEnabled()), slog.Bool("payme_test", cfg.PaymeTest), slog.String("tryon", cfg.TryOn.Provider))

	// Telegram-бот магазинов и SMS клиентам о заказах — через очередь outbound_messages.
	var bot *telegram.Bot
	if cfg.TelegramBotToken != "" {
		bot = &telegram.Bot{Token: cfg.TelegramBotToken}
		if cfg.TelegramWebhookURL != "" {
			if err := bot.SetWebhook(ctx, cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret); err != nil {
				log.Error("telegram setWebhook failed", slog.String("err", err.Error()))
			}
		}
	}
	ob := &outbox.Worker{DB: pool, Log: log,
		OnPermanent: func(ctx context.Context, channel, recipient string) {
			if id, err := strconv.ParseInt(recipient, 10, 64); err == nil && channel == outbox.ChannelTelegram {
				_ = telegram.UnlinkChat(ctx, pool, id) // бот заблокирован — не шлём больше в этот чат
			}
		}}
	if bot != nil {
		ob.Telegram = bot
	}
	if cfg.OrderSMS {
		if s, ok := smsSender.(outbox.SMSSender); ok {
			ob.SMS = s
		}
	}
	go ob.Run(ctx, 5*time.Second)
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, err := orders.RemindPending(ctx, pool, cfg.SiteURL, cfg.OrderRemindAfter, time.Now()); err != nil && ctx.Err() == nil {
					log.Error("order reminders", slog.String("err", err.Error()))
				}
			}
		}
	}()

	// уменьшенные копии фото товаров для витрины (и для уже загруженных фото)
	go (&products.Variants{DB: pool, Store: store, Log: log}).Run(ctx, 15*time.Second)
	views := &products.ViewCounter{Redis: rdb, DB: pool, Log: log}
	go views.Run(ctx, 30*time.Second)
	fitting := &tryon.FittingCounter{Redis: rdb, DB: pool, Log: log,
		// общий счётчик и дневная статистика магазина
		Apply: func(ctx context.Context, ids []uuid.UUID, counts []int64) error {
			return products.RecordFittingAdds(ctx, pool, ids, counts)
		}}
	go fitting.Run(ctx, 30*time.Second)
	ret := server.RetentionFrom(cfg)
	tryonJob := func(name string, fn func(context.Context, *pgxpool.Pool, media.Storage, tryon.Retention, time.Time) (int64, error)) janitor.Job {
		return janitor.Job{Name: name, Run: func(ctx context.Context) (int64, error) { return fn(ctx, pool, store, ret, time.Now()) }}
	}
	go (&janitor.Janitor{DB: pool, Log: log, Jobs: []janitor.Job{{
		Name: "delivery_photos",
		Run: func(ctx context.Context) (int64, error) {
			return orders.PurgeDeliveryPhotos(ctx, pool, store, orders.PhotoRetention, time.Now())
		},
	}, {
		Name: "orphan_delivery_photos",
		Run: func(ctx context.Context) (int64, error) {
			return orders.PurgeOrphanDeliveryPhotos(ctx, pool, store, time.Now())
		},
	}, {
		// брошенные неоплаченные платежи (Payme сам отменяет свои транзакции через 12 ч)
		Name: "expire_pending_payments",
		Run: func(ctx context.Context) (int64, error) {
			return payments.ExpirePending(ctx, pool, 24*time.Hour)
		},
	}, tryonJob("tryon_photos", tryon.PurgePhotos), tryonJob("tryon_results", tryon.PurgeResults),
		tryonJob("tryon_saved_photos", tryon.PurgeSavedPhotos), tryonJob("tryon_orphans", tryon.PurgeOrphans),
	}}).Run(ctx, time.Hour)

	api := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.New(server.Deps{Cfg: cfg, Log: log, DB: pool, Redis: rdb, Storage: store, SMS: smsSender, Mail: mailSender, Views: views, Fitting: fitting, Telegram: bot}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      75 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	internal := &http.Server{Addr: cfg.InternalAddr, Handler: internalMux(), ReadHeaderTimeout: 5 * time.Second}

	errc := make(chan error, 2)
	go func() { errc <- api.ListenAndServe() }()
	go func() { errc <- internal.ListenAndServe() }()
	log.Info("api started", slog.String("cfg", cfg.String()))

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = internal.Shutdown(sctx)
	return api.Shutdown(sctx)
}

// internalMux — метрики и профилировщик. Слушает только внутренний адрес.
func internalMux() http.Handler {
	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.Handler())
	m.HandleFunc("/debug/pprof/", pprof.Index)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	return m
}

// runDemoData — «api demo-data remove» удаляет тестовые магазины, если они были
// загружены прежней версией программы. В prod — только с флагом --allow-prod.
func runDemoData(ctx context.Context, log *slog.Logger, cfg config.Config, pool *pgxpool.Pool, args []string) error {
	remove, allowProd := false, false
	for _, a := range args {
		switch a {
		case "remove":
			remove = true
		case "--allow-prod":
			allowProd = true
		default:
			return errors.New("usage: api demo-data remove [--allow-prod]")
		}
	}
	if !remove {
		return errors.New("test data is no longer bundled; only «api demo-data remove [--allow-prod]» is available")
	}
	if cfg.IsProd() && !allowProd {
		return errors.New("APP_ENV=prod: add --allow-prod to remove the demo stores from production")
	}
	store, err := newStorage(cfg)
	if err != nil {
		return err
	}
	res, err := demoseed.Remove(ctx, pool, store, log)
	if err != nil {
		return err
	}
	fmt.Printf("demo stores deleted: %d\n", res.StoresDeleted)
	if len(res.StoresArchived) > 0 {
		fmt.Printf("stores with orders were archived instead of deleted: %v\n", res.StoresArchived)
	}
	return nil
}

// newStorage: основное хранилище + хранилище биометрии в Узбекистане. Без
// BIOMETRIC_S3_ENDPOINT (только dev/test) биометрия лежит в основном бакете.
func newStorage(cfg config.Config) (media.Storage, error) {
	main, err := media.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL)
	if err != nil {
		return nil, err
	}
	if err := main.WithPublicEndpoint(cfg.S3PublicEndpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL); err != nil {
		return nil, err
	}
	if cfg.BiometricS3Endpoint == "" {
		return media.Routed{Default: main, Biometric: main}, nil
	}
	bio, err := media.NewS3(cfg.BiometricS3Endpoint, cfg.BiometricS3Region, cfg.BiometricS3Bucket,
		cfg.BiometricS3AccessKey, cfg.BiometricS3SecretKey, cfg.BiometricS3UseSSL)
	if err != nil {
		return nil, err
	}
	return media.Routed{Default: main, Biometric: bio}, nil
}
