package products

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/testutil"
)

func TestCardImageSQLMatchesVariantKey(t *testing.T) {
	// CardImageSQL собирает ключ миниатюры в SQL; он должен совпадать с media.VariantKey
	if want := media.VariantKey("pub/x.jpg", media.VariantCard); want != "pub/x_800.jpg" {
		t.Fatalf("VariantKey changed (%s): update CardImageSQL", want)
	}
}

func TestVariantsBuildsAndResets(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	store := media.NewMemory()
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2000, 2000)), nil)
	owner := uuid.New()
	good, missing := media.NewKey(owner, media.PurposeProductImage), media.NewKey(owner, media.PurposeProductImage)
	_ = store.Put(ctx, good, b.Bytes(), "image/jpeg")

	var storeID, catID, pid uuid.UUID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO stores (slug, name, status) VALUES ('v-store', 'V', 'active') RETURNING id`).Scan(&storeID))
	must(pool.QueryRow(ctx, `SELECT id FROM categories LIMIT 1`).Scan(&catID))
	must(pool.QueryRow(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price)
		VALUES ($1, $2, 'S1', 'T', 'D', 'B', 100) RETURNING id`, storeID, catID).Scan(&pid))
	_, err := pool.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, 'front', $2), ($1, 'side', $3)`, pid, good, missing)
	must(err)

	v := &Variants{DB: pool, Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	n, err := v.RunOnce(ctx)
	must(err)
	if n != 2 {
		t.Fatalf("processed %d, want 2", n)
	}
	state := func(angle string) (s string) {
		must(pool.QueryRow(ctx, `SELECT variants FROM product_images WHERE product_id = $1 AND angle = $2`, pid, angle).Scan(&s))
		return s
	}
	if state("front") != "ready" || state("side") != "failed" {
		t.Fatalf("states front=%s side=%s", state("front"), state("side"))
	}
	for _, side := range media.ProductVariants {
		if _, err := store.Get(ctx, media.VariantKey(good, side)); err != nil {
			t.Fatalf("variant %d not stored: %v", side, err)
		}
	}
	// карточка корзины берёт копию 800 px, когда она готова
	var card string
	must(pool.QueryRow(ctx, `SELECT `+CardImageSQL+` FROM products p WHERE p.id = $1`, pid).Scan(&card))
	if card != media.VariantKey(good, media.VariantCard) {
		t.Fatalf("card image %q", card)
	}
	// замена фото снова ставит его в очередь
	_, err = pool.Exec(ctx, `UPDATE product_images SET key = $2 WHERE product_id = $1 AND angle = 'front'`, pid, media.NewKey(owner, media.PurposeProductImage))
	must(err)
	if state("front") != "pending" {
		t.Fatal("key change must reset variants to pending")
	}
}
