// Package demoseed удаляет тестовые магазины, которые раньше создавала команда
// «api demo-data» («api demo-data remove»). Сами тестовые данные и фото убраны
// из программы перед запуском; удаление оставлено, чтобы очистить базы, где
// они уже были загружены.
package demoseed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
)

// Slugs — адреса тестовых магазинов, которые создавала прежняя версия demo-data.
var Slugs = []string{"demo-denim-lab", "demo-atlas-moda", "demo-street-code", "demo-gentleman"}

type RemoveResult struct {
	StoresDeleted  int
	StoresArchived []string // у магазина есть заказы: удалить нельзя, магазин архивирован
}

// Remove удаляет тестовые магазины из Slugs вместе с товарами, учётными записями
// и файлами. Магазины с заказами (заказы — учётные документы) не удаляются, а
// архивируются: витрина их больше не показывает.
func Remove(ctx context.Context, pool *pgxpool.Pool, store media.Storage, log *slog.Logger) (RemoveResult, error) {
	var res RemoveResult
	if log == nil {
		log = slog.Default()
	}
	for _, slug := range Slugs {
		var keys []string
		var deleted, archived bool
		err := db.Tx(ctx, pool, func(tx pgx.Tx) error {
			var storeID uuid.UUID
			err := tx.QueryRow(ctx, `SELECT id FROM stores WHERE slug = $1 FOR UPDATE`, slug).Scan(&storeID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			var hasOrders bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM orders WHERE store_id = $1)`, storeID).Scan(&hasOrders); err != nil {
				return err
			}
			if hasOrders {
				archived = true
				if _, err := tx.Exec(ctx, `UPDATE stores SET status = 'archived' WHERE id = $1`, storeID); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `UPDATE products SET status = 'unpublished' WHERE store_id = $1 AND status = 'published'`, storeID)
				return err
			}
			rows, err := tx.Query(ctx, `
				SELECT i.key FROM product_images i JOIN products p ON p.id = i.product_id WHERE p.store_id = $1
				UNION ALL SELECT cover_key FROM stores WHERE id = $1 AND cover_key IS NOT NULL
				UNION ALL SELECT logo_key FROM stores WHERE id = $1 AND logo_key IS NOT NULL`, storeID)
			if err != nil {
				return err
			}
			if keys, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
				return err
			}
			for _, q := range []string{
				`DELETE FROM products WHERE store_id = $1`,
				`DELETE FROM users WHERE store_id = $1`,
				`DELETE FROM stores WHERE id = $1`,
			} {
				if _, err := tx.Exec(ctx, q, storeID); err != nil {
					return err
				}
			}
			deleted = true
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("store %s: %w", slug, err)
		}
		if archived {
			res.StoresArchived = append(res.StoresArchived, slug)
		}
		if !deleted {
			continue
		}
		res.StoresDeleted++
		// файлы — после коммита: при ошибке хранилища в БД не останется ссылок на удалённое
		for _, k := range keys {
			for _, key := range append([]string{k}, variantKeys(k)...) {
				if err := store.Delete(ctx, key); err != nil {
					log.Warn("demo file delete failed", slog.String("key", key), slog.String("err", err.Error()))
				}
			}
		}
	}
	return res, nil
}

func variantKeys(key string) []string {
	out := make([]string, 0, len(media.ProductVariants))
	for _, side := range media.ProductVariants {
		out = append(out, media.VariantKey(key, side))
	}
	return out
}
