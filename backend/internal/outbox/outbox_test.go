package outbox

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/boutique-market/backend/internal/testutil"
)

// slowTelegram отвечает медленно и считает, сколько раз ушло каждое сообщение.
type slowTelegram struct {
	mu    sync.Mutex
	delay time.Duration
	sent  map[string]int
}

func (s *slowTelegram) Send(ctx context.Context, _ int64, m Message) error {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	s.sent[m.Text]++
	s.mu.Unlock()
	return nil
}

// Медленный провайдер + несколько инстансов API: пачка не укладывается в аренду,
// но каждое сообщение всё равно уходит ровно один раз.
func TestWorker_SlowProviderNoDuplicatesAcrossInstances(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	oldLease, oldTimeout := leaseTTL, sendTimeout
	leaseTTL, sendTimeout = 2*time.Second, 300*time.Millisecond
	t.Cleanup(func() { leaseTTL, sendTimeout = oldLease, oldTimeout })

	const total = 12
	for i := 0; i < total; i++ {
		if err := ToTelegramChat(ctx, pool, 42, Message{Text: string(rune('a' + i))}); err != nil {
			t.Fatal(err)
		}
	}
	tg := &slowTelegram{delay: 200 * time.Millisecond, sent: map[string]int{}}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	var wg sync.WaitGroup
	deadline := time.Now().Add(20 * time.Second)
	for w := 0; w < 3; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wk := &Worker{DB: pool, Log: log, Telegram: tg}
			for time.Now().Before(deadline) {
				var left int
				if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbound_messages WHERE sent_at IS NULL`).Scan(&left); err != nil || left == 0 {
					return
				}
				if _, err := wk.RunOnce(ctx); err != nil {
					t.Error(err)
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
		}()
	}
	wg.Wait()
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if len(tg.sent) != total {
		t.Fatalf("sent %d distinct messages, want %d", len(tg.sent), total)
	}
	for k, n := range tg.sent {
		if n != 1 {
			t.Errorf("message %q sent %d times", k, n)
		}
	}
}
