// Package demoseed наполняет платформу тестовыми магазинами и товарами
// (команда «api demo-data») и удаляет их («api demo-data remove»).
//
// Повторный запуск безопасен: существующие магазины, учётные записи и товары
// (по slug и артикулу) не меняются, только докачиваются фото, которых не хватает.
// Покупателей seed не создаёт.
package demoseed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/products"
)

// FetchFunc скачивает фото по id Unsplash.
type FetchFunc func(ctx context.Context, photoID string) ([]byte, error)

type Options struct {
	// Password — пароль учётных записей тестовых магазинов (вход в кабинет магазина
	// по логину из каталога). Минимум 10 символов. Пустой — магазины без входа нельзя,
	// поэтому seed требует пароль.
	Password string
	Fetch    FetchFunc // nil — UnsplashFetch
	MaxBytes int64     // предел размера скачанного фото
	Log      *slog.Logger
}

type Result struct {
	StoresCreated   int
	ProductsCreated int
	PhotosAdded     int
	PhotoFailures   []string // артикулы товаров без фото (оставлены черновиками)
}

var photoIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// UnsplashFetch скачивает фото шириной 1600 px через официальную ссылку скачивания
// (https://unsplash.com/photos/<id>/download), как это делает кнопка Download на сайте.
func UnsplashFetch(client *http.Client) FetchFunc {
	return func(ctx context.Context, id string) ([]byte, error) {
		if !photoIDRe.MatchString(id) {
			return nil, fmt.Errorf("bad unsplash id %q", id)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			"https://unsplash.com/photos/"+id+"/download?force=true&w=1600", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "boutique-market-demo-seed/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unsplash %s: HTTP %d", id, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 30<<20))
	}
}

// Seed создаёт магазины (статус active, с учётной записью магазина) и их товары
// (опубликованы, если фото скачалось; иначе черновик).
func Seed(ctx context.Context, pool *pgxpool.Pool, store media.Storage, opt Options) (Result, error) {
	var res Result
	if len(opt.Password) < 10 {
		return res, errors.New("store password must be at least 10 characters")
	}
	if opt.Fetch == nil {
		opt.Fetch = UnsplashFetch(&http.Client{Timeout: 60 * time.Second})
	}
	if opt.MaxBytes <= 0 {
		opt.MaxBytes = 10 << 20
	}
	if opt.Log == nil {
		opt.Log = slog.Default()
	}
	hash, err := auth.HashPassword(opt.Password)
	if err != nil {
		return res, err
	}
	cats, err := categoryIDs(ctx, pool)
	if err != nil {
		return res, err
	}
	for _, s := range Catalog {
		storeID, userID, created, err := ensureStore(ctx, pool, s, hash)
		if err != nil {
			return res, fmt.Errorf("store %s: %w", s.Slug, err)
		}
		if created {
			res.StoresCreated++
			opt.Log.Info("demo store created", slog.String("slug", s.Slug), slog.String("login", s.Login))
		}
		var coverSrc []byte
		for _, p := range s.Products {
			catID, ok := cats[p.Category]
			if !ok {
				return res, fmt.Errorf("product %s: unknown category %q", p.SKU, p.Category)
			}
			productID, hasPhoto, created, err := ensureProduct(ctx, pool, storeID, catID, p)
			if err != nil {
				return res, fmt.Errorf("product %s: %w", p.SKU, err)
			}
			if created {
				res.ProductsCreated++
			}
			if hasPhoto {
				continue
			}
			img, err := fetchPhoto(ctx, opt, p.Photo)
			if err != nil {
				opt.Log.Warn("demo photo not downloaded; product stays draft",
					slog.String("sku", p.SKU), slog.String("photo", p.Photo), slog.String("err", err.Error()))
				res.PhotoFailures = append(res.PhotoFailures, p.SKU)
				continue
			}
			key := media.NewKey(userID, media.PurposeProductImage)
			if err := store.Put(ctx, key, img, "image/jpeg"); err != nil {
				return res, fmt.Errorf("product %s: upload photo: %w", p.SKU, err)
			}
			if err := attachPhoto(ctx, pool, productID, key); err != nil {
				_ = store.Delete(ctx, key)
				return res, fmt.Errorf("product %s: %w", p.SKU, err)
			}
			res.PhotosAdded++
			if coverSrc == nil {
				coverSrc = img
			}
		}
		if coverSrc != nil {
			if err := ensureCover(ctx, pool, store, storeID, userID, coverSrc); err != nil {
				return res, fmt.Errorf("store %s: cover: %w", s.Slug, err)
			}
		}
	}
	return res, nil
}

func fetchPhoto(ctx context.Context, opt Options, id string) ([]byte, error) {
	raw, err := opt.Fetch(ctx, id)
	if err != nil {
		return nil, err
	}
	// та же очистка, что и у загрузок магазинов: проверка типа, EXIF, перекодирование в JPEG
	return media.Process(bytes.NewReader(raw), opt.MaxBytes)
}

