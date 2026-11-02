package server

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/admin"
	"github.com/boutique-market/backend/internal/media"
	ordersPkg "github.com/boutique-market/backend/internal/orders"
)

// doH — запрос с дополнительными заголовками (отдельный http.Client с той же cookie-jar,
// чтобы параллельные вызовы не гонялись за общий транспорт).
func (c *client) doH(method, path string, body any, hdr map[string]string) resp {
	c.e.t.Helper()
	cc := *c
	cc.http = &http.Client{Jar: c.http.Jar, Transport: headerRT{base: http.DefaultTransport, hdr: hdr}}
	return cc.do(method, path, body)
}

type headerRT struct {
	base http.RoundTripper
	hdr  map[string]string
}

func (h headerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	for k, v := range h.hdr {
		r.Header.Set(k, v)
	}
	return h.base.RoundTrip(r)
}

type orderFixture struct {
	e                 *env
	adm               *client
	storeA, storeB    *client
	storeAID, storeB2 string
	prodA, prodA2     string
	prodB             string
}

func newOrderFixture(t *testing.T) *orderFixture {
	e := setup(t)
	ctx := context.Background()
	if _, err := admin.CreateAdmin(ctx, e.d.DB, "root", "admin-password-123"); err != nil {
		t.Fatal(err)
	}
	f := &orderFixture{e: e, adm: e.client()}
	f.adm.token = f.adm.must(200, "POST", "/v1/auth/login", map[string]any{"login": "root", "password": "admin-password-123"}).data()["access_token"].(string)
	mkStore := func(slug string) (*client, string) {
		id := f.adm.must(201, "POST", "/v1/admin/stores", map[string]any{"slug": slug, "name": "Бутик " + slug, "login": slug, "password": "store-password-1"}).data()["store_id"].(string)
		f.adm.must(200, "POST", "/v1/admin/stores/"+id+"/status", map[string]any{"status": "active"})
		c := e.client()
		c.token = c.must(200, "POST", "/v1/auth/login", map[string]any{"login": slug, "password": "store-password-1"}).data()["access_token"].(string)
		return c, id
	}
	f.storeA, f.storeAID = mkStore("alpha")
	f.storeB, f.storeB2 = mkStore("beta")
	var catID uuid.UUID
	_ = e.d.DB.QueryRow(ctx, `SELECT id FROM categories LIMIT 1`).Scan(&catID)
	mkProd := func(storeID string, price int64, inStock bool) string {
		var id uuid.UUID
		err := e.d.DB.QueryRow(ctx, `INSERT INTO products (store_id, category_id, sku, title, description, brand, price, in_stock, status)
			VALUES ($1, $2, $3, 'Платье', 'D', 'Acme', $4, $5, 'published') RETURNING id`, storeID, catID, uuid.NewString(), price, inStock).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = e.d.DB.Exec(ctx, `INSERT INTO product_images (product_id, angle, key) VALUES ($1, 'front', $2)`, id, "p/"+id.String()+".jpg")
		return id.String()
	}
	f.prodA = mkProd(f.storeAID, 100000, true)
	f.prodA2 = mkProd(f.storeAID, 50000, true)
	f.prodB = mkProd(f.storeB2, 70000, true)
	return f
}

func checkoutBody(items ...map[string]any) map[string]any {
	return map[string]any{
		"items":                 items,
		"customer_name":         "Азиз <b>Каримов</b>",
		"customer_comment":      "Позвоните за час",
		"delivery_time_comment": "после 18:00",
		"delivery": map[string]any{
			"street": "ул. Навои", "house": "5А", "entrance": "2", "floor": "7", "apartment": "41",
			"address_comment": "вход со двора", "lat": 41.311081, "lng": 69.240562,
		},
	}
}

func item(id string, qty int) map[string]any { return map[string]any{"product_id": id, "qty": qty} }

func idem(k string) map[string]string { return map[string]string{"Idempotency-Key": k} }

