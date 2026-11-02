package products

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// ViewCounter считает просмотры в Redis (HINCRBY, без блокировок строк в PostgreSQL)
// и периодически сбрасывает их в products.views_count одним батч-запросом.
type ViewCounter struct {
	Redis *redis.Client
	DB    *pgxpool.Pool
	Log   *slog.Logger
}

const viewsKey = "views:pending"

func (v *ViewCounter) Hit(ctx context.Context, id uuid.UUID) {
	if v == nil {
		return
	}
	if err := v.Redis.HIncrBy(ctx, viewsKey, id.String(), 1).Err(); err != nil {
		v.Log.Warn("view counter unavailable", slog.String("err", err.Error()))
	}
}

// Run сбрасывает счётчики каждые interval до отмены ctx (и последний раз при остановке).
func (v *ViewCounter) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = v.Flush(fctx)
			cancel()
			return
		case <-t.C:
			if err := v.Flush(ctx); err != nil {
				v.Log.Error("views flush failed", slog.String("err", err.Error()))
			}
		}
	}
}

// Flush атомарно забирает накопленное (RENAME) и пишет в БД. При ошибке БД
// счётчики возвращаются в Redis, чтобы не потерять просмотры.
func (v *ViewCounter) Flush(ctx context.Context) error {
	tmp := viewsKey + ":flush:" + uuid.NewString()
	if err := v.Redis.Rename(ctx, viewsKey, tmp).Err(); err != nil {
		if err.Error() == "ERR no such key" {
			return nil
		}
		return err
	}
	m, err := v.Redis.HGetAll(ctx, tmp).Result()
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(m))
	counts := make([]int64, 0, len(m))
	for k, s := range m {
		id, err1 := uuid.Parse(k)
		n, err2 := strconv.ParseInt(s, 10, 64)
		if err1 == nil && err2 == nil && n > 0 {
			ids, counts = append(ids, id), append(counts, n)
		}
	}
	_, err = v.DB.Exec(ctx, `
		UPDATE products p SET views_count = p.views_count + x.n
		FROM unnest($1::uuid[], $2::bigint[]) AS x(id, n)
		WHERE p.id = x.id`, ids, counts)
	if err != nil {
		pipe := v.Redis.TxPipeline()
		for i, id := range ids {
			pipe.HIncrBy(ctx, viewsKey, id.String(), counts[i])
		}
		_, _ = pipe.Exec(ctx)
		return err
	}
	return v.Redis.Del(ctx, tmp).Err()
}
