// Command api — HTTP API маркетплейса.
//
//	api                      запустить сервер (по умолчанию)
//	api migrate up|down|status
//	api create-admin <login> (пароль из ADMIN_PASSWORD)
//	api worker               только воркер AI-примерки (prod: отдельный процесс/контейнер)
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
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/boutique-market/backend/internal/admin"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/janitor"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/orders"
	"github.com/boutique-market/backend/internal/platform/cache"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/server"
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
	pool, err := db.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	if len(args) > 0 {
		switch args[0] {
		case "migrate":
			cmd := "up"
			if len(args) > 1 {
				cmd = args[1]
			}
			return db.Migrate(ctx, pool, cmd, args[2:]...)
		case "create-admin":
			if len(args) < 2 || os.Getenv("ADMIN_PASSWORD") == "" {
				return errors.New("usage: ADMIN_PASSWORD=... api create-admin <login>")
			}
			id, err := admin.CreateAdmin(ctx, pool, args[1], os.Getenv("ADMIN_PASSWORD"))
			if err == nil {
				fmt.Println("admin created:", id)
			}
			return err
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
		Concurrency: cfg.TryOn.Workers, Timeout: cfg.TryOn.Timeout, MaxAttempts: cfg.TryOn.MaxAttempts}
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
	var sms auth.SMSSender = auth.LogSender{Log: log}
	if cfg.IsProd() {
		// TODO: подключить SMS-провайдера (Eskiz / Playmobile) — без него prod не стартует.
		return errors.New("SMS provider is not configured for prod")
	}

	views := &products.ViewCounter{Redis: rdb, DB: pool, Log: log}
	go views.Run(ctx, 30*time.Second)
	fitting := &tryon.FittingCounter{Redis: rdb, DB: pool, Log: log}
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
	}, tryonJob("tryon_photos", tryon.PurgePhotos), tryonJob("tryon_results", tryon.PurgeResults),
		tryonJob("tryon_saved_photos", tryon.PurgeSavedPhotos), tryonJob("tryon_orphans", tryon.PurgeOrphans),
	}}).Run(ctx, time.Hour)

	api := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.New(server.Deps{Cfg: cfg, Log: log, DB: pool, Redis: rdb, Storage: store, SMS: sms, Views: views, Fitting: fitting}),
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

// newStorage: основное хранилище + хранилище биометрии в Узбекистане. Без
// BIOMETRIC_S3_ENDPOINT (только dev/test) биометрия лежит в основном бакете.
func newStorage(cfg config.Config) (media.Storage, error) {
	main, err := media.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL)
	if err != nil {
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