func TestCheckoutSplitsByStoreAndIsIdempotent(t *testing.T) {
	f := newOrderFixture(t)
	c := f.e.client()
	c.loginClient("+998901110000")

	body := checkoutBody(item(f.prodA, 2), item(f.prodB, 1), item(f.prodA2, 1))
	if r := c.do("POST", "/v1/orders", body); r.Status != 400 || r.errCode() != "idempotency_key_required" {
		t.Fatalf("missing key: %d %s", r.Status, r.Raw)
	}
	r1 := c.doH("POST", "/v1/orders", body, idem("checkout-0001"))
	if r1.Status != 201 {
		t.Fatalf("checkout: %d %s", r1.Status, r1.Raw)
	}
	orders := r1.data()["orders"].([]any)
	if len(orders) != 2 {
		t.Fatalf("want 2 orders (per store), got %d", len(orders))
	}
	a := orders[0].(map[string]any)
	if a["store_id"] != f.storeAID || a["items_total"].(float64) != 250000 || a["status"] != "pending" {
		t.Fatalf("store A order: %v", a)
	}
	// повтор с тем же ключом => тот же ответ, без новых заказов
	r2 := c.doH("POST", "/v1/orders", body, idem("checkout-0001"))
	if r2.Status != 201 || !reflect.DeepEqual(r2.Body, r1.Body) {
		t.Fatalf("replay differs: %d %s", r2.Status, r2.Raw)
	}
	// тот же ключ с другим телом => ошибка
	other := checkoutBody(item(f.prodB, 3))
	if r := c.doH("POST", "/v1/orders", other, idem("checkout-0001")); r.errCode() != "idempotency_key_reused" {
		t.Fatalf("key reuse: %s", r.Raw)
	}
	var n int
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM orders`).Scan(&n)
	if n != 2 {
		t.Fatalf("orders in db: %d", n)
	}
	// параллельные дубли — заказ создаётся один раз
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.doH("POST", "/v1/orders", checkoutBody(item(f.prodB, 1)), idem("parallel-key-1"))
		}()
	}
	wg.Wait()
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM orders`).Scan(&n)
	if n != 3 {
		t.Fatalf("parallel duplicates created orders: %d", n)
	}

	list := c.must(200, "GET", "/v1/orders", nil).Body["data"].([]any)
	if len(list) != 3 {
		t.Fatalf("client list: %d", len(list))
	}
}

func TestCheckoutValidation(t *testing.T) {
	f := newOrderFixture(t)
	c := f.e.client()
	c.loginClient("+998901110001")
	ctx := context.Background()

	bad := checkoutBody(item(f.prodA, 1))
	bad["delivery"].(map[string]any)["house"] = ""
	bad["delivery"].(map[string]any)["lat"] = 500.0
	bad["customer_comment"] = strings.Repeat("x", 1001)
	r := c.doH("POST", "/v1/orders", bad, idem("validation-1"))
	if r.Status != 422 {
		t.Fatalf("validation: %d %s", r.Status, r.Raw)
	}
	fields := r.Body["error"].(map[string]any)["fields"].(map[string]any)
	for _, k := range []string{"delivery.house", "delivery.location", "customer_comment"} {
		if fields[k] == nil {
			t.Errorf("missing field error %s: %v", k, fields)
		}
	}
	// нет в наличии / снят с публикации
	_, _ = f.e.d.DB.Exec(ctx, `UPDATE products SET in_stock = false WHERE id = $1`, f.prodA)
	_, _ = f.e.d.DB.Exec(ctx, `UPDATE products SET status = 'blocked' WHERE id = $1`, f.prodB)
	r = c.doH("POST", "/v1/orders", checkoutBody(item(f.prodA, 1), item(f.prodB, 1), item(f.prodA2, 1)), idem("stock-check-1"))
	if r.Status != 409 || r.errCode() != "products_unavailable" {
		t.Fatalf("unavailable: %d %s", r.Status, r.Raw)
	}
	fields = r.Body["error"].(map[string]any)["fields"].(map[string]any)
	if fields[f.prodA] != "out_of_stock" || fields[f.prodB] != "unavailable" || fields[f.prodA2] != nil {
		t.Fatalf("unavailable fields: %v", fields)
	}
	// чужой ключ фото не принимается
	body := checkoutBody(item(f.prodA2, 1))
	body["delivery"].(map[string]any)["photo_key"] = media.NewKey(uuid.New(), media.PurposeDeliveryPhoto)
	if r := c.doH("POST", "/v1/orders", body, idem("foreign-photo")); r.Status != 422 {
		t.Fatalf("foreign photo key: %d %s", r.Status, r.Raw)
	}
	// своё фото, но загруженное для примерки, тоже нельзя
	bodyKey := c.must(201, "POST", "/v1/uploads?purpose=body", pngFile(t)).data()["key"].(string)
	body["delivery"].(map[string]any)["photo_key"] = bodyKey
	if r := c.doH("POST", "/v1/orders", body, idem("body-photo-key")); r.Status != 422 {
		t.Fatalf("body photo as delivery photo: %d %s", r.Status, r.Raw)
	}
	// неизвестные поля (например, status) отклоняются
	body = checkoutBody(item(f.prodA2, 1))
	body["status"] = "delivered"
	if r := c.doH("POST", "/v1/orders", body, idem("mass-assign")); r.Status != 400 {
		t.Fatalf("mass assignment: %d %s", r.Status, r.Raw)
	}
	var n int
	_ = f.e.d.DB.QueryRow(ctx, `SELECT count(*) FROM orders`).Scan(&n)
	if n != 0 {
		t.Fatalf("invalid checkouts created %d orders", n)
	}
}

