// Package shop — покупательская корзина и избранные товары клиента.
// Корзина — отдельный модуль, не смешивается с примерочной (ТЗ, раздел 9).
// Всё хранится на сервере, поэтому корзина одна на всех устройствах клиента.
package shop

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/products"
)

const (
	MaxCartItems = 100
	MaxFavorites = 500
	MaxQty       = 99
)

type Handlers struct {
	DB         *pgxpool.Pool
	PublicBase string
}

// CartRoutes монтируются под /v1/me/cart (только клиент).
func (h *Handlers) CartRoutes(r chi.Router) {
	r.Get("/", h.cart)
	// ?size=M — размер, если у товара есть размеры
	r.Put("/{productID}", h.cartPut)
	r.Delete("/{productID}", h.cartDelete)
	r.Delete("/", h.cartClear)
}

// FavoriteRoutes монтируются под /v1/me/favorites (только клиент).
func (h *Handlers) FavoriteRoutes(r chi.Router) {
	r.Get("/", h.favorites)
	r.Get("/ids", h.favoriteIDs)
	r.Put("/{productID}", h.favoritePut)
	r.Delete("/{productID}", h.favoriteDelete)
}

// Card — товар в корзине/избранном. Available=false — снят с публикации, магазин
// не активен или нет в наличии: такой товар нельзя оформить.
type Card struct {
	ProductID   uuid.UUID `json:"product_id"`
	Title       string    `json:"title"`
	Brand       string    `json:"brand"`
	Price       int64     `json:"price"`
	StoreID     uuid.UUID `json:"store_id"`
	StoreName   string    `json:"store_name"`
	CategoryID  uuid.UUID `json:"category_id"`
	Zone        string    `json:"zone"`
	IsSet       bool      `json:"is_set"` // комплект: верх и низ на одном фото (примерочная из корзины)
	ImageURL    string    `json:"image_url,omitempty"`
	InStock     bool      `json:"in_stock"`
	HasSizes    bool      `json:"has_sizes"`
	Size        string    `json:"size,omitempty"`
	SizeStock   *int      `json:"size_stock,omitempty"` // остаток выбранного размера (только если меньше количества)
	Available   bool      `json:"available"`
	Unavailable string    `json:"unavailable_reason,omitempty"` // out_of_stock | not_enough_stock | size_required | unavailable
	Qty         int       `json:"qty,omitempty"`
	AddedAt     time.Time `json:"added_at"`
	imageKey    *string
	status      string
	storeStatus string
	stock       *int // остаток размера из корзины (nil — размер не найден)
}

func (c *Card) finish(base string) {
	if c.imageKey != nil {
		c.ImageURL = media.PublicURL(base, *c.imageKey)
	}
	inCart := c.Qty > 0
	switch {
	case c.status != "published" || c.storeStatus != "active":
		c.Unavailable = "unavailable"
	case inCart && c.HasSizes && c.stock == nil:
		c.Unavailable = "size_required" // размер удалён магазином или не выбран
	case inCart && c.HasSizes && *c.stock == 0, !c.HasSizes && !c.InStock, !inCart && !c.InStock:
		c.Unavailable = "out_of_stock"
	case inCart && c.HasSizes && *c.stock < c.Qty:
		c.Unavailable = "not_enough_stock"
		c.SizeStock = c.stock
	default:
		c.Available = true
	}
}

const cardCols = `p.id, p.title, p.brand, p.price, p.store_id, s.name, p.category_id, c.zone::text, p.is_set,
	` + products.CardImageSQL + `,
	p.in_stock, EXISTS (SELECT 1 FROM product_sizes ps WHERE ps.product_id = p.id), p.status::text, s.status::text`

func scanCard(row pgx.Row, c *Card, extra ...any) error {
	dst := append([]any{&c.ProductID, &c.Title, &c.Brand, &c.Price, &c.StoreID, &c.StoreName, &c.CategoryID, &c.Zone, &c.IsSet,
		&c.imageKey, &c.InStock, &c.HasSizes, &c.status, &c.storeStatus}, extra...)
	return row.Scan(dst...)
}

