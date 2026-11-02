// Package testutil — изолированная БД на каждый тестовый пакет.
// Нужна переменная TEST_DATABASE_URL (например postgres://postgres@localhost:5432/postgres?sslmode=disable);
// без неё интеграционные тесты пропускаются.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/platform/db"
)

// DB создаёт новую базу test_<random>, накатывает миграции и удаляет её после теста.
func DB(t testing.TB) *pgxpool.Pool {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect admin db: %v", err)
	}
	var b [6]byte
	_, _ = rand.Read(b[:])
	name := "test_" + hex.EncodeToString(b[:])
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create db: %v", err)
	}
	u, _ := url.Parse(base)
	u.Path = "/" + name
	pool, err := db.Connect(ctx, u.String(), 10)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if err := db.Migrate(ctx, pool, "up"); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = admin.Close(ctx)
	})
	return pool
}
