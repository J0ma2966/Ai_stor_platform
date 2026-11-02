package orders

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/testutil"
)

func TestPurgeOrphanDeliveryPhotos(t *testing.T) {
	pool := testutil.DB(t)
	ctx := context.Background()
	store := media.NewMemory()
	owner := uuid.New()
	old := time.Now().Add(-48 * time.Hour)

	put := func(p media.Purpose, age time.Time) string {
		k := media.NewKey(owner, p)
		_ = store.Put(ctx, k, []byte("x"), "image/jpeg")
		store.Mod[k] = age
		return k
	}
	orphan := put(media.PurposeDeliveryPhoto, old)
	fresh := put(media.PurposeDeliveryPhoto, time.Now()) // клиент ещё оформляет
	attached := put(media.PurposeDeliveryPhoto, old)     // прикреплено к заказу
	body := put(media.PurposeBodyPhoto, old)             // не наша зона
	product := put(media.PurposeProductImage, old)       // публичное, не трогаем

	var uid, sid uuid.UUID
	_ = pool.QueryRow(ctx, `INSERT INTO users (role, phone) VALUES ('client', '+998907770000') RETURNING id`).Scan(&uid)
	_ = pool.QueryRow(ctx, `INSERT INTO stores (slug, name) VALUES ('orph', 'O') RETURNING id`).Scan(&sid)
	var oid uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO orders (checkout_id, user_id, store_id, items_total, customer_name, customer_phone)
		VALUES (gen_random_uuid(), $1, $2, 100, 'A', '+998907770000') RETURNING id`, uid, sid).Scan(&oid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO order_deliveries (order_id, street, house, photo_key) VALUES ($1, 'S', '1', $2)`, oid, attached); err != nil {
		t.Fatal(err)
	}

	n, err := PurgeOrphanDeliveryPhotos(ctx, pool, store, time.Now())
	if err != nil || n != 1 {
		t.Fatalf("purged %d, err %v; want 1", n, err)
	}
	if _, ok := store.Objs[orphan]; ok {
		t.Fatal("orphan not deleted")
	}
	for _, k := range []string{fresh, attached, body, product} {
		if _, ok := store.Objs[k]; !ok {
			t.Fatalf("deleted %s which must stay", k)
		}
	}
}
