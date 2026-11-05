package products

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/boutique-market/backend/internal/audit"
	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/httpx"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
)

// storeGet — полная карточка своего товара (для формы редактирования).
func (h *Handlers) storeGet(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var it Product
	err = h.DB.QueryRow(r.Context(), `
		SELECT p.id, p.store_id, p.category_id, c.zone::text, p.sku, p.title, p.description, p.brand, p.price, p.in_stock,
		       p.status, p.moderation_reason, p.created_at, p.views_count, p.fitting_adds,
		       `+imagesSQL+`
		FROM products p JOIN categories c ON c.id = p.category_id
		WHERE p.id = $1 AND p.store_id = $2`, id, p.StoreID).
		Scan(&it.ID, &it.StoreID, &it.CategoryID, &it.Zone, &it.SKU, &it.Title, &it.Description, &it.Brand, &it.Price, &it.InStock,
			&it.Status, &it.Reason, &it.CreatedAt, &it.Views, &it.FittingAdds, &it.imageKeys)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	it.Images, it.Thumbs = h.urls(it.imageKeys)
	if it.Sizes, err = LoadSizes(r.Context(), h.DB, it.ID, true); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusOK, it)
}

type updateReq struct {
	CategoryID  *uuid.UUID        `json:"category_id"`
	SKU         *string           `json:"sku" validate:"omitempty,min=1,max=64"`
	Title       *string           `json:"title" validate:"omitempty,min=1,max=300"`
	Description *string           `json:"description" validate:"omitempty,min=1,max=10000"`
	Brand       *string           `json:"brand" validate:"omitempty,min=1,max=120"`
	Price       *int64            `json:"price" validate:"omitempty,gt=0"`
	InStock     *bool             `json:"in_stock"`
	Images      map[string]string `json:"images" validate:"omitempty,max=3,dive,keys,oneof=front side back,endkeys,required"`
	// nil — размеры не меняются, [] — убрать все размеры (наличие снова по флажку in_stock).
	Sizes *[]SizeIn `json:"sizes"`
}

// storeUpdate — частичное редактирование. Заблокированный администратором товар
// не редактируется. Товар, снятый модератором (unpublished), после правки
// возвращается в черновик: магазин исправил замечание и может снова опубликовать.
func (h *Handlers) storeUpdate(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var req updateReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for angle, key := range req.Images {
		if !media.OwnsKey(p.UserID, media.PurposeProductImage, key) {
			httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "image key not owned", Fields: map[string]string{"images." + angle: "not_owned"}})
			return
		}
	}
	var sizes []SizeIn
	if req.Sizes != nil {
		var fields map[string]string
		if sizes, fields = NormalizeSizes(*req.Sizes); fields != nil {
			httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "invalid sizes", Fields: fields})
			return
		}
	}
	trim := func(s *string) *string {
		if s == nil {
			return nil
		}
		v := strings.TrimSpace(*s)
		return &v
	}
	var old []string
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(r.Context(), `SELECT status::text FROM products WHERE id = $1 AND store_id = $2 FOR UPDATE`, id, p.StoreID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound
		}
		if err != nil {
			return err
		}
		if status == "blocked" {
			return httpx.NewError(http.StatusConflict, "product_blocked", "product is blocked by moderation")
		}
		if _, err := tx.Exec(r.Context(), `UPDATE products SET
				category_id = COALESCE($3, category_id),
				sku         = COALESCE($4, sku),
				title       = COALESCE($5, title),
				description = COALESCE($6, description),
				brand       = COALESCE($7, brand),
				price       = COALESCE($8, price),
				in_stock    = COALESCE($9, in_stock),
				status      = CASE WHEN status = 'unpublished' THEN 'draft'::product_status ELSE status END
			WHERE id = $1 AND store_id = $2`,
			id, p.StoreID, req.CategoryID, trim(req.SKU), trim(req.Title), trim(req.Description), trim(req.Brand), req.Price, req.InStock); err != nil {
			return err
		}
		for angle, key := range req.Images {
			var prev *string
			err := tx.QueryRow(r.Context(), `SELECT key FROM product_images WHERE product_id = $1 AND angle = $2`, id, angle).Scan(&prev)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if prev != nil && *prev != key {
				old = append(old, *prev)
			}
			if _, err := tx.Exec(r.Context(), `INSERT INTO product_images (product_id, angle, key) VALUES ($1, $2, $3)
				ON CONFLICT (product_id, angle) DO UPDATE SET key = EXCLUDED.key`, id, angle, key); err != nil {
				return err
			}
		}
		if req.Sizes != nil {
			return ReplaceSizes(r.Context(), tx, id, sizes)
		}
		return nil
	})
	if err != nil {
		h.productWriteErr(w, r, err)
		return
	}
	h.deleteUnreferenced(r, old)
	h.storeGet(w, r)
}

func (h *Handlers) productWriteErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case db.IsUniqueViolation(err, "products_store_id_sku_key"):
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "sku_taken", "product with this SKU already exists"))
	case isFKViolation(err):
		httpx.Fail(w, r, &httpx.Error{Status: 422, Code: "validation_failed", Message: "unknown category", Fields: map[string]string{"category_id": "unknown"}})
	default:
		httpx.Fail(w, r, err)
	}
}

// storeDelete удаляет товар. В оформленных заказах остаётся снимок (название,
// цена, фото), поэтому файлы, на которые ссылаются заказы, не удаляются.
func (h *Handlers) storeDelete(w http.ResponseWriter, r *http.Request) {
	p := auth.MustFrom(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	var keys []string
	err = db.Tx(r.Context(), h.DB, func(tx pgx.Tx) error {
		rows, err := tx.Query(r.Context(), `SELECT pi.key FROM product_images pi JOIN products p ON p.id = pi.product_id
			WHERE p.id = $1 AND p.store_id = $2`, id, p.StoreID)
		if err != nil {
			return err
		}
		if keys, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		tag, err := tx.Exec(r.Context(), `DELETE FROM products WHERE id = $1 AND store_id = $2 AND status <> 'blocked'`, id, p.StoreID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.NewError(http.StatusNotFound, "not_found", "product not found or locked by moderation")
		}
		return audit.Log(r.Context(), tx, "product.delete", "product", id.String(), nil)
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.deleteUnreferenced(r, keys)
	httpx.NoContent(w)
}

// deleteUnreferenced удаляет файлы, которые больше нигде не используются
// (ни в товарах, ни в снимках заказов). Ошибки хранилища не ломают ответ:
// осиротевший файл безопасен (ключ случайный), его можно дочистить позже.
func (h *Handlers) deleteUnreferenced(r *http.Request, keys []string) {
	if h.Media == nil || len(keys) == 0 {
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT k FROM unnest($1::text[]) AS k
		WHERE NOT EXISTS (SELECT 1 FROM product_images WHERE key = k)
		  AND NOT EXISTS (SELECT 1 FROM order_items WHERE image_key = k)`, keys)
	if err != nil {
		return
	}
	free, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return
	}
	for _, k := range free {
		// вместе с оригиналом — его уменьшенные копии (их могло и не быть: удаление идемпотентно)
		for _, key := range append([]string{k}, variantKeys(k)...) {
			if err := h.Media.Delete(r.Context(), key); err != nil {
				httpx.LoggerFrom(r.Context()).Warn("product image delete failed")
			}
		}
	}
}

func variantKeys(key string) []string {
	out := make([]string, 0, len(media.ProductVariants))
	for _, side := range media.ProductVariants {
		out = append(out, media.VariantKey(key, side))
	}
	return out
}
