package products

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/httpx"
)

// «Двойка» (products.is_set, миграция 00032) — парный комплект: верх и низ на одном
// фото. Флаг допустим только у категорий зоны top: в примерочной двойка занимает
// зоны «верх» и «низ» (низ к ней не выбирается). При переносе товара в категорию
// другой зоны флаг сбрасывается (см. storeUpdate), поэтому читатели доверяют is_set.

// ZoneSetHolder — зона категории, в которой может быть двойка.
const ZoneSetHolder = "top"

var errSetTopOnly = &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed",
	Message: "a two-piece set must be in a top category", Fields: map[string]string{"is_set": "top_only"}}

// checkSetCategory — категория существует и относится к зоне top.
func checkSetCategory(ctx context.Context, q pgx.Tx, categoryID uuid.UUID) error {
	var zone string
	err := q.QueryRow(ctx, `SELECT zone::text FROM categories WHERE id = $1`, categoryID).Scan(&zone)
	if errors.Is(err, pgx.ErrNoRows) {
		return &httpx.Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "unknown category",
			Fields: map[string]string{"category_id": "unknown"}}
	}
	if err != nil {
		return err
	}
	if zone != ZoneSetHolder {
		return errSetTopOnly
	}
	return nil
}
