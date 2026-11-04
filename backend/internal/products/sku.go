package products

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// autoSKUPrefix — префикс автоматически выданных артикулов. Формат "A-000123"
// читается в Excel как текст (без потери ведущих нулей) и подходит для имён
// фото при массовой загрузке ("A-000123_front.jpg").
const autoSKUPrefix = "A-"

// maxSKUAttempts ограничивает пропуск номеров, уже занятых вручную введёнными артикулами.
const maxSKUAttempts = 1000

func formatAutoSKU(n int64) string { return fmt.Sprintf("%s%06d", autoSKUPrefix, n) }

// NextSKU выдаёт следующий свободный артикул магазина внутри транзакции tx.
// Артикул уникален в пределах магазина (как и ограничение products_store_id_sku_key).
// Строка счётчика блокируется до конца транзакции, поэтому параллельные
// добавления товаров одного магазина получают разные номера; номер,
// занятый вручную введённым артикулом, пропускается.
func NextSKU(ctx context.Context, tx pgx.Tx, storeID uuid.UUID) (string, error) {
	for range maxSKUAttempts {
		var n int64
		if err := tx.QueryRow(ctx, `
			INSERT INTO store_sku_counters (store_id, last) VALUES ($1, 1)
			ON CONFLICT (store_id) DO UPDATE SET last = store_sku_counters.last + 1
			RETURNING last`, storeID).Scan(&n); err != nil {
			return "", err
		}
		sku := formatAutoSKU(n)
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM products WHERE store_id = $1 AND lower(sku) = lower($2))`,
			storeID, sku).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return sku, nil
		}
	}
	return "", fmt.Errorf("products: no free auto SKU for store %s after %d attempts", storeID, maxSKUAttempts)
}

// skuOrAuto возвращает введённый артикул без пробелов по краям или, если он пуст, новый автоартикул.
func skuOrAuto(ctx context.Context, tx pgx.Tx, storeID uuid.UUID, sku string) (string, error) {
	if s := strings.TrimSpace(sku); s != "" {
		return s, nil
	}
	return NextSKU(ctx, tx, storeID)
}
