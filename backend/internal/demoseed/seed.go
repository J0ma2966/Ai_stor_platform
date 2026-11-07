// Package demoseed наполняет платформу тестовыми магазинами и товарами
// (команда «api demo-data») и удаляет их («api demo-data remove»).
//
// Фото товаров встроены в программу (папка photos/), интернет не нужен.
// Повторный запуск приводит тестовые магазины к каталогу: создаёт недостающее,
// обновляет описания магазинов, названия, цены и размеры товаров, заменяет фото,
// которое отличается от файла в photos/ (например, нарисованную картинку прошлой
// версии), и убирает товары прошлых версий каталога (Retired). Пароли учётных
// записей магазинов не меняются. Покупателей seed не создаёт.
package demoseed

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/platform/db"
	"github.com/boutique-market/backend/internal/products"
)

//go:embed photos/*.jpg
var photoFiles embed.FS

// PhotoFile — путь к фото товара внутри photos/: артикул в нижнем регистре.
func PhotoFile(sku string) string { return "photos/" + strings.ToLower(sku) + ".jpg" }

// Photo возвращает фото товара после той же обработки, что и загрузки магазинов
// (проверка типа, очистка метаданных, JPEG). Результат детерминирован: по нему
// повторный запуск узнаёт, что фото в хранилище уже нужное.
func Photo(sku string) ([]byte, error) {
	if b, ok := processed.Load(sku); ok {
		return b.([]byte), nil
	}
	raw, err := photoFiles.ReadFile(PhotoFile(sku))
	if err != nil {
		return nil, err
	}
	b, err := media.Process(bytes.NewReader(raw), 10<<20)
	if err != nil {
		return nil, err
	}
	processed.Store(sku, b)
	return b, nil
}

// processed — уже обработанные фото (артикул → JPEG): повторная обработка дорогая.
var processed sync.Map

type Options struct {
	// Password — пароль новых учётных записей тестовых магазинов (вход в кабинет
	// магазина по логину из каталога). Минимум 10 символов. Пароль уже созданных
	// учётных записей не меняется.
	Password string
	Log      *slog.Logger
}

type Result struct {
	StoresCreated   int
	ProductsCreated int
	ProductsUpdated int // название, описание, цена или категория приведены к каталогу
	PhotosAdded     int
	PhotosReplaced  int // старая картинка (например, нарисованная) заменена фото из photos/
	ProductsRetired int // удалены товары прошлых версий каталога
}

