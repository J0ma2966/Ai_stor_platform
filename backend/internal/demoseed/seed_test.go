package demoseed

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/testutil"
)

func TestCatalogIsValid(t *testing.T) {
	slugs, logins, skus := map[string]bool{}, map[string]bool{}, map[string]bool{}
	zones := map[string]int{"headwear": 0, "tops": 0, "bottoms": 0, "shoes": 0}
	notice, err := os.ReadFile("photos/NOTICE.md")
	if err != nil {
		t.Fatal(err)
	}
	sets := 0
	for _, s := range Catalog {
		if !strings.HasPrefix(s.Slug, SlugPrefix) || slugs[s.Slug] || logins[s.Login] {
			t.Errorf("store %q: slug must start with %q, slug and login must be unique", s.Slug, SlugPrefix)
		}
		slugs[s.Slug], logins[s.Login] = true, true
		for _, p := range s.Products {
			// артикул уникален во всём каталоге: по нему называется файл фото
			if skus[p.SKU] {
				t.Errorf("%s: duplicate sku", p.SKU)
			}
			skus[p.SKU] = true
			if _, ok := zones[p.Category]; !ok {
				t.Errorf("%s: unknown category %q", p.SKU, p.Category)
			}
			zones[p.Category]++
			if p.Set {
				sets++
				if p.Category != "tops" {
					t.Errorf("%s: a set must be in the top zone", p.SKU)
				}
			}
			if p.Price < 10_000 || p.Title == "" || p.Description == "" || p.Brand == "" || len(p.Sizes) == 0 {
				t.Errorf("%s: incomplete product", p.SKU)
			}
			in := make([]products.SizeIn, len(p.Sizes))
			for i, sz := range p.Sizes {
				in[i] = products.SizeIn{Size: sz.Size, Stock: sz.Stock}
			}
			if _, fields := products.NormalizeSizes(in); fields != nil {
				t.Errorf("%s: invalid sizes %v", p.SKU, fields)
			}
			// фото есть, проходит ту же проверку, что и загрузки магазинов, и достаточно крупное для примерки
			img, err := Photo(p.SKU)
			if err != nil {
				t.Errorf("%s: photo %s: %v", p.SKU, PhotoFile(p.SKU), err)
				continue
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(img))
			if err != nil || cfg.Width < 380 || cfg.Height < cfg.Width {
				t.Errorf("%s: photo must be a portrait JPEG at least 380 px wide, got %dx%d (%v)", p.SKU, cfg.Width, cfg.Height, err)
			}
			if name := strings.TrimPrefix(PhotoFile(p.SKU), "photos/"); !bytes.Contains(notice, []byte(name)) {
				t.Errorf("%s: photos/NOTICE.md must name the source of %s", p.SKU, name)
			}
		}
	}
	for z, n := range zones {
		if n == 0 {
			t.Errorf("fitting room zone %s has no products", z)
		}
	}
	if sets == 0 {
		t.Error("catalog needs at least one set for the fitting room")
	}
	// в папке нет лишних файлов: всё, что встроено в программу, используется
	entries, err := fs.ReadDir(photoFiles, "photos")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !skus[strings.ToUpper(strings.TrimSuffix(e.Name(), ".jpg"))] {
			t.Errorf("photos/%s does not belong to any product", e.Name())
		}
	}
	for _, sku := range Retired {
		if skus[sku] {
			t.Errorf("%s is both in the catalog and retired", sku)
		}
	}
}