func TestDeliveryWindowAccessAndStatuses(t *testing.T) {
	f := newOrderFixture(t)
	c := f.e.client()
	c.loginClient("+998901110002")
	stranger := f.e.client()
	stranger.loginClient("+998901110003")

	photo := c.must(201, "POST", "/v1/uploads?purpose=delivery", pngFile(t)).data()["key"].(string)
	body := checkoutBody(item(f.prodA, 1), item(f.prodB, 1))
	body["delivery"].(map[string]any)["photo_key"] = photo
	orders := c.mustH(201, "POST", "/v1/orders", body, idem("window-0001")).data()["orders"].([]any)
	oa := orders[0].(map[string]any)["id"].(string)
	ob := orders[1].(map[string]any)["id"].(string)

	// окно доставки магазина A
	win := f.storeA.must(200, "GET", "/v1/store/orders/"+oa, nil).data()
	d := win["delivery"].(map[string]any)
	if d["street"] != "ул. Навои" || d["entrance"] != "2" || d["floor"] != "7" || d["has_photo"] != true || d["photo_url"] == "" {
		t.Fatalf("delivery window: %v", d)
	}
	if !strings.Contains(d["maps_url"].(string), "41.311081,69.240562") {
		t.Fatalf("maps url: %v", d["maps_url"])
	}
	copyText := win["copy_text"].(string)
	for _, want := range []string{"Адрес: ул. Навои, дом 5А", "Подъезд: 2", "Этаж: 7", "Время доставки: после 18:00", "Телефон: +998901110002"} {
		if !strings.Contains(copyText, want) {
			t.Errorf("copy text missing %q:\n%s", want, copyText)
		}
	}
	if win["customer_comment"] != "Позвоните за час" {
		t.Fatalf("comment: %v", win["customer_comment"])
	}

	// IDOR: чужой магазин, чужой клиент, клиент через store-роут
	f.storeB.must(404, "GET", "/v1/store/orders/"+oa, nil)
	f.storeB.must(404, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": "confirmed"})
	stranger.must(404, "GET", "/v1/orders/"+oa, nil)
	c.must(403, "GET", "/v1/store/orders/"+oa, nil)
	c.must(403, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": "confirmed"})
	f.storeA.must(403, "GET", "/v1/orders/"+oa, nil)
	f.storeA.must(404, "GET", "/v1/store/orders/not-a-uuid", nil)
	if n := len(f.storeB.must(200, "GET", "/v1/store/orders", nil).Body["data"].([]any)); n != 1 {
		t.Fatalf("store B must see only its order, got %d", n)
	}

	// переходы статусов
	f.storeA.must(409, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": "delivered"})
	f.storeA.must(422, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": "rejected"})
	f.storeA.must(200, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": "confirmed", "from": "pending"})
	if r := f.storeA.do("POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": "assembling", "from": "pending"}); r.errCode() != "status_changed" {
		t.Fatalf("stale from: %s", r.Raw)
	}
	for _, s := range []string{"assembling", "in_delivery", "delivered"} {
		r := f.storeA.must(200, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": s})
		if r.data()["status"] != s {
			t.Fatalf("status %s: %v", s, r.data()["status"])
		}
	}
	f.storeA.must(409, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": "assembling"})
	// магазин B отклоняет свой заказ с причиной
	f.storeB.must(200, "POST", "/v1/store/orders/"+ob+"/status", map[string]any{"status": "rejected", "reason": "Нет нужного размера"})

	// клиент видит статусы и историю
	got := c.must(200, "GET", "/v1/orders/"+oa, nil).data()
	if got["status"] != "delivered" || got["step"].(float64) != 4 {
		t.Fatalf("client status: %v %v", got["status"], got["step"])
	}
	hist := got["history"].([]any)
	if len(hist) != 5 {
		t.Fatalf("history len %d", len(hist))
	}
	if _, ok := hist[1].(map[string]any)["actor_role"]; ok {
		t.Error("actor role must be hidden from client")
	}
	if got["copy_text"] != nil || got["next_statuses"] != nil {
		t.Error("client must not get store-only fields")
	}
	rb := c.must(200, "GET", "/v1/orders/"+ob, nil).data()
	if rb["status"] != "rejected" || rb["rejection_reason"] != "Нет нужного размера" {
		t.Fatalf("rejected order: %v", rb)
	}
	// фильтр списка магазина по статусу
	if n := len(f.storeA.must(200, "GET", "/v1/store/orders?status=delivered", nil).Body["data"].([]any)); n != 1 {
		t.Fatalf("filter delivered: %d", n)
	}
	f.storeA.must(400, "GET", "/v1/store/orders?status=bogus", nil)

	// админ видит любой заказ; аудит записан
	f.adm.must(200, "GET", "/v1/admin/orders/"+ob, nil)
	var n int
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'order.status'`).Scan(&n)
	if n != 5 {
		t.Fatalf("audit entries: %d", n)
	}
}

func (c *client) mustH(status int, method, path string, body any, hdr map[string]string) resp {
	c.e.t.Helper()
	r := c.doH(method, path, body, hdr)
	if r.Status != status {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, r.Status, status, r.Raw)
	}
	return r
}

func TestDeliveryPhotoRetention(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	c := f.e.client()
	c.loginClient("+998901110009")
	photo := c.must(201, "POST", "/v1/uploads?purpose=delivery", pngFile(t)).data()["key"].(string)
	body := checkoutBody(item(f.prodA, 1), item(f.prodB, 1))
	body["delivery"].(map[string]any)["photo_key"] = photo
	orders := c.mustH(201, "POST", "/v1/orders", body, idem("retention-01")).data()["orders"].([]any)
	oa := orders[0].(map[string]any)["id"].(string)
	ob := orders[1].(map[string]any)["id"].(string)
	store := f.e.d.Storage.(*media.Memory)
	if _, ok := store.Objs[photo]; !ok {
		t.Fatal("photo not stored")
	}
	purge := func() int64 {
		n, err := ordersPkg.PurgeDeliveryPhotos(ctx, f.e.d.DB, store, ordersPkg.PhotoRetention, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	age := func(id string, days int) {
		if _, err := f.e.d.DB.Exec(ctx, `ALTER TABLE orders DISABLE TRIGGER orders_touch`); err != nil {
			t.Fatal(err)
		}
		_, _ = f.e.d.DB.Exec(ctx, `UPDATE orders SET updated_at = now() - make_interval(days => $2) WHERE id = $1`, id, days)
		_, _ = f.e.d.DB.Exec(ctx, `ALTER TABLE orders ENABLE TRIGGER orders_touch`)
	}
	// заказ A доставлен 31 день назад, но заказ B того же оформления ещё в работе — фото нужно B
	for _, s := range []string{"confirmed", "assembling", "in_delivery", "delivered"} {
		f.storeA.must(200, "POST", "/v1/store/orders/"+oa+"/status", map[string]any{"status": s})
	}
	age(oa, 31)
	if n := purge(); n != 0 {
		t.Fatalf("purged while another order still needs the photo: %d", n)
	}
	// B отклонён 29 дней назад — срок ещё не вышел
	f.storeB.must(200, "POST", "/v1/store/orders/"+ob+"/status", map[string]any{"status": "rejected", "reason": "нет размера"})
	age(ob, 29)
	if n := purge(); n != 0 {
		t.Fatalf("purged before retention: %d", n)
	}
	age(ob, 31)
	if n := purge(); n != 2 {
		t.Fatalf("want 2 rows cleared, got %d", n)
	}
	if _, ok := store.Objs[photo]; ok {
		t.Fatal("photo object must be deleted")
	}
	d := f.storeA.must(200, "GET", "/v1/store/orders/"+oa, nil).data()["delivery"].(map[string]any)
	if d["has_photo"] != false || d["photo_url"] != nil {
		t.Fatalf("photo still referenced: %v", d)
	}
}
