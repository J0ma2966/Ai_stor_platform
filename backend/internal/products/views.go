package products

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/boutique-market/backend/internal/platform/db"
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
	// Общий счётчик и дневная статистика (для отчёта магазина за период) — одним запросом.
	err = applyCounts(ctx, v.DB, flushViewsSQL, ids, counts)
	if err != nil {
		pipe := v.Redis.TxPipeline()
		for i, id := range ids {
			pipe.HIncrBy(ctx, viewsKey, id.String(), counts[i])
		}
		pipe.Del(ctx, tmp) // иначе временный ключ остаётся в Redis навсегда
		_, _ = pipe.Exec(ctx)
		return err
	}
	return v.Redis.Del(ctx, tmp).Err()
}

// StatsDay — «сегодня» для дневной статистики, по времени Ташкента.
const StatsDay = `(now() AT TIME ZONE 'Asia/Tashkent')::date`

// RecordFittingAdd — товар добавлен в примерочную: общий счётчик и дневная статистика.
// Вызывает модуль примерочной (дедупликацию по клиенту делает он).
func RecordFittingAdd(ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, productID uuid.UUID) error {
	_, err := q.Exec(ctx, fittingAddSQL, productID)
	return err
}

// Запросы собираются из констант на этапе компиляции.
const (
	flushViewsSQL = `
		WITH u AS (
			UPDATE products p SET views_count = p.views_count + x.n
			FROM unnest($1::uuid[], $2::bigint[]) AS x(id, n)
			WHERE p.id = x.id RETURNING p.id, x.n
		)
		INSERT INTO product_stats_daily (product_id, day, views)
		SELECT id, ` + StatsDay + `, n FROM u
		ON CONFLICT (product_id, day) DO UPDATE SET views = product_stats_daily.views + EXCLUDED.views`
	fittingAddSQL = `
		WITH u AS (UPDATE products SET fitting_adds = fitting_adds + 1 WHERE id = $1 RETURNING id)
		INSERT INTO product_stats_daily (product_id, day, fitting_adds)
		SELECT id, ` + StatsDay + `, 1 FROM u
		ON CONFLICT (product_id, day) DO UPDATE SET fitting_adds = product_stats_daily.fitting_adds + 1`
	fittingAddsSQL = `
		WITH u AS (
			UPDATE products p SET fitting_adds = p.fitting_adds + x.n
			FROM unnest($1::uuid[], $2::bigint[]) AS x(id, n)
			WHERE p.id = x.id RETURNING p.id, x.n
		)
		INSERT INTO product_stats_daily (product_id, day, fitting_adds)
		SELECT id, ` + StatsDay + `, n FROM u
		ON CONFLICT (product_id, day) DO UPDATE SET fitting_adds = product_stats_daily.fitting_adds + EXCLUDED.fitting_adds`
)

// RecordFittingAdds — батчевая версия RecordFittingAdd: счётчик примерочной копится
// в Redis и сбрасывается сюда пачкой (ids[i] получил counts[i] добавлений).
func RecordFittingAdds(ctx context.Context, pool *pgxpool.Pool, ids []uuid.UUID, counts []int64) error {
	if len(ids) != len(counts) {
		return errors.New("RecordFittingAdds: ids and counts length mismatch")
	}
	return applyCounts(ctx, pool, fittingAddsSQL, ids, counts)
}

// applyCounts пишет пачку счётчиков (stmt с параметрами $1 ids, $2 counts).
// Строки products блокируются заранее по возрастанию id — в том же порядке, что
// при оформлении заказа. Иначе UPDATE … FROM unnest берёт блокировки в случайном
// порядке (ids приходят из map), и сбросы с трёх инстансов API и покупки тех же
// товаров взаимно блокируются (deadlock).
func applyCounts(ctx context.Context, pool *pgxpool.Pool, stmt string, ids []uuid.UUID, counts []int64) error {
	if len(ids) == 0 {
		return nil
	}
	idx := make([]int, len(ids))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(a, b int) bool { return bytes.Compare(ids[idx[a]][:], ids[idx[b]][:]) < 0 })
	sIDs, sCounts := make([]uuid.UUID, len(ids)), make([]int64, len(ids))
	for i, j := range idx {
		sIDs[i], sCounts[i] = ids[j], counts[j]
	}
	return db.TxRetry(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM products WHERE id = ANY($1) ORDER BY id FOR NO KEY UPDATE`, sIDs); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, stmt, sIDs, sCounts)
		return err
	})
}
