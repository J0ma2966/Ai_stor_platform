package products

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Размеры товара (S, M, 42, 38/40…) с остатком по каждому. Когда у товара есть
// размеры, products.in_stock пересчитывает триггер БД (00014): товар «в наличии»,
// если остаток хотя бы одного размера больше нуля.

const (
	MaxSizes    = 30
	MaxSizeLen  = 20
	MaxStockQty = 100000
)

// Size — размер в ответе API. Stock отдаётся только кабинету магазина;
// покупателю — только «есть / нет» и «осталось мало».
type Size struct {
	Size    string `json:"size"`
	Stock   *int   `json:"stock,omitempty"`
	InStock bool   `json:"in_stock"`
	Low     bool   `json:"low,omitempty"` // осталось 1–2 шт.
}

// SizeIn — размер в запросе магазина.
type SizeIn struct {
	Size  string `json:"size"`
	Stock int    `json:"stock"`
}

// NormalizeSize приводит обозначение размера к единому виду: без лишних пробелов,
// латиница в верхнем регистре (s → S, xl → XL).
func NormalizeSize(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, s)
}

// NormalizeSizes проверяет список размеров. Возвращает ошибки по полям.
func NormalizeSizes(in []SizeIn) ([]SizeIn, map[string]string) {
	fields := map[string]string{}
	if len(in) > MaxSizes {
		fields["sizes"] = "max"
		return nil, fields
	}
	out := make([]SizeIn, 0, len(in))
	seen := map[string]bool{}
	for i, s := range in {
		s.Size = NormalizeSize(s.Size)
		switch n := utf8.RuneCountInString(s.Size); {
		case n == 0:
			fields[fmt.Sprintf("sizes[%d].size", i)] = "required"
		case n > MaxSizeLen:
			fields[fmt.Sprintf("sizes[%d].size", i)] = "max"
		case seen[s.Size]:
			fields[fmt.Sprintf("sizes[%d].size", i)] = "duplicate"
		}
		if s.Stock < 0 || s.Stock > MaxStockQty {
			fields[fmt.Sprintf("sizes[%d].stock", i)] = "range"
		}
		seen[s.Size] = true
		out = append(out, s)
	}
	if len(fields) > 0 {
		return nil, fields
	}
	return out, nil
}

// ReplaceSizes заменяет набор размеров товара (порядок сохраняется).
func ReplaceSizes(ctx context.Context, tx pgx.Tx, productID uuid.UUID, sizes []SizeIn) error {
	names := make([]string, len(sizes))
	for i, s := range sizes {
		names[i] = s.Size
	}
	if _, err := tx.Exec(ctx, `DELETE FROM product_sizes WHERE product_id = $1 AND NOT (size = ANY($2))`, productID, names); err != nil {
		return err
	}
	for i, s := range sizes {
		if _, err := tx.Exec(ctx, `INSERT INTO product_sizes (product_id, size, stock, sort) VALUES ($1, $2, $3, $4)
			ON CONFLICT (product_id, size) DO UPDATE SET stock = EXCLUDED.stock, sort = EXCLUDED.sort`,
			productID, s.Size, s.Stock, i); err != nil {
			return err
		}
	}
	return nil
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// LoadSizes читает размеры товара. withStock — показать точный остаток (кабинет магазина).
func LoadSizes(ctx context.Context, q querier, productID uuid.UUID, withStock bool) ([]Size, error) {
	rows, err := q.Query(ctx, `SELECT size, stock FROM product_sizes WHERE product_id = $1 ORDER BY sort, size`, productID)
	if err != nil {
		return nil, err
	}
	out := []Size{}
	for rows.Next() {
		var s Size
		var stock int
		if err := rows.Scan(&s.Size, &stock); err != nil {
			rows.Close()
			return nil, err
		}
		s.InStock = stock > 0
		if withStock {
			v := stock
			s.Stock = &v
		} else {
			s.Low = stock > 0 && stock <= 2
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