func categoryIDs(ctx context.Context, pool *pgxpool.Pool) (map[string]uuid.UUID, error) {
	rows, err := pool.Query(ctx, `SELECT slug, id FROM categories`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]uuid.UUID{}
	for rows.Next() {
		var slug string
		var id uuid.UUID
		if err := rows.Scan(&slug, &id); err != nil {
			return nil, err
		}
		out[slug] = id
	}
	return out, rows.Err()
}

func ensureStore(ctx context.Context, pool *pgxpool.Pool, s Store, hash string) (storeID, userID uuid.UUID, created bool, err error) {
	err = db.Tx(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id FROM stores WHERE slug = $1`, s.Slug).Scan(&storeID)
		if errors.Is(err, pgx.ErrNoRows) {
			created = true
			err = tx.QueryRow(ctx, `
				INSERT INTO stores (slug, name, description, phone, address, lat, lng, status, activated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', now()) RETURNING id`,
				s.Slug, s.Name, s.Description, s.Phone, s.Address, s.Lat, s.Lng).Scan(&storeID)
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT id FROM users WHERE store_id = $1`, storeID).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(ctx, `
				INSERT INTO users (role, login, password_hash, store_id) VALUES ('store', $1, $2, $3) RETURNING id`,
				s.Login, hash, storeID).Scan(&userID)
			if db.IsUniqueViolation(err, "users_login_key") {
				return fmt.Errorf("login %q is already used by another account", s.Login)
			}
		}
		return err
	})
	return
}

func ensureProduct(ctx context.Context, pool *pgxpool.Pool, storeID, catID uuid.UUID, p Product) (id uuid.UUID, hasPhoto, created bool, err error) {
	err = db.Tx(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT p.id, EXISTS (SELECT 1 FROM product_images i WHERE i.product_id = p.id)
			FROM products p WHERE p.store_id = $1 AND p.sku = $2`, storeID, p.SKU).Scan(&id, &hasPhoto)
		if err == nil || !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		created = true
		// черновик до появления фото: витрина не показывает товары без картинки
		if err := tx.QueryRow(ctx, `
			INSERT INTO products (store_id, category_id, sku, title, description, brand, price, in_stock, is_set, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8, 'draft') RETURNING id`,
			storeID, catID, p.SKU, p.Title, p.Description, p.Brand, p.Price*100, p.Set).Scan(&id); err != nil {
			return err
		}
		sizes := make([]products.SizeIn, len(p.Sizes))
		for i, s := range p.Sizes {
			sizes[i] = products.SizeIn{Size: s.Size, Stock: s.Stock}
		}
		return products.ReplaceSizes(ctx, tx, id, sizes)
	})
	return
}

func attachPhoto(ctx context.Context, pool *pgxpool.Pool, productID uuid.UUID, key string) error {
	return db.Tx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, 'front', $2)`, productID, key); err != nil {
			return err
		}
		// публикуем только черновики seed; товар, который сняли с публикации вручную, не трогаем
		_, err := tx.Exec(ctx, `
			UPDATE products SET status = 'published', published_at = COALESCE(published_at, now())
			WHERE id = $1 AND status = 'draft'`, productID)
		return err
	})
}

func ensureCover(ctx context.Context, pool *pgxpool.Pool, store media.Storage, storeID, userID uuid.UUID, img []byte) error {
	var has bool
	if err := pool.QueryRow(ctx, `SELECT cover_key IS NOT NULL FROM stores WHERE id = $1`, storeID).Scan(&has); err != nil || has {
		return err
	}
	key := media.NewKey(userID, media.PurposeStoreBranding)
	if err := store.Put(ctx, key, img, "image/jpeg"); err != nil {
		return err
	}
	tag, err := pool.Exec(ctx, `UPDATE stores SET cover_key = $2 WHERE id = $1 AND cover_key IS NULL`, storeID, key)
	if err != nil || tag.RowsAffected() == 0 {
		_ = store.Delete(ctx, key)
	}
	return err
}

// RemoveResult — что сделала команда remove.
type RemoveResult struct {
	StoresDeleted  int
	StoresArchived []string // у магазина есть заказы: удалить нельзя, магазин архивирован
}

// Remove удаляет тестовые магазины из Catalog вместе с товарами, учётными записями
// и файлами. Магазины с заказами (заказы — учётные документы) не удаляются, а
// архивируются: витрина их больше не показывает.
func Remove(ctx context.Context, pool *pgxpool.Pool, store media.Storage, log *slog.Logger) (RemoveResult, error) {
	var res RemoveResult
	if log == nil {
		log = slog.Default()
	}
	for _, s := range Catalog {
		var keys []string
		var deleted, archived bool
		err := db.Tx(ctx, pool, func(tx pgx.Tx) error {
			var storeID uuid.UUID
			err := tx.QueryRow(ctx, `SELECT id FROM stores WHERE slug = $1 FOR UPDATE`, s.Slug).Scan(&storeID)
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
			return res, fmt.Errorf("store %s: %w", s.Slug, err)
		}
		if archived {
			res.StoresArchived = append(res.StoresArchived, s.Slug)
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