// Seed создаёт или обновляет тестовые магазины (статус active, с учётной записью
// магазина) и их товары с фото; новые товары сразу опубликованы.
func Seed(ctx context.Context, pool *pgxpool.Pool, store media.Storage, opt Options) (Result, error) {
	var res Result
	if len(opt.Password) < 10 {
		return res, errors.New("store password must be at least 10 characters")
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
		n, err := retire(ctx, pool, store, storeID, opt.Log)
		if err != nil {
			return res, fmt.Errorf("store %s: retire old products: %w", s.Slug, err)
		}
		res.ProductsRetired += n
		var cover []byte
		for _, p := range s.Products {
			catID, ok := cats[p.Category]
			if !ok {
				return res, fmt.Errorf("product %s: unknown category %q", p.SKU, p.Category)
			}
			img, err := Photo(p.SKU)
			if err != nil {
				return res, fmt.Errorf("product %s: photo %s: %w", p.SKU, PhotoFile(p.SKU), err)
			}
			if cover == nil {
				cover = img
			}
			productID, created, updated, err := upsertProduct(ctx, pool, storeID, catID, p)
			if err != nil {
				return res, fmt.Errorf("product %s: %w", p.SKU, err)
			}
			if created {
				res.ProductsCreated++
			} else if updated {
				res.ProductsUpdated++
			}
			added, replaced, err := syncPhoto(ctx, pool, store, opt.Log, productID, userID, img)
			if err != nil {
				return res, fmt.Errorf("product %s: %w", p.SKU, err)
			}
			if added {
				res.PhotosAdded++
			}
			if replaced {
				res.PhotosReplaced++
			}
		}
		if cover != nil {
			if err := syncCover(ctx, pool, store, opt.Log, storeID, userID, cover); err != nil {
				return res, fmt.Errorf("store %s: cover: %w", s.Slug, err)
			}
		}
	}
	return res, nil
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
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			created = true
			err = tx.QueryRow(ctx, `
				INSERT INTO stores (slug, name, description, phone, address, lat, lng, status, activated_at)
				VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', now()) RETURNING id`,
				s.Slug, s.Name, s.Description, s.Phone, s.Address, s.Lat, s.Lng).Scan(&storeID)
		case err == nil:
			// описание из каталога: прошлые версии обещали товары, которых больше нет
			_, err = tx.Exec(ctx, `UPDATE stores SET name = $2, description = $3 WHERE id = $1
				AND (name, description) IS DISTINCT FROM ($2, $3)`, storeID, s.Name, s.Description)
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

// upsertProduct создаёт товар (черновик до появления фото: витрина не показывает
// товары без картинки) или приводит существующий к каталогу. Статус существующего
// товара не меняется: снятый с публикации вручную так и остаётся снятым.
func upsertProduct(ctx context.Context, pool *pgxpool.Pool, storeID, catID uuid.UUID, p Product) (id uuid.UUID, created, updated bool, err error) {
	err = db.Tx(ctx, pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id FROM products WHERE store_id = $1 AND sku = $2 FOR UPDATE`, storeID, p.SKU).Scan(&id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			created = true
			err = tx.QueryRow(ctx, `
				INSERT INTO products (store_id, category_id, sku, title, description, brand, price, in_stock, is_set, status)
				VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8, 'draft') RETURNING id`,
				storeID, catID, p.SKU, p.Title, p.Description, p.Brand, p.Price*100, p.Set).Scan(&id)
		case err == nil:
			var tag pgconn.CommandTag
			tag, err = tx.Exec(ctx, `
				UPDATE products SET category_id = $2, title = $3, description = $4, brand = $5, price = $6, is_set = $7
				WHERE id = $1 AND (category_id, title, description, brand, price, is_set) IS DISTINCT FROM ($2, $3, $4, $5, $6, $7)`,
				id, catID, p.Title, p.Description, p.Brand, p.Price*100, p.Set)
			updated = tag.RowsAffected() > 0
		}
		if err != nil {
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

// sameObject — в хранилище по key лежит ровно want.
func sameObject(ctx context.Context, store media.Storage, key string, want []byte) bool {
	got, err := store.Get(ctx, key)
	return err == nil && bytes.Equal(got, want)
}

// syncPhoto ставит товару фото «спереди», если его нет или оно отличается от img,
// и публикует черновик. Старый файл удаляется после коммита.
func syncPhoto(ctx context.Context, pool *pgxpool.Pool, store media.Storage, log *slog.Logger, productID, userID uuid.UUID, img []byte) (added, replaced bool, err error) {
	var old string
	err = pool.QueryRow(ctx, `SELECT key FROM product_images WHERE product_id = $1 AND angle = 'front'`, productID).Scan(&old)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, false, err
	}
	if old != "" && sameObject(ctx, store, old, img) {
		return false, false, nil
	}
	key := media.NewKey(userID, media.PurposeProductImage)
	if err := store.Put(ctx, key, img, "image/jpeg"); err != nil {
		return false, false, fmt.Errorf("upload photo: %w", err)
	}
	err = db.Tx(ctx, pool, func(tx pgx.Tx) error {
		var tag pgconn.CommandTag
		var err error
		if old == "" {
			tag, err = tx.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, 'front', $2)
				ON CONFLICT DO NOTHING`, productID, key)
		} else {
			// смена ключа сбрасывает уменьшенные копии в pending — воркер сделает их заново
			tag, err = tx.Exec(ctx, `UPDATE product_images SET key = $3 WHERE product_id = $1 AND angle = 'front' AND key = $2`,
				productID, old, key)
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("product photo was changed at the same time; run the command again")
		}
		// публикуем только черновики; товар, который сняли с публикации вручную, не трогаем
		_, err = tx.Exec(ctx, `
			UPDATE products SET status = 'published', published_at = COALESCE(published_at, now())
			WHERE id = $1 AND status = 'draft'`, productID)
		return err
	})
	if err != nil {
		_ = store.Delete(ctx, key)
		return false, false, err
	}
	if old != "" {
		deleteUnreferenced(ctx, pool, store, log, []string{old})
	}
	return old == "", old != "", nil
}

