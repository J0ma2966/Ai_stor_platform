package products

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/media"
)

// Variants строит уменьшенные копии фото товаров (media.ProductVariants) в фоне:
// для новых фото и для загруженных до появления копий. Пока копий нет, витрина
// отдаёт оригинал, так что задержка ничего не ломает.
// На нескольких инстансах API работает один (pg_try_advisory_lock).
type Variants struct {
	DB    *pgxpool.Pool
	Store media.Storage
	Log   *slog.Logger
	Batch int // фото за один проход (0 — 20)
}

const variantsLockID = 774_201_002 // janitor — 774_201_001

// Run обрабатывает очередь каждые interval до отмены ctx. Если за проход
// набралась полная пачка, следующий проход начинается сразу (догоняем бэклог).
func (v *Variants) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		n, err := v.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			v.Log.Error("image variants failed", slog.String("err", err.Error()))
		}
		if err == nil && n >= v.batch() {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (v *Variants) batch() int {
	if v.Batch > 0 {
		return v.Batch
	}
	return 20
}

// RunOnce обрабатывает до Batch фото; возвращает, сколько обработано.
func (v *Variants) RunOnce(ctx context.Context) (int, error) {
	conn, err := v.DB.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, variantsLockID).Scan(&got); err != nil || !got {
		return 0, err
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, variantsLockID) }()

	rows, err := conn.Query(ctx, `SELECT product_id, angle::text, key FROM product_images
		WHERE variants = 'pending' LIMIT $1`, v.batch())
	if err != nil {
		return 0, err
	}
	type item struct {
		productID  uuid.UUID
		angle, key string
	}
	items, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (item, error) {
		var it item
		return it, r.Scan(&it.productID, &it.angle, &it.key)
	})
	if err != nil {
		return 0, err
	}
	for _, it := range items {
		state := "ready"
		if err := v.build(ctx, it.key); err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			if !errors.Is(err, media.ErrCorrupt) && !errors.Is(err, media.ErrTooLarge) && !isNotFound(err) {
				// хранилище недоступно — попробуем в следующий раз
				return 0, err
			}
			v.Log.Warn("image variants skipped", slog.String("key", it.key), slog.String("err", err.Error()))
			state = "failed"
		}
		// key в условии: если фото заменили, пока строились копии, строка снова pending
		if _, err := conn.Exec(ctx, `UPDATE product_images SET variants = $4
			WHERE product_id = $1 AND angle = $2::photo_angle AND key = $3`, it.productID, it.angle, it.key, state); err != nil {
			return 0, err
		}
	}
	return len(items), nil
}

func (v *Variants) build(ctx context.Context, key string) error {
	orig, err := v.Store.Get(ctx, key)
	if err != nil {
		return err
	}
	out, err := media.MakeVariants(orig, media.ProductVariants)
	if err != nil {
		return err
	}
	for side, data := range out {
		if err := v.Store.Put(ctx, media.VariantKey(key, side), data, "image/jpeg"); err != nil {
			return err
		}
	}
	return nil
}

func isNotFound(err error) bool { return errors.Is(err, media.ErrObjectNotFound) }
