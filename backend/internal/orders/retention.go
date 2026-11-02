package orders

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/media"
)

// PhotoRetention — сколько хранится фото места доставки после завершения заказа
// (доставлен / отклонён / отменён). Решение владельца продукта от 2026-11-02: 30 дней.
const PhotoRetention = 30 * 24 * time.Hour

const purgeBatch = 500

// PurgeDeliveryPhotos удаляет фото мест доставки, срок хранения которых истёк.
// Один ключ может быть у нескольких заказов одного оформления (по магазинам),
// поэтому объект удаляется, только когда истёк срок у всех заказов с этим ключом.
// Порядок: сначала объект в хранилище, потом ссылка в БД — при сбое удаления
// ссылка остаётся и следующий запуск повторит попытку (сирот с ПД не остаётся).
func PurgeDeliveryPhotos(ctx context.Context, db *pgxpool.Pool, store media.Storage, retention time.Duration, now time.Time) (int64, error) {
	rows, err := db.Query(ctx, `
		SELECT d.photo_key
		FROM order_deliveries d JOIN orders o ON o.id = d.order_id
		WHERE d.photo_key IS NOT NULL
		GROUP BY d.photo_key
		HAVING bool_and(o.status IN ('delivered', 'rejected', 'cancelled') AND o.updated_at < $1)
		LIMIT $2`, now.Add(-retention), purgeBatch)
	if err != nil {
		return 0, err
	}
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return 0, err
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	var n int64
	for _, k := range keys {
		if err := store.Delete(ctx, k); err != nil {
			return n, err
		}
		tag, err := db.Exec(ctx, `UPDATE order_deliveries SET photo_key = NULL WHERE photo_key = $1`, k)
		if err != nil {
			return n, err
		}
		n += tag.RowsAffected()
	}
	return n, nil
}
