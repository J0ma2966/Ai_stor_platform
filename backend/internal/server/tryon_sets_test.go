package server

import (
	"context"
	"image/color"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/tryon"
)

// «Двойка» (products.is_set): верх и низ на одном фото. Магазин ставит флаг только
// у товаров зоны «верх»; в примерке двойка занимает зоны top и bottom.
func TestTryOnTwoPieceSet(t *testing.T) {
	f := newTryonFixture(t)
	ctx := context.Background()
	mk := func(zone string) string {
		var id uuid.UUID
		if err := f.e.d.DB.QueryRow(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price, status)
			VALUES ($1, (SELECT id FROM categories WHERE zone = $2 LIMIT 1), $3, 'Вещь', 'D', 'Acme', 100000, 'published')
			RETURNING id`, f.storeAID, zone, uuid.NewString()).Scan(&id); err != nil {
			t.Fatal(err)
		}
		key := "pub/u/" + uuid.NewString() + "/product/" + strings.Repeat("b", 32) + ".jpg"
		_ = f.mem.Put(ctx, key, jpegOf(t, 40, 60, color.RGBA{0, 0, 200, 255}), "image/jpeg")
		if _, err := f.e.d.DB.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, 'front', $2)`, id, key); err != nil {
			t.Fatal(err)
		}
		return id.String()
	}
	bottom := mk("bottom")
	set := f.top2

	// флаг «двойка» — только для категорий зоны «верх»
	if r := f.storeA.do("PATCH", "/v1/store/products/"+bottom, map[string]any{"is_set": true}); r.Status != 422 || !strings.Contains(r.Raw, "top_only") {
		t.Fatalf("set on bottom must be rejected: %d %s", r.Status, r.Raw)
	}
	if d := f.storeA.must(200, "PATCH", "/v1/store/products/"+set, map[string]any{"is_set": true}).data(); d["is_set"] != true {
		t.Fatalf("store update: %v", d)
	}
	if d := f.cli.must(200, "GET", "/v1/products/"+set, nil).data(); d["is_set"] != true || d["zone"] != "top" {
		t.Fatalf("public card: %v", d)
	}
	// в каталоге комплект — обычный товар списка, с флагом
	found := false
	for _, it := range f.cli.must(200, "GET", "/v1/products?limit=60", nil).Body["data"].([]any) {
		if m := it.(map[string]any); m["id"] == set {
			found = m["is_set"] == true
		}
	}
	if !found {
		t.Fatal("list must carry is_set")
	}

	// двойка закрывает низ: вместе с брюками — zone_taken
	f.cli.must(204, "POST", "/v1/tryon/consent", nil)
	if r := f.cli.doH("POST", "/v1/tryon/generations", f.body(set, bottom), idem("gen-set-0001")); r.Status != 422 || !strings.Contains(r.Raw, "zone_taken") {
		t.Fatalf("set + bottom must be rejected: %d %s", r.Status, r.Raw)
	}
	if r := f.cli.doH("POST", "/v1/tryon/generations", f.body(bottom, set), idem("gen-set-0002")); r.Status != 422 || !strings.Contains(r.Raw, "zone_taken") {
		t.Fatalf("bottom + set must be rejected: %d %s", r.Status, r.Raw)
	}
	var mu sync.Mutex
	var seen []tryon.Garment
	f.providerFail = func(r tryon.Request) error {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, r.Garments...)
		return nil
	}
	id := f.create(t, "gen-set-0003", f.hat, set)
	f.runOnce(t)
	var sets int
	for _, g := range seen {
		if g.Set {
			sets++
			if g.Zone != tryon.ZoneTop {
				t.Fatalf("set garment zone = %q", g.Zone)
			}
		}
	}
	if sets != len(tryon.Angles) {
		t.Fatalf("provider must get the set flag on every angle: %+v", seen)
	}
	g := f.cli.must(200, "GET", "/v1/tryon/generations/"+id, nil).data()
	found = false
	for _, p := range g["products"].([]any) {
		pm := p.(map[string]any)
		if pm["id"] == set {
			found = pm["is_set"] == true
		}
	}
	if g["status"] != "succeeded" || !found {
		t.Fatalf("generation: %v", g)
	}

	// перенос в категорию «низ» сбрасывает флаг
	var bottomsCat string
	_ = f.e.d.DB.QueryRow(ctx, `SELECT id::text FROM categories WHERE zone = 'bottom' LIMIT 1`).Scan(&bottomsCat)
	if d := f.storeA.must(200, "PATCH", "/v1/store/products/"+set, map[string]any{"category_id": bottomsCat}).data(); d["is_set"] != false {
		t.Fatalf("moving to bottoms must clear is_set: %v", d)
	}
}