func jpegBytes(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 60, 80))
	for x := 0; x < 60; x++ {
		for y := 0; y < 80; y++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 3), 120, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSeedIdempotentAndRemove(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	store := media.NewMemory()
	total := 0
	var catalogSKUs []string
	for _, s := range Catalog {
		total += len(s.Products)
		for _, p := range s.Products {
			catalogSKUs = append(catalogSKUs, p.SKU)
		}
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	str := func(q string, args ...any) string {
		t.Helper()
		var s string
		if err := pool.QueryRow(ctx, q, args...).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	photoKey := func(sku string) string {
		return str(`SELECT i.key FROM product_images i JOIN products p ON p.id = i.product_id WHERE p.sku = $1 AND i.angle = 'front'`, sku)
	}
	mustEqualPhoto := func(key, sku string) {
		t.Helper()
		want, err := Photo(sku)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(store.Objs[key], want) {
			t.Fatalf("%s: stored file %s is not the bundled photo", sku, key)
		}
	}

	if _, err := Seed(ctx, pool, store, Options{Password: "short"}); err == nil {
		t.Fatal("short password must be rejected")
	}
	res, err := Seed(ctx, pool, store, Options{Password: "demo-password-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res != (Result{StoresCreated: len(Catalog), ProductsCreated: total, PhotosAdded: total}) {
		t.Fatalf("first run: %+v", res)
	}
	if n := count(`SELECT count(*) FROM products WHERE status = 'published'`); n != total {
		t.Fatalf("published = %d, want %d", n, total)
	}
	if n := count(`SELECT count(*) FROM stores WHERE status = 'active' AND cover_key IS NOT NULL AND slug LIKE 'demo-%'`); n != len(Catalog) {
		t.Fatalf("active stores with cover = %d", n)
	}
	if n := count(`SELECT count(*) FROM products WHERE is_set`); n == 0 {
		t.Fatal("sets were not marked")
	}
	if n := count(`SELECT count(*) FROM users WHERE role = 'client'`); n != 0 {
		t.Fatal("seed must not create shoppers")
	}
	// цена в тийинах
	if n := count(`SELECT count(*) FROM products WHERE sku = 'DL-TEE-WHT' AND price = 14900000`); n != 1 {
		t.Fatal("price must be stored in tiyin")
	}
	// у «почти распроданного» худи остаток только у одного размера, товар в наличии
	if n := count(`SELECT count(*) FROM products WHERE sku = 'DL-HD-BLK' AND in_stock`); n != 1 {
		t.Fatal("in_stock must follow sizes")
	}
	// в хранилище — именно фото из папки; ключи принадлежат учётной записи магазина,
	// иначе кабинет не сохранит товар
	rows, err := pool.Query(ctx, `SELECT u.id, p.sku, i.key FROM product_images i JOIN products p ON p.id = i.product_id JOIN users u ON u.store_id = p.store_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var uid [16]byte
		var sku, key string
		if err := rows.Scan(&uid, &sku, &key); err != nil {
			t.Fatal(err)
		}
		if !media.OwnsKey(uid, media.PurposeProductImage, key) {
			t.Fatalf("key %s is not owned by the store account", key)
		}
		mustEqualPhoto(key, sku)
	}
	rows.Close()
	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE login = $1`, Catalog[1].Login).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, _ := auth.VerifyPassword("demo-password-1", hash); !ok {
		t.Fatal("store password does not match")
	}

	// Состояние после прошлой версии команды (как у пользователя): нарисованная картинка
	// вместо фото и такая же обложка, черновик без фото, старое описание магазина,
	// товар, которого больше нет в каталоге, и товар, добавленный вручную.
	drawn := jpegBytes(t)
	oldTee := photoKey("DL-TEE-WHT")
	drawnKey := strings.Replace(oldTee, ".jpg", "x.jpg", 1)
	_ = store.Put(ctx, drawnKey, drawn, "image/jpeg")
	_ = store.Delete(ctx, oldTee)
	if _, err := pool.Exec(ctx, `UPDATE product_images SET key = $2 WHERE key = $1`, oldTee, drawnKey); err != nil {
		t.Fatal(err)
	}
	oldCover := str(`SELECT cover_key FROM stores WHERE slug = $1`, Catalog[0].Slug)
	drawnCover := strings.Replace(oldCover, ".jpg", "x.jpg", 1)
	_ = store.Put(ctx, drawnCover, drawn, "image/jpeg")
	_ = store.Delete(ctx, oldCover)
	if _, err := pool.Exec(ctx, `UPDATE stores SET cover_key = $2, description = 'старое описание' WHERE slug = $1`, Catalog[0].Slug, drawnCover); err != nil {
		t.Fatal(err)
	}
	oldSet := photoKey("AM-SET-PLT")
	_ = store.Delete(ctx, oldSet)
	if _, err := pool.Exec(ctx, `DELETE FROM product_images WHERE key = $1`, oldSet); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE products SET status = 'draft', published_at = NULL WHERE sku = 'AM-SET-PLT'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE products SET title = 'Топ с круглым вырезом, чёрный', price = 1 WHERE sku = 'AM-TOP-BLK'`); err != nil {
		t.Fatal(err)
	}
	retiredKey := strings.Replace(oldTee, ".jpg", "r.jpg", 1)
	_ = store.Put(ctx, retiredKey, drawn, "image/jpeg")
	for _, sku := range []string{Retired[0], "MY-OWN-1"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO products (store_id, category_id, sku, title, description, brand, price, status)
			SELECT s.id, c.id, $2, 'Старый товар', 'Описание', 'Brand', 100000, 'published'
			FROM stores s, categories c WHERE s.slug = $1 AND c.slug = 'tops'`, Catalog[0].Slug, sku); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) SELECT id, 'front', $2 FROM products WHERE sku = $1`, Retired[0], retiredKey); err != nil {
		t.Fatal(err)
	}

	// повторный запуск: ничего не дублирует, заменяет картинки на фото, публикует черновик,
	// возвращает описания из каталога и убирает старый товар
	res, err = Seed(ctx, pool, store, Options{Password: "another-password"})
	if err != nil {
		t.Fatal(err)
	}
	if res != (Result{ProductsUpdated: 1, ProductsRetired: 1, PhotosAdded: 1, PhotosReplaced: 1}) {
		t.Fatalf("second run: %+v", res)
	}
	if n := count(`SELECT count(*) FROM products WHERE status = 'published' AND sku = ANY($1)`, catalogSKUs); n != total {
		t.Fatalf("published after rerun = %d, want %d", n, total)
	}
	mustEqualPhoto(photoKey("DL-TEE-WHT"), "DL-TEE-WHT")
	mustEqualPhoto(photoKey("AM-SET-PLT"), "AM-SET-PLT")
	mustEqualPhoto(str(`SELECT cover_key FROM stores WHERE slug = $1`, Catalog[0].Slug), Catalog[0].Products[0].SKU)
	for _, k := range []string{drawnKey, drawnCover, retiredKey} {
		if store.Has(k) {
			t.Fatalf("old file %s was not deleted", k)
		}
	}
	if n := count(`SELECT count(*) FROM products WHERE sku = 'AM-TOP-BLK' AND title = $1 AND price = $2`, Catalog[1].Products[7].Title, Catalog[1].Products[7].Price*100); n != 1 || Catalog[1].Products[7].SKU != "AM-TOP-BLK" {
		t.Fatal("product fields must be restored from the catalog")
	}
	if d := str(`SELECT description FROM stores WHERE slug = $1`, Catalog[0].Slug); d != Catalog[0].Description {
		t.Fatalf("store description = %q", d)
	}
	if n := count(`SELECT count(*) FROM products WHERE sku = $1`, Retired[0]); n != 0 {
		t.Fatal("retired product must be deleted")
	}
	if n := count(`SELECT count(*) FROM products WHERE sku = 'MY-OWN-1' AND status = 'published'`); n != 1 {
		t.Fatal("a product added by hand must not be touched")
	}
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE login = $1`, Catalog[1].Login).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, _ := auth.VerifyPassword("demo-password-1", hash); !ok {
		t.Fatal("rerun must not change an existing password")
	}
	// третий запуск: всё уже как в каталоге, файлы не перезаливаются
	objects := len(store.Objs)
	if res, err = Seed(ctx, pool, store, Options{Password: "another-password"}); err != nil || res != (Result{}) {
		t.Fatalf("third run: %+v, %v", res, err)
	}
	if len(store.Objs) != objects {
		t.Fatalf("third run changed files: %d -> %d", objects, len(store.Objs))
	}

	// магазин с заказом архивируется, остальные удаляются вместе с файлами
	var storeID, productID [16]byte
	if err := pool.QueryRow(ctx, `SELECT s.id, p.id FROM stores s JOIN products p ON p.store_id = s.id WHERE s.slug = $1 LIMIT 1`, Catalog[2].Slug).Scan(&storeID, &productID); err != nil {
		t.Fatal(err)
	}
	var clientID [16]byte
	if err := pool.QueryRow(ctx, `INSERT INTO users (role, phone) VALUES ('client', '+998900000001') RETURNING id`).Scan(&clientID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO orders (checkout_id, user_id, store_id, items_total, customer_name, customer_phone) VALUES (gen_random_uuid(), $1, $2, 100, 'Тест', '+998900000001')`, clientID, storeID); err != nil {
		t.Fatalf("insert order: %v", err)
	}
	rr, err := Remove(ctx, pool, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rr.StoresDeleted != len(Catalog)-1 || len(rr.StoresArchived) != 1 || rr.StoresArchived[0] != Catalog[2].Slug {
		t.Fatalf("remove: %+v", rr)
	}
	if n := count(`SELECT count(*) FROM products WHERE status = 'published'`); n != 0 {
		t.Fatal("archived store products must be unpublished")
	}
	if n := count(`SELECT count(*) FROM users WHERE role = 'store'`); n != 1 {
		t.Fatalf("store users left = %d, want 1 (archived store)", n)
	}
	left := 0
	for k := range store.Objs {
		if strings.HasPrefix(k, "pub/") {
			left++
		}
	}
	if want := len(Catalog[2].Products) + 1; left != want { // фото архивного магазина + обложка
		t.Fatalf("files left = %d, want %d", left, want)
	}
}
