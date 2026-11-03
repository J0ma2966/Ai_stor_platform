package tryon

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// FittingCounter копит добавления «В примерочную» в Redis (HINCRBY) и раз в
// интервал пишет их в БД одним батчем — популярный товар не создаёт горячую строку
// в products (как ViewCounter для просмотров).
type FittingCounter struct {
	Redis *redis.Client
	DB    *pgxpool.Pool
	Log   *slog.Logger
	// Apply пишет батч в БД. По умолчанию — products.fitting_adds.
	// TODO(stats): после мержа PR #7 добавить запись в дневную статистику product_stats_daily.
	Apply func(ctx context.Context, ids []uuid.UUID, counts []int64) error
}

const fittingKey = "fitting:pending"

func (c *FittingCounter) Hit(ctx context.Context, id uuid.UUID) error {
	return c.Redis.HIncrBy(ctx, fittingKey, id.String(), 1).Err()
}

func (c *FittingCounter) apply(ctx context.Context, ids []uuid.UUID, counts []int64) error {
	if c.Apply != nil {
		return c.Apply(ctx, ids, counts)
	}
	_, err := c.DB.Exec(ctx, `
		UPDATE products p SET fitting_adds = p.fitting_adds + x.n
		FROM unnest($1::uuid[], $2::bigint[]) AS x(id, n)
		WHERE p.id = x.id`, ids, counts)
	return err
}

// Flush атомарно забирает накопленное (RENAME) и пишет в БД; при ошибке БД
// возвращает счётчики в Redis.
func (c *FittingCounter) Flush(ctx context.Context) error {
	tmp := fittingKey + ":flush:" + uuid.NewString()
	if err := c.Redis.Rename(ctx, fittingKey, tmp).Err(); err != nil {
		if err.Error() == "ERR no such key" {
			return nil
		}
		return err
	}
	m, err := c.Redis.HGetAll(ctx, tmp).Result()
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
	if err := c.apply(ctx, ids, counts); err != nil {
		pipe := c.Redis.TxPipeline()
		for i, id := range ids {
			pipe.HIncrBy(ctx, fittingKey, id.String(), counts[i])
		}
		pipe.Del(ctx, tmp)
		_, _ = pipe.Exec(ctx)
		return err
	}
	return c.Redis.Del(ctx, tmp).Err()
}

func (c *FittingCounter) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = c.Flush(fctx)
			cancel()
			return
		case <-t.C:
			if err := c.Flush(ctx); err != nil {
				c.Log.Error("fitting counter flush failed", slog.String("err", err.Error()))
			}
		}
	}
}