// syncCover ставит обложкой магазина img (фото первого товара), если обложки нет
// или она другая.
func syncCover(ctx context.Context, pool *pgxpool.Pool, store media.Storage, log *slog.Logger, storeID, userID uuid.UUID, img []byte) error {
	var old *string
	if err := pool.QueryRow(ctx, `SELECT cover_key FROM stores WHERE id = $1`, storeID).Scan(&old); err != nil {
		return err
	}
	if old != nil && sameObject(ctx, store, *old, img) {
		return nil
	}
	key := media.NewKey(userID, media.PurposeStoreBranding)
	if err := store.Put(ctx, key, img, "image/jpeg"); err != nil {
		return err
	}
	tag, err := pool.Exec(ctx, `UPDATE stores SET cover_key = $3 WHERE id = $1 AND cover_key IS NOT DISTINCT FROM $2`, storeID, old, key)
	if err == nil && tag.RowsAffected() == 0 {
		err = errors.New("store cover was changed at the same time; run the command again")
	}
	if err != nil {
		_ = store.Delete(ctx, key)
		return err
	}
	if old != nil {
		deleteUnreferenced(ctx, pool, store, log, []string{*old})
	}
	return nil
}

// retire удаляет из тестового магазина товары прошлых версий каталога (Retired).
// Товары, которые добавили вручную, и заблокированные модерацией не трогаются.
// Заказы хранят снимок товара, поэтому удалять можно и заказанные товары.
func retire(ctx context.Context, pool *pgxpool.Pool, store media.Storage, storeID uuid.UUID, log *slog.Logger) (int, error) {
	var keys []string
	var n int
	err := db.Tx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT i.key FROM product_images i JOIN products p ON p.id = i.product_id
			WHERE p.store_id = $1 AND p.sku = ANY($2) AND p.status <> 'blocked'`, storeID, Retired)
		if err != nil {
			return err
		}
		if keys, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, `DELETE FROM products WHERE store_id = $1 AND sku = ANY($2) AND status <> 'blocked' RETURNING id`, storeID, Retired)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		n = len(ids)
		return err
	})
	if err != nil {
		return 0, err
	}
	deleteUnreferenced(ctx, pool, store, log, keys)
	return n, nil
}

// deleteUnreferenced удаляет файлы (с уменьшенными копиями), на которые больше не
// ссылаются ни товары, ни снимки заказов, ни магазины. Ошибка хранилища не
// критична: осиротевший файл безопасен (ключ случайный).
func deleteUnreferenced(ctx context.Context, pool *pgxpool.Pool, store media.Storage, log *slog.Logger, keys []string) {
	if len(keys) == 0 {
		return
	}
	rows, err := pool.Query(ctx, `SELECT k FROM unnest($1::text[]) AS k
		WHERE NOT EXISTS (SELECT 1 FROM product_images WHERE key = k)
		  AND NOT EXISTS (SELECT 1 FROM order_items WHERE image_key = k)
		  AND NOT EXISTS (SELECT 1 FROM stores WHERE cover_key = k OR logo_key = k)`, keys)
	if err != nil {
		log.Warn("demo file cleanup skipped", slog.String("err", err.Error()))
		return
	}
	free, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		log.Warn("demo file cleanup skipped", slog.String("err", err.Error()))
		return
	}
	for _, k := range free {
		for _, key := range append([]string{k}, variantKeys(k)...) {
			if err := store.Delete(ctx, key); err != nil {
				log.Warn("demo file delete failed", slog.String("key", key), slog.String("err", err.Error()))
			}
		}
	}
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
