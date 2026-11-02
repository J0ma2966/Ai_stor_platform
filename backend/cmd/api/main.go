// Command api — HTTP API маркетплейса.
//
//	api                      запустить сервер (по умолчанию)
//	api migrate up|down|status
//	api create-admin <login> (пароль из ADMIN_PASSWORD)
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

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/boutique-market/backend/internal/admin"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/config"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/cache"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/server"
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
		case "serve":
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

	store, err := media.NewS3(cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3UseSSL)
	if err != nil {
		return err
	}
	var sms auth.SMSSender = auth.LogSender{Log: log}
	if cfg.IsProd() {
		// TODO: подключить SMS-провайдера (Eskiz / Playmobile) — без него prod не стартует.
		return errors.New("SMS provider is not configured for prod")
	}

	views := &products.ViewCounter{Redis: rdb, DB: pool, Log: log}
	go views.Run(ctx, 30*time.Second)

	api := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.New(server.Deps{Cfg: cfg, Log: log, DB: pool, Redis: rdb, Storage: store, SMS: sms, Views: views}),
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
