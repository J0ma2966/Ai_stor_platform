package stores

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
)

const maxPeriod = 366 * 24 * time.Hour

type TopProduct struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Brand       string    `json:"brand"`
	Status      string    `json:"status"`
	Views       int64     `json:"views"`
	FittingAdds int64     `json:"fitting_adds"`
	Ordered     int64     `json:"ordered"` // штук в заказах за период
}

type DayPoint struct {
	Day         string `json:"day"`
	Views       int64  `json:"views"`
	FittingAdds int64  `json:"fitting_adds"`
}

type Stats struct {
	From              string       `json:"from"`
	To                string       `json:"to"`
	ProductsTotal     int64        `json:"products_total"`
	ProductsPublished int64        `json:"products_published"`
	Views             int64        `json:"views"`
	FittingAdds       int64        `json:"fitting_adds"`
	TryOns            int64        `json:"tryons"` // успешные AI-примерки с товарами магазина
	Orders            int64        `json:"orders"`
	OrdersDelivered   int64        `json:"orders_delivered"`
	Revenue           int64        `json:"revenue"` // сумма доставленных заказов, тийины
	ViewsAllTime      int64        `json:"views_all_time"`
	FittingAllTime    int64        `json:"fitting_adds_all_time"`
	Top               []TopProduct `json:"top_products"`
	Daily             []DayPoint   `json:"daily"`
}

// period читает ?from=YYYY-MM-DD&to=YYYY-MM-DD (включительно, по Ташкенту). По умолчанию — 30 дней.
func period(r *http.Request) (time.Time, time.Time, error) {
	loc := tashkent()
	today := time.Now().In(loc)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
	from, to := today.AddDate(0, 0, -29), today
	var err error
	if v := r.URL.Query().Get("from"); v != "" {
		if from, err = time.ParseInLocation(time.DateOnly, v, loc); err != nil {
			return from, to, httpx.NewError(400, "bad_filter", "from must be YYYY-MM-DD")
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if to, err = time.ParseInLocation(time.DateOnly, v, loc); err != nil {
			return from, to, httpx.NewError(400, "bad_filter", "to must be YYYY-MM-DD")
		}
	}
	if to.Before(from) || to.Sub(from) > maxPeriod {
		return from, to, httpx.NewError(400, "bad_filter", "period must be 1..366 days")
	}
	return from, to, nil
}

func tashkent() *time.Location {
	if l, err := time.LoadLocation("Asia/Tashkent"); err == nil {
		return l
	}
	return time.FixedZone("UZT", 5*3600)
}

// stats — только собственная статистика магазина (ТЗ 5.1). Финансы платформы и
// данные конкурентов недоступны.
func (h *Handlers) stats(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	from, to, err := period(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	end := to.AddDate(0, 0, 1) // полуинтервал [from, end)
	st := Stats{From: from.Format(time.DateOnly), To: to.Format(time.DateOnly), Top: []TopProduct{}, Daily: []DayPoint{}}
	ctx := r.Context()
	err = func() error {
		if err := h.DB.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status = 'published'),
				COALESCE(sum(views_count), 0), COALESCE(sum(fitting_adds), 0)
			FROM products WHERE store_id = $1`, p.StoreID).
			Scan(&st.ProductsTotal, &st.ProductsPublished, &st.ViewsAllTime, &st.FittingAllTime); err != nil {
			return err
		}
		if err := h.DB.QueryRow(ctx, `SELECT COALESCE(sum(d.views), 0), COALESCE(sum(d.fitting_adds), 0)
			FROM product_stats_daily d JOIN products pr ON pr.id = d.product_id
			WHERE pr.store_id = $1 AND d.day >= $2::date AND d.day <= $3::date`, p.StoreID, st.From, st.To).
			Scan(&st.Views, &st.FittingAdds); err != nil {
			return err
		}
		if err := h.DB.QueryRow(ctx, `SELECT count(*) FROM generations g
			WHERE g.status = 'succeeded' AND g.created_at >= $2 AND g.created_at < $3
			  AND g.product_ids && ARRAY(SELECT id FROM products WHERE store_id = $1)`, p.StoreID, from, end).
			Scan(&st.TryOns); err != nil {
			return err
		}
		if err := h.DB.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status = 'delivered'),
				COALESCE(sum(items_total) FILTER (WHERE status = 'delivered'), 0)
			FROM orders WHERE store_id = $1 AND created_at >= $2 AND created_at < $3`, p.StoreID, from, end).
			Scan(&st.Orders, &st.OrdersDelivered, &st.Revenue); err != nil {
			return err
		}
		rows, err := h.DB.Query(ctx, `
			WITH v AS (
				SELECT d.product_id, sum(d.views) AS views, sum(d.fitting_adds) AS adds
				FROM product_stats_daily d JOIN products pr ON pr.id = d.product_id
				WHERE pr.store_id = $1 AND d.day >= $2::date AND d.day <= $3::date GROUP BY d.product_id
			), o AS (
				SELECT oi.product_id, sum(oi.qty) AS qty FROM order_items oi JOIN orders od ON od.id = oi.order_id
				WHERE od.store_id = $1 AND od.created_at >= $4 AND od.created_at < $5 AND od.status <> 'rejected'
				  AND oi.product_id IS NOT NULL GROUP BY oi.product_id
			)
			SELECT pr.id, pr.title, pr.brand, pr.status::text, COALESCE(v.views, 0), COALESCE(v.adds, 0), COALESCE(o.qty, 0)
			FROM products pr LEFT JOIN v ON v.product_id = pr.id LEFT JOIN o ON o.product_id = pr.id
			WHERE pr.store_id = $1 AND (v.product_id IS NOT NULL OR o.product_id IS NOT NULL)
			ORDER BY COALESCE(o.qty, 0) * 50 + COALESCE(v.adds, 0) * 5 + COALESCE(v.views, 0) DESC, pr.id
			LIMIT 10`, p.StoreID, st.From, st.To, from, end)
		if err != nil {
			return err
		}
		if st.Top, err = pgx.CollectRows(rows, pgx.RowToStructByPos[TopProduct]); err != nil {
			return err
		}
		if st.Top == nil {
			st.Top = []TopProduct{}
		}
		rows, err = h.DB.Query(ctx, `
			SELECT to_char(g.day, 'YYYY-MM-DD'), COALESCE(sum(d.views), 0), COALESCE(sum(d.fitting_adds), 0)
			FROM generate_series($2::date, $3::date, interval '1 day') AS g(day)
			LEFT JOIN product_stats_daily d ON d.day = g.day::date
			     AND d.product_id IN (SELECT id FROM products WHERE store_id = $1)
			GROUP BY g.day ORDER BY g.day`, p.StoreID, st.From, st.To)
		if err != nil {
			return err
		}
		st.Daily, err = pgx.CollectRows(rows, pgx.RowToStructByPos[DayPoint])
		return err
	}()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, st)
}