func productID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "productID"))
	if err != nil {
		return uuid.Nil, httpx.ErrNotFound
	}
	return id, nil
}

// ---------- корзина ----------

type CartView struct {
	Items      []Card `json:"items"`
	TotalQty   int    `json:"total_qty"`
	Total      int64  `json:"total"`        // сумма доступных к оформлению позиций
	CanOrder   bool   `json:"can_checkout"` // есть доступные и нет недоступных
	HasProblem bool   `json:"has_unavailable"`
}

func (h *Handlers) loadCart(r *http.Request, uid uuid.UUID) (*CartView, error) {
	rows, err := h.DB.Query(r.Context(), cartSQL, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v := &CartView{Items: []Card{}}
	for rows.Next() {
		var c Card
		if err := scanCard(rows, &c, &c.Qty, &c.AddedAt, &c.Size, &c.stock); err != nil {
			return nil, err
		}
		c.finish(h.PublicBase)
		if c.Available {
			v.Total += c.Price * int64(c.Qty)
			v.TotalQty += c.Qty
		} else {
			v.HasProblem = true
		}
		v.Items = append(v.Items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	v.CanOrder = v.TotalQty > 0 && !v.HasProblem
	return v, nil
}

func (h *Handlers) cart(w http.ResponseWriter, r *http.Request) {
	v, err := h.loadCart(r, auth.MustFrom(r.Context()).UserID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, v)
}

type qtyReq struct {
	Qty int `json:"qty" validate:"required,min=1,max=99"`
}

var errLimit = errors.New("limit")

// cartPut — добавить товар или задать количество. Добавить можно только
// опубликованный товар активного магазина, который есть в наличии. Если у товара
// есть размеры, размер обязателен (?size=M) и количество не больше остатка.
func (h *Handlers) cartPut(w http.ResponseWriter, r *http.Request) {
	pid, err := productID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var req qtyReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	size := products.NormalizeSize(r.URL.Query().Get("size"))
	uid := auth.MustFrom(r.Context()).UserID
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var inStock, hasSizes bool
		err := tx.QueryRow(r.Context(), `SELECT p.in_stock, EXISTS (SELECT 1 FROM product_sizes ps WHERE ps.product_id = p.id)
			FROM products p JOIN stores s ON s.id = p.store_id
			WHERE p.id = $1 AND p.status = 'published' AND s.status = 'active'`, pid).Scan(&inStock, &hasSizes)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		switch {
		case hasSizes && size == "":
			return &httpx.Error{Status: 422, Code: "size_required", Message: "choose a size", Fields: map[string]string{"size": "required"}}
		case hasSizes:
			var stock int
			err := tx.QueryRow(r.Context(), `SELECT stock FROM product_sizes WHERE product_id = $1 AND size = $2`, pid, size).Scan(&stock)
			if errors.Is(err, pgx.ErrNoRows) {
				return &httpx.Error{Status: 422, Code: "size_unknown", Message: "no such size", Fields: map[string]string{"size": "unknown"}}
			}
			if err != nil {
				return err
			}
			if stock == 0 {
				return httpx.NewError(http.StatusConflict, "out_of_stock", "size is out of stock")
			}
			if req.Qty > stock {
				return &httpx.Error{Status: http.StatusConflict, Code: "not_enough_stock", Message: "not enough items of this size",
					Fields: map[string]string{"available": strconv.Itoa(stock)}}
			}
		case size != "":
			return &httpx.Error{Status: 422, Code: "size_unknown", Message: "product has no sizes", Fields: map[string]string{"size": "unknown"}}
		case !inStock:
			return httpx.NewError(http.StatusConflict, "out_of_stock", "product is out of stock")
		}
		// лимит позиций: блокируем строку пользователя, чтобы параллельные запросы не обошли его
		if _, err := tx.Exec(r.Context(), `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, uid); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM cart_items WHERE user_id = $1 AND NOT (product_id = $2 AND size = $3)`,
			uid, pid, size).Scan(&n); err != nil {
			return err
		}
		if n >= MaxCartItems {
			return errLimit
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO cart_items (user_id, product_id, size, qty) VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, product_id, size) DO UPDATE SET qty = EXCLUDED.qty`, uid, pid, size, req.Qty)
		return err
	})
	if errors.Is(err, errLimit) {
		httpx.Fail(w, r, httpx.NewError(422, "cart_full", "too many items in cart"))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.cart(w, r)
}

func (h *Handlers) cartDelete(w http.ResponseWriter, r *http.Request) {
	pid, err := productID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	size := products.NormalizeSize(r.URL.Query().Get("size"))
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM cart_items WHERE user_id = $1 AND product_id = $2 AND size = $3`,
		auth.MustFrom(r.Context()).UserID, pid, size); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.cart(w, r)
}

func (h *Handlers) cartClear(w http.ResponseWriter, r *http.Request) {
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM cart_items WHERE user_id = $1`, auth.MustFrom(r.Context()).UserID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.cart(w, r)
}

// ---------- избранное ----------

func (h *Handlers) favorites(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), favoritesSQL, auth.MustFrom(r.Context()).UserID, MaxFavorites)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := []Card{}
	for rows.Next() {
		var c Card
		if err := scanCard(rows, &c, &c.AddedAt); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		c.finish(h.PublicBase)
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.List(w, items, "")
}

// favoriteIDs — только id, чтобы каталог быстро отрисовал «сердечки».
func (h *Handlers) favoriteIDs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `SELECT product_id FROM favorites WHERE user_id = $1 LIMIT $2`,
		auth.MustFrom(r.Context()).UserID, MaxFavorites)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if ids == nil {
		ids = []uuid.UUID{}
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, ids)
}

func (h *Handlers) favoritePut(w http.ResponseWriter, r *http.Request) {
	pid, err := productID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	uid := auth.MustFrom(r.Context()).UserID
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM products p JOIN stores s ON s.id = p.store_id
			WHERE p.id = $1 AND p.status = 'published' AND s.status = 'active')`, pid).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return httpx.ErrNotFound
		}
		if _, err := tx.Exec(r.Context(), `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, uid); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM favorites WHERE user_id = $1`, uid).Scan(&n); err != nil {
			return err
		}
		if n >= MaxFavorites {
			return errLimit
		}
		_, err := tx.Exec(r.Context(), `INSERT INTO favorites (user_id, product_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, uid, pid)
		return err
	})
	if errors.Is(err, errLimit) {
		httpx.Fail(w, r, httpx.NewError(422, "favorites_full", "too many favorites"))
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handlers) favoriteDelete(w http.ResponseWriter, r *http.Request) {
	pid, err := productID(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := h.DB.Exec(r.Context(), `DELETE FROM favorites WHERE user_id = $1 AND product_id = $2`,
		auth.MustFrom(r.Context()).UserID, pid); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// Запросы собираются из констант на этапе компиляции.
const (
	cartSQL = `SELECT ` + cardCols + `, ci.qty, ci.added_at, ci.size, sz.stock
		FROM cart_items ci JOIN products p ON p.id = ci.product_id JOIN stores s ON s.id = p.store_id
		JOIN categories c ON c.id = p.category_id
		LEFT JOIN product_sizes sz ON sz.product_id = ci.product_id AND sz.size = ci.size
		WHERE ci.user_id = $1 ORDER BY s.name, ci.added_at, ci.size`
	favoritesSQL = `SELECT ` + cardCols + `, f.created_at
		FROM favorites f JOIN products p ON p.id = f.product_id JOIN stores s ON s.id = p.store_id
		JOIN categories c ON c.id = p.category_id
		WHERE f.user_id = $1 ORDER BY f.created_at DESC LIMIT $2`
)
