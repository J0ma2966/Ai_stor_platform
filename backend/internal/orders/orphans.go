package orders

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/media"
)

// OrphanGrace — сколько ждём, прежде чем считать фото доставки «брошенным»:
// клиент мог загрузить фото и ещё не нажать «Оформить».
const OrphanGrace = 24 * time.Hour

// PurgeOrphanDeliveryPhotos удаляет фото места доставки, которые были загружены,
// но так и не попали ни в один заказ (клиент бросил оформление).
func PurgeOrphanDeliveryPhotos(ctx context.Context, db *pgxpool.Pool, store media.Storage, now time.Time) (int64, error) {
	var candidates []string
	err := store.List(ctx, "priv/u/", now.Add(-OrphanGrace), func(key string) error {
		if strings.Contains(key, "/"+string(media.PurposeDeliveryPhoto)+"/") {
			candidates = append(candidates, key)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	var n int64
	for len(candidates) > 0 {
		batch := candidates[:min(len(candidates), 500)]
		candidates = candidates[len(batch):]
		rows, err := db.Query(ctx, `
			SELECT k FROM unnest($1::text[]) AS k
			WHERE NOT EXISTS (SELECT 1 FROM order_deliveries d WHERE d.photo_key = k)`, batch)
		if err != nil {
			return n, err
		}
		var orphans []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				rows.Close()
				return n, err
			}
			orphans = append(orphans, k)
		}
		if err := rows.Err(); err != nil {
			return n, err
		}
		for _, k := range orphans {
			if err := store.Delete(ctx, k); err != nil {
				return n, err
			}
			n++
		}
	}
	return n, nil
}
