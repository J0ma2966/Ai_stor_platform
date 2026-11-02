package janitor

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/testutil"
)

func TestJanitorDeletesOnlyOldRows(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	var uid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (role, phone) VALUES ('client', '+998906666666') RETURNING id`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `
		INSERT INTO idempotency_keys (user_id, scope, key, request_hash, created_at) VALUES
		  ($1, 'checkout', 'old-key-0001', '\x00', now() - interval '3 days'),
		  ($1, 'checkout', 'new-key-0001', '\x00', now() - interval '1 hour');
		`, uid)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, family_id, token_hash, expires_at) VALUES
		  ($1, gen_random_uuid(), '\x01', now() - interval '30 days'),
		  ($1, gen_random_uuid(), '\x02', now() + interval '30 days')`, uid)
	if err != nil {
		t.Fatal(err)
	}
	j := &Janitor{DB: pool, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := j.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	var keys, tokens int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_keys`).Scan(&keys)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens`).Scan(&tokens)
	if keys != 1 || tokens != 1 {
		t.Fatalf("keys=%d tokens=%d, want 1 and 1", keys, tokens)
	}
}
