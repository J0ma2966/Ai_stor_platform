package demoseed

import (
	"context"
	"testing"

	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/testutil"
)

func TestRemoveDeletesOnlyDemoStores(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	store := media.NewMemory()
	mk := func(slug, cover string) {
		t.Helper()
		var id [16]byte
		if err := pool.QueryRow(ctx, `INSERT INTO stores (slug, name, status, cover_key) VALUES ($1, $1, 'active', $2) RETURNING id`, slug, cover).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO users (role, login, password_hash, store_id) VALUES ('store', $1, 'x', $2)`, slug, id); err != nil {
			t.Fatal(err)
		}
		store.Objs[cover] = []byte("img")
	}
	mk("demo-denim-lab", "covers/demo.jpg")
	mk("real-boutique", "covers/real.jpg")

	res, err := Remove(ctx, pool, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.StoresDeleted != 1 || len(res.StoresArchived) != 0 {
		t.Fatalf("result: %+v", res)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM stores WHERE slug LIKE 'demo-%'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("demo stores left: %d (%v)", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM stores s JOIN users u ON u.store_id = s.id WHERE s.slug = 'real-boutique'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("real store must stay: %d (%v)", n, err)
	}
	if _, ok := store.Objs["covers/demo.jpg"]; ok {
		t.Fatal("demo cover file must be deleted")
	}
	if _, ok := store.Objs["covers/real.jpg"]; !ok {
		t.Fatal("real store file must stay")
	}
	// повторный запуск ничего не ломает
	if res, err := Remove(ctx, pool, store, nil); err != nil || res.StoresDeleted != 0 {
		t.Fatalf("second run: %+v %v", res, err)
	}
}
