package demoseed

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/boutique-market/backend/internal/auth"
	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/products"
	"github.com/boutique-market/backend/internal/testutil"
)

func TestCatalogIsValid(t *testing.T) {
	slugs, logins, photos := map[string]bool{}, map[string]bool{}, map[string]bool{}
	zones := map[string]bool{"headwear": true, "tops": true, "bottoms": true, "shoes": true}
	sets := 0
	for _, s := range Catalog {
		if !strings.HasPrefix(s.Slug, SlugPrefix) || slugs[s.Slug] || logins[s.Login] {
			t.Errorf("store %q: slug must start with %q, slug and login must be unique", s.Slug, SlugPrefix)
		}
		slugs[s.Slug], logins[s.Login] = true, true
		skus := map[string]bool{}
		for _, p := range s.Products {
			if skus[p.SKU] {
				t.Errorf("%s: duplicate sku %s", s.Slug, p.SKU)
			}
			skus[p.SKU] = true
			if !zones[p.Category] {
				t.Errorf("%s: unknown category %q", p.SKU, p.Category)
			}
			if p.Set {
				sets++
				if p.Category != "tops" {
					t.Errorf("%s: a set must be in the top zone", p.SKU)
				}
			}
			if !photoIDRe.MatchString(p.Photo) || photos[p.Photo] {
				t.Errorf("%s: photo id %q must be a unique 11-char Unsplash id", p.SKU, p.Photo)
			}
			photos[p.Photo] = true
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
		}
	}
	if sets == 0 {
		t.Error("catalog needs at least one set for the fitting room")
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
	pic := jpegBytes(t)
	failing := map[string]bool{Catalog[0].Products[0].Photo: true}
	fetch := func(_ context.Context, id string) ([]byte, error) {
		if failing[id] {
			return nil, errors.New("network down")
		}
		return pic, nil
	}
	total := 0
	for _, s := range Catalog {
		total += len(s.Products)
	}

	if _, err := Seed(ctx, pool, store, Options{Password: "short", Fetch: fetch}); err == nil {
		t.Fatal("short password must be rejected")
	}
	res, err := Seed(ctx, pool, store, Options{Password: "demo-password-1", Fetch: fetch})
	if err != nil {
		t.Fatal(err)
	}
	if res.StoresCreated != len(Catalog) || res.ProductsCreated != total || res.PhotosAdded != total-1 || len(res.PhotoFailures) != 1 {
		t.Fatalf("first run: %+v", res)
	}
	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM products WHERE status = 'published'`); n != total-1 {
		t.Fatalf("published = %d, want %d", n, total-1)
	}
	if n := count(`SELECT count(*) FROM products WHERE status = 'draft' AND sku = $1`, Catalog[0].Products[0].SKU); n != 1 {
		t.Fatal("product without photo must stay a draft")
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
	// ключи фото принадлежат учётной записи магазина, иначе кабинет не сохранит товар
	rows, err := pool.Query(ctx, `SELECT u.id, i.key FROM product_images i JOIN products p ON p.id = i.product_id JOIN users u ON u.store_id = p.store_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var uid [16]byte
		var key string
		if err := rows.Scan(&uid, &key); err != nil {
			t.Fatal(err)
		}
		if !media.OwnsKey(uid, media.PurposeProductImage, key) {
			t.Fatalf("key %s is not owned by the store account", key)
		}
		if _, ok := store.Objs[key]; !ok {
			t.Fatalf("file %s not uploaded", key)
		}
	}
	rows.Close()
	var hash string
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE login = $1`, Catalog[1].Login).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, _ := auth.VerifyPassword("demo-password-1", hash); !ok {
		t.Fatal("store password does not match")
	}

	// второй запуск: ничего не дублирует, докачивает недостающее фото
	delete(failing, Catalog[0].Products[0].Photo)
	res, err = Seed(ctx, pool, store, Options{Password: "another-password", Fetch: fetch})
	if err != nil {
		t.Fatal(err)
	}
	if res.StoresCreated != 0 || res.ProductsCreated != 0 || res.PhotosAdded != 1 || len(res.PhotoFailures) != 0 {
		t.Fatalf("second run: %+v", res)
	}
	if n := count(`SELECT count(*) FROM products WHERE status = 'published'`); n != total {
		t.Fatalf("published after rerun = %d, want %d", n, total)
	}
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM users WHERE login = $1`, Catalog[1].Login).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if ok, _ := auth.VerifyPassword("demo-password-1", hash); !ok {
		t.Fatal("rerun must not change an existing password")
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
