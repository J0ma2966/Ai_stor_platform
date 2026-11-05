// Package db: пул соединений PostgreSQL, транзакции и миграции.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/boutique-market/backend/migrations"
)

func Connect(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	// Защита от «зависших» запросов: сервер сам прервёт запрос дольше 10 с.
	if _, ok := cfg.ConnConfig.RuntimeParams["statement_timeout"]; !ok {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	}
	if _, ok := cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"]; !ok {
		cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30000"
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}

// Migrate выполняет goose-команду (up, down, status, ...) по встроенным миграциям.
func Migrate(ctx context.Context, pool *pgxpool.Pool, command string, args ...string) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()
	return migrate(ctx, sqlDB, command, args...)
}

func migrate(ctx context.Context, sqlDB *sql.DB, command string, args ...string) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	// AllowMissing: ветки разрабатываются параллельно и берут разные диапазоны номеров
	// (00010+, 00020+, 00030+), поэтому миграция с меньшим номером может прийти позже.
	return goose.RunWithOptionsContext(ctx, command, sqlDB, ".", args, goose.WithAllowMissing())
}

// Tx выполняет fn в транзакции; при ошибке или панике — откат.
func Tx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IsRetryable — транзакцию отменил сам PostgreSQL из-за конкуренции
// (40P01 deadlock_detected, 40001 serialization_failure); её можно повторить целиком.
func IsRetryable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001")
}

// TxRetry — Tx, который повторяет транзакцию (до 3 попыток), если PostgreSQL отменил
// её из-за deadlock или конфликта сериализации. fn должна быть повторяемой: менять
// только БД (откатывается вместе с транзакцией) и переприсваивать свои результаты.
func TxRetry(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	var err error
	for attempt := 1; ; attempt++ {
		err = Tx(ctx, pool, fn)
		if err == nil || attempt == 3 || !IsRetryable(err) || ctx.Err() != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt*attempt) * 20 * time.Millisecond):
		}
	}
}

// IsUniqueViolation проверяет нарушение уникальности (опционально — по имени ограничения).
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return constraint == "" || pgErr.ConstraintName == constraint
	}
	return false
}

var ErrNotFound = errors.New("not found")

// NotFound превращает pgx.ErrNoRows в ErrNotFound.
func NotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
