package products

import (
	"net/http"

	"github.com/boutique-market/backend/internal/httpx"
)

// MaxPromoted — сколько рекламных товаров отдаёт карусель на главной.
const MaxPromoted = 20

// promoted: GET /v1/promotions — товары с идущей рекламой для карусели «Реклама»
// на главной (сайт и приложение). Заявки и решения — пакет promotions.
// Показывается реклама со status = 'approved' и starts_at <= now() < ends_at;
// снятый с витрины товар или заблокированный магазин выпадают сами.
// Порядок: недавно запущенные первыми, внутри секунды — стабильно по id.
func (h *Handlers) promoted(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT p.id, p.store_id, s.name, s.slug, p.category_id, c.zone::text, p.title, p.brand, p.price, p.in_stock, p.is_set, p.created_at,
		       p.rating_avg::float8, p.rating_count, `+imagesSQL+`,
		       EXISTS (SELECT 1 FROM product_sizes ps WHERE ps.product_id = p.id)
		FROM product_promotions pr
		JOIN products p ON p.id = pr.product_id AND p.status = 'published'
		JOIN stores s ON s.id = p.store_id AND s.status = 'active'
		JOIN categories c ON c.id = p.category_id
		WHERE pr.status = 'approved' AND pr.starts_at <= now() AND now() < pr.ends_at
		ORDER BY pr.starts_at DESC, pr.id
		LIMIT $1`, MaxPromoted)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.StoreID, &p.StoreName, &p.StoreSlug, &p.CategoryID, &p.Zone, &p.Title, &p.Brand, &p.Price, &p.InStock, &p.IsSet, &p.CreatedAt,
			&p.RatingAvg, &p.RatingCount, &p.imageKeys, &p.HasSizes); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		p.Images, p.Thumbs = h.urls(p.imageKeys)
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Короткий кэш: остановленная администратором реклама пропадает в течение минуты.
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpx.List(w, items, "")
}
