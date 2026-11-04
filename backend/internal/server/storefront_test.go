package server

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"github.com/boutique-market/backend/internal/media"
	"github.com/boutique-market/backend/internal/products"
)

type memHas struct{ s media.Storage }

func (m memHas) Has(key string) bool {
	found := false
	_ = m.s.List(context.Background(), key, time.Now().Add(time.Hour), func(k string) error {
		found = found || k == key
		return nil
	})
	return found
}

func listAny(r resp) []any { l, _ := r.Body["data"].([]any); return l }

func uploadKeys(t *testing.T, c *client, purpose string, n int) []string {
	t.Helper()
	keys := make([]string, n)
	for i := range keys {
		keys[i] = c.must(201, "POST", "/v1/uploads?purpose="+purpose, pngFile(t)).data()["key"].(string)
	}
	return keys
}

func TestPublicStoresAndStoreProfile(t *testing.T) {
	f := newOrderFixture(t)
	anon := f.e.client()

	stores := listAny(anon.must(200, "GET", "/v1/stores", nil))
	if len(stores) != 2 {
		t.Fatalf("stores: %v", stores)
	}
	if _, leaked := stores[0].(map[string]any)["status"]; leaked {
		t.Fatal("public store exposes status")
	}
	anon.must(404, "GET", "/v1/stores/nope", nil)

	// логотип: только свой ключ purpose=store
	foreign := uploadKeys(t, f.storeB, "store", 1)[0]
	if r := f.storeA.do("PATCH", "/v1/store/profile", map[string]any{"logo_key": foreign}); r.Status != 422 {
		t.Fatalf("foreign logo accepted: %s", r.Raw)
	}
	productKey := uploadKeys(t, f.storeA, "product", 1)[0]
	if r := f.storeA.do("PATCH", "/v1/store/profile", map[string]any{"logo_key": productKey}); r.Status != 422 {
		t.Fatalf("product key accepted as logo: %s", r.Raw)
	}
	logo := uploadKeys(t, f.storeA, "store", 1)[0]
	prof := f.storeA.must(200, "PATCH", "/v1/store/profile", map[string]any{
		"name": "Альфа", "description": "Лучший бутик", "logo_key": logo, "phone": "+998901112233",
		"contacts": map[string]any{"instagram": "alpha_uz", "website": "https://alpha.uz"},
		"address":  "Ташкент, ул. Навои 1", "lat": 41.3, "lng": 69.2,
	}).data()
	if prof["name"] != "Альфа" || !strings.HasPrefix(prof["logo_url"].(string), "https://cdn.test/pub/") {
		t.Fatalf("profile: %v", prof)
	}
	f.storeA.must(422, "PATCH", "/v1/store/profile", map[string]any{"lat": 41.3})
	f.storeA.must(400, "PATCH", "/v1/store/profile", map[string]any{"status": "active"}) // нельзя сменить статус

	pub := anon.must(200, "GET", "/v1/stores/alpha", nil).data()
	if pub["name"] != "Альфа" || pub["product_count"].(float64) != 2 {
		t.Fatalf("public store: %v", pub)
	}
	if listAny(anon.must(200, "GET", "/v1/stores?q=альф", nil)) == nil {
		t.Fatal("search by name")
	}
	// каталог: магазин и зона в карточке
	card := anon.must(200, "GET", "/v1/products/"+f.prodA, nil).data()
	if card["store_slug"] != "alpha" || card["zone"] == "" {
		t.Fatalf("card: %v", card)
	}
}

func TestStoreProductEditDeleteAndPublishRules(t *testing.T) {
	f := newOrderFixture(t)
	cats := listAny(f.storeA.must(200, "GET", "/v1/categories", nil))
	cat := cats[1].(map[string]any)["id"].(string)

	// товар без фото нельзя опубликовать
	var pid uuid.UUID
	_ = f.e.d.DB.QueryRow(context.Background(), `INSERT INTO products (store_id, category_id, sku, title, description, brand, price)
		VALUES ($1, $2, 'NOIMG', 'T', 'D', 'B', 100) RETURNING id`, f.storeAID, cat).Scan(&pid)
	if r := f.storeA.do("PATCH", "/v1/store/products/"+pid.String()+"/status", map[string]any{"status": "published"}); r.errCode() != "images_required" {
		t.Fatalf("publish without images: %s", r.Raw)
	}
	keys := uploadKeys(t, f.storeA, "product", 3)
	upd := f.storeA.must(200, "PATCH", "/v1/store/products/"+pid.String(), map[string]any{
		"title": "Новое", "price": 990000, "category_id": cat, "in_stock": false,
		"images": map[string]string{"front": keys[0], "side": keys[1], "back": keys[2]},
	}).data()
	if upd["title"] != "Новое" || upd["price"].(float64) != 990000 || upd["in_stock"] != false || len(upd["images"].(map[string]any)) != 3 {
		t.Fatalf("update: %v", upd)
	}
	f.storeA.must(200, "PATCH", "/v1/store/products/"+pid.String()+"/status", map[string]any{"status": "published"})

	// чужой магазин: 404 на всё
	f.storeB.must(404, "GET", "/v1/store/products/"+pid.String(), nil)
	f.storeB.must(404, "PATCH", "/v1/store/products/"+pid.String(), map[string]any{"title": "x"})
	f.storeB.must(404, "DELETE", "/v1/store/products/"+pid.String(), nil)
	f.storeB.must(404, "PATCH", "/v1/store/products/"+pid.String()+"/status", map[string]any{"status": "draft"})
	// чужие ключи фото
	bk := uploadKeys(t, f.storeB, "product", 1)[0]
	f.storeA.must(422, "PATCH", "/v1/store/products/"+pid.String(), map[string]any{"images": map[string]string{"front": bk}})
	// дубликат артикула
	if r := f.storeA.do("PATCH", "/v1/store/products/"+pid.String(), map[string]any{"sku": "NOIMG"}); r.Status != 200 {
		t.Fatalf("same sku on itself: %s", r.Raw)
	}

	// замена фото удаляет старый файл из хранилища
	mem := memHas{f.e.d.Storage}
	newFront := uploadKeys(t, f.storeA, "product", 1)[0]
	f.storeA.must(200, "PATCH", "/v1/store/products/"+pid.String(), map[string]any{"images": map[string]string{"front": newFront}})
	if mem.Has(keys[0]) {
		t.Fatal("replaced image not deleted")
	}
	// удаление: товар исчезает, файлы тоже
	f.storeA.must(204, "DELETE", "/v1/store/products/"+pid.String(), nil)
	f.storeA.must(404, "GET", "/v1/store/products/"+pid.String(), nil)
	if mem.Has(newFront) || mem.Has(keys[1]) {
		t.Fatal("images of deleted product left in storage")
	}
}

func TestCartAndFavorites(t *testing.T) {
	f := newOrderFixture(t)
	cl := f.e.client()
	cl.loginClient("+998901234500")

	f.storeA.must(403, "GET", "/v1/me/cart", nil) // корзина только у клиента
	cl.must(422, "PUT", "/v1/me/cart/"+f.prodA, map[string]any{"qty": 0})
	cl.must(422, "PUT", "/v1/me/cart/"+f.prodA, map[string]any{"qty": 100})
	cl.must(404, "PUT", "/v1/me/cart/"+uuid.NewString(), map[string]any{"qty": 1})
	cl.must(200, "PUT", "/v1/me/cart/"+f.prodA, map[string]any{"qty": 2})
	cart := cl.must(200, "PUT", "/v1/me/cart/"+f.prodB, map[string]any{"qty": 1}).data()
	if cart["total"].(float64) != 2*100000+70000 || cart["can_checkout"] != true || len(cart["items"].([]any)) != 2 {
		t.Fatalf("cart: %v", cart)
	}
	// товар закончился → нельзя оформить, позиция помечена
	_, _ = f.e.d.DB.Exec(context.Background(), `UPDATE products SET in_stock = false WHERE id = $1`, f.prodB)
	cart = cl.must(200, "GET", "/v1/me/cart", nil).data()
	if cart["can_checkout"] != false || cart["has_unavailable"] != true || cart["total"].(float64) != 200000 {
		t.Fatalf("cart with out-of-stock: %v", cart)
	}
	if r := cl.do("PUT", "/v1/me/cart/"+f.prodB, map[string]any{"qty": 3}); r.errCode() != "out_of_stock" {
		t.Fatalf("add out-of-stock: %s", r.Raw)
	}
	cl.must(200, "DELETE", "/v1/me/cart/"+f.prodB, nil)
	// оформление убирает оформленные позиции из корзины
	cl.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 2)), idem(uuid.NewString()))
	if n := len(cl.must(200, "GET", "/v1/me/cart", nil).data()["items"].([]any)); n != 0 {
		t.Fatalf("cart not cleared after checkout: %d", n)
	}

	// избранное
	cl.must(204, "PUT", "/v1/me/favorites/"+f.prodA, nil)
	cl.must(204, "PUT", "/v1/me/favorites/"+f.prodA, nil) // идемпотентно
	cl.must(404, "PUT", "/v1/me/favorites/"+uuid.NewString(), nil)
	ids := cl.must(200, "GET", "/v1/me/favorites/ids", nil).Body["data"].([]any)
	if len(ids) != 1 || ids[0] != f.prodA {
		t.Fatalf("favorite ids: %v", ids)
	}
	favs := listAny(cl.must(200, "GET", "/v1/me/favorites", nil))
	if len(favs) != 1 || favs[0].(map[string]any)["available"] != true {
		t.Fatalf("favorites: %v", favs)
	}
	other := f.e.client()
	other.loginClient("+998901234501")
	if n := len(listAny(other.must(200, "GET", "/v1/me/favorites", nil))); n != 0 {
		t.Fatal("favorites leak between users")
	}
	cl.must(204, "DELETE", "/v1/me/favorites/"+f.prodA, nil)
	if n := len(listAny(cl.must(200, "GET", "/v1/me/favorites", nil))); n != 0 {
		t.Fatal("favorite not removed")
	}
}

func TestNotificationsForOrdersAndModeration(t *testing.T) {
	f := newOrderFixture(t)
	cl := f.e.client()
	cl.loginClient("+998901234510")
	r := cl.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 1)), idem(uuid.NewString()))
	oid := r.data()["orders"].([]any)[0].(map[string]any)["id"].(string)

	// магазину пришло «новый заказ», другому магазину — ничего
	if n := f.storeA.must(200, "GET", "/v1/me/notifications/unread-count", nil).data()["unread"].(float64); n != 1 {
		t.Fatalf("store unread=%v", n)
	}
	if n := len(listAny(f.storeB.must(200, "GET", "/v1/me/notifications", nil))); n != 0 {
		t.Fatal("other store notified")
	}
	f.storeA.must(200, "POST", "/v1/store/orders/"+oid+"/status", map[string]any{"status": "confirmed"})
	items := listAny(cl.must(200, "GET", "/v1/me/notifications", nil))
	if len(items) != 1 {
		t.Fatalf("client notifications: %v", items)
	}
	n0 := items[0].(map[string]any)
	if n0["kind"] != "order_status" || n0["data"].(map[string]any)["status"] != "confirmed" || n0["read"] != false {
		t.Fatalf("notification: %v", n0)
	}
	// чужое уведомление отметить нельзя (молча игнорируется)
	f.storeA.must(204, "POST", "/v1/me/notifications/read", map[string]any{"ids": []any{n0["id"]}})
	if cl.must(200, "GET", "/v1/me/notifications/unread-count", nil).data()["unread"].(float64) != 1 {
		t.Fatal("other user marked my notification")
	}
	cl.must(422, "POST", "/v1/me/notifications/read", map[string]any{})
	cl.must(204, "POST", "/v1/me/notifications/read", map[string]any{"all": true})
	if cl.must(200, "GET", "/v1/me/notifications/unread-count", nil).data()["unread"].(float64) != 0 {
		t.Fatal("mark all read")
	}

	// модерация: снятие → не в каталоге, магазин уведомлён, после правки — черновик
	anon := f.e.client()
	f.adm.must(422, "POST", "/v1/admin/products/"+f.prodA2+"/status", map[string]any{"status": "unpublished"})
	f.adm.must(200, "POST", "/v1/admin/products/"+f.prodA2+"/status", map[string]any{"status": "unpublished", "reason": "Фото не соответствуют"})
	anon.must(404, "GET", "/v1/products/"+f.prodA2, nil)
	if r := f.storeA.do("PATCH", "/v1/store/products/"+f.prodA2+"/status", map[string]any{"status": "published"}); r.Status != 404 {
		t.Fatalf("store republished moderated product directly: %s", r.Raw)
	}
	st := f.storeA.must(200, "GET", "/v1/store/products/"+f.prodA2, nil).data()
	if st["status"] != "unpublished" || st["moderation_reason"] != "Фото не соответствуют" {
		t.Fatalf("store sees: %v", st)
	}
	var kinds []string
	for _, it := range listAny(f.storeA.must(200, "GET", "/v1/me/notifications", nil)) {
		kinds = append(kinds, it.(map[string]any)["kind"].(string))
	}
	if strings.Join(kinds, ",") != "product_moderated,order_created" {
		t.Fatalf("store notification kinds: %v", kinds)
	}
	if st := f.storeA.must(200, "PATCH", "/v1/store/products/"+f.prodA2, map[string]any{"title": "Исправлено"}).data(); st["status"] != "draft" {
		t.Fatalf("after fix status=%v", st["status"])
	}
	// блокировка: магазин не может ни редактировать, ни удалить
	f.adm.must(200, "POST", "/v1/admin/products/"+f.prodA2+"/status", map[string]any{"status": "blocked", "reason": "Подделка"})
	f.storeA.must(409, "PATCH", "/v1/store/products/"+f.prodA2, map[string]any{"title": "x"})
	f.storeA.must(404, "DELETE", "/v1/store/products/"+f.prodA2, nil)
	list1 := listAny(f.adm.must(200, "GET", "/v1/admin/products?status=blocked", nil))
	if len(list1) != 1 || list1[0].(map[string]any)["id"] != f.prodA2 {
		t.Fatalf("admin filter: %v", list1)
	}
	f.adm.must(400, "GET", "/v1/admin/products?status=evil", nil)
	// пагинация списка админа
	page := f.adm.must(200, "GET", "/v1/admin/products?limit=2", nil)
	if len(listAny(page)) != 2 || page.Body["next_cursor"] == nil {
		t.Fatalf("admin page: %s", page.Raw)
	}
	page2 := f.adm.must(200, "GET", "/v1/admin/products?limit=2&cursor="+page.Body["next_cursor"].(string), nil)
	if len(listAny(page2)) != 1 {
		t.Fatalf("admin page2: %s", page2.Raw)
	}
	f.storeA.must(403, "GET", "/v1/admin/products", nil)
}

func TestCategoriesAndSettingsAdmin(t *testing.T) {
	f := newOrderFixture(t)
	anon := f.e.client()
	f.adm.must(422, "POST", "/v1/admin/categories", map[string]any{"slug": "dresses"})
	f.adm.must(422, "POST", "/v1/admin/categories", map[string]any{"slug": "Bad Slug", "zone": "top", "name_ru": "x", "name_uz": "y"})
	id := f.adm.must(201, "POST", "/v1/admin/categories", map[string]any{"slug": "dresses", "zone": "top", "name_ru": "Платья", "name_uz": "Ko'ylaklar", "sort": 25}).data()["id"].(string)
	f.adm.must(409, "POST", "/v1/admin/categories", map[string]any{"slug": "dresses", "zone": "top", "name_ru": "x", "name_uz": "y"})
	if n := len(listAny(anon.must(200, "GET", "/v1/categories", nil))); n != 5 {
		t.Fatalf("public categories: %d", n)
	}
	f.adm.must(200, "PATCH", "/v1/admin/categories/"+id, map[string]any{"is_active": false})
	if n := len(listAny(anon.must(200, "GET", "/v1/categories", nil))); n != 4 {
		t.Fatalf("inactive category still public: %d", n)
	}
	f.adm.must(422, "PATCH", "/v1/admin/categories/"+id, map[string]any{"parent_id": id})
	f.adm.must(404, "PATCH", "/v1/admin/categories/"+uuid.NewString(), map[string]any{"sort": 1})
	if n := len(listAny(f.adm.must(200, "GET", "/v1/admin/categories", nil))); n != 5 {
		t.Fatalf("admin categories: %d", n)
	}

	s := f.adm.must(200, "PUT", "/v1/admin/settings", map[string]any{"generation_cost": 25, "support_phone": "+998712000000", "support_email": "help@boutique.uz"}).data()
	if s["generation_cost"].(float64) != 25 {
		t.Fatalf("settings: %v", s)
	}
	f.adm.must(422, "PUT", "/v1/admin/settings", map[string]any{"generation_cost": -1})
	f.adm.must(400, "PUT", "/v1/admin/settings", map[string]any{"unknown_key": 1})
	f.adm.must(422, "PUT", "/v1/admin/settings", map[string]any{"support_phone": "12345"})
	pub := anon.must(200, "GET", "/v1/platform", nil).data()
	if pub["generation_cost"].(float64) != 25 || pub["support_email"] != "help@boutique.uz" {
		t.Fatalf("public settings: %v", pub)
	}
	if _, ok := pub["welcome_bonus"]; ok {
		t.Fatal("welcome_bonus exposed publicly")
	}
	var n int
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action IN ('category.create','category.update','settings.update')`).Scan(&n)
	if n != 3 {
		t.Fatalf("audit entries: %d", n)
	}
}

func xlsx(t *testing.T, rows [][]any) []byte {
	t.Helper()
	x := excelize.NewFile()
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := x.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if err := x.Write(&b); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestExcelImportAndBulkPhotos(t *testing.T) {
	f := newOrderFixture(t)
	tpl := f.storeA.do("GET", "/v1/store/products/import/template", nil)
	if tpl.Status != 200 || !strings.HasPrefix(tpl.Raw, "PK") {
		t.Fatalf("template: %d", tpl.Status)
	}
	hdr := []any{"sku", "title", "description", "category", "brand", "price", "in_stock"}
	file := xlsx(t, [][]any{
		hdr,
		{"IMP-1", "Кепка", "Хлопок", "headwear", "Acme", "150 000", "да"},
		{"IMP-2", "Брюки", "Лён", "Pastki kiyimlar", "Acme", 320000.5, "yo'q"},
		{"IMP-3", "", "x", "tops", "Acme", "1", "да"},              // нет названия
		{"IMP-4", "Туфли", "Кожа", "socks", "Acme", "abc", "?"},    // 3 ошибки
		{"IMP-1", "Дубль", "x", "tops", "Acme", "1", "да"},         // дубль артикула в файле
		{"", "Шапка", "Шерсть", "headwear", "Acme", "90000", "да"}, // без артикула — автоартикул
		{"", "Шарф", "Шерсть", "tops", "Acme", "90000", "да"},      // второй пустой — не дубль
		{},
	})
	res := f.storeA.must(200, "POST", "/v1/store/products/import", file).data()
	if res["total"].(float64) != 7 || res["created"].(float64) != 4 || res["failed"].(float64) != 3 {
		t.Fatalf("import result: %v", res)
	}
	if g := res["generated"].([]any); len(g) != 2 || g[0].(map[string]any)["sku"] != "A-000001" || g[1].(map[string]any)["sku"] != "A-000002" ||
		g[0].(map[string]any)["row"].(float64) != 7 {
		t.Fatalf("generated: %v", res["generated"])
	}
	if n := len(res["errors"].([]any)); n != 5 {
		t.Fatalf("errors: %v", res["errors"])
	}
	var price int64
	var inStock bool
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT price, in_stock FROM products WHERE sku = 'IMP-2'`).Scan(&price, &inStock)
	if price != 32000050 || inStock {
		t.Fatalf("IMP-2 price=%d in_stock=%v", price, inStock)
	}
	// повторный импорт обновляет, а не дублирует
	res = f.storeA.must(200, "POST", "/v1/store/products/import", xlsx(t, [][]any{hdr, {"IMP-1", "Кепка 2", "Хлопок", "headwear", "Acme", "160000", "да"}})).data()
	if res["created"].(float64) != 0 || res["updated"].(float64) != 1 {
		t.Fatalf("reimport: %v", res)
	}
	// неверный файл и нет столбцов
	f.storeA.must(415, "POST", "/v1/store/products/import", []byte("not an excel file"))
	if r := f.storeA.do("POST", "/v1/store/products/import", xlsx(t, [][]any{{"sku", "title"}})); r.errCode() != "missing_columns" {
		t.Fatalf("missing columns: %s", r.Raw)
	}
	// товары другого магазина не задеты
	if n := len(listAny(f.storeB.must(200, "GET", "/v1/store/products", nil))); n != 1 {
		t.Fatalf("store B products: %d", n)
	}

	keys := uploadKeys(t, f.storeA, "product", 4)
	foreign := uploadKeys(t, f.storeB, "product", 1)[0]
	out := f.storeA.must(200, "POST", "/v1/store/products/images/bulk", map[string]any{"items": []map[string]any{
		{"filename": "IMP-1_front.jpg", "key": keys[0]},
		{"filename": "IMP-1_side.JPG", "key": keys[1]},
		{"filename": "imp-1_back.png", "key": keys[2]},
		{"filename": "UNKNOWN_front.jpg", "key": keys[3]},
		{"filename": "IMP-2.jpg", "key": keys[3]},
		{"filename": "IMP-2_front.jpg", "key": foreign},
	}}).Body["data"].([]any)
	var errs []string
	for _, o := range out {
		e, _ := o.(map[string]any)["error"].(string)
		errs = append(errs, e)
	}
	if strings.Join(errs, ",") != ",,,unknown_sku,bad_name,not_owned" {
		t.Fatalf("bulk results: %v", errs)
	}
	var pid string
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT id FROM products WHERE sku = 'IMP-1'`).Scan(&pid)
	f.storeA.must(200, "PATCH", "/v1/store/products/"+pid+"/status", map[string]any{"status": "published"})
}

func TestStoreStats(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	vc := &products.ViewCounter{Redis: f.e.d.Redis, DB: f.e.d.DB, Log: f.e.d.Log}
	for i := 0; i < 4; i++ {
		vc.Hit(ctx, uuid.MustParse(f.prodA))
	}
	vc.Hit(ctx, uuid.MustParse(f.prodB))
	if err := vc.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := products.RecordFittingAdd(ctx, f.e.d.DB, uuid.MustParse(f.prodA)); err != nil {
		t.Fatal(err)
	}
	// батч из счётчика примерочной (Redis → БД)
	if err := products.RecordFittingAdds(ctx, f.e.d.DB, []uuid.UUID{uuid.MustParse(f.prodA)}, []int64{2}); err != nil {
		t.Fatal(err)
	}
	cl := f.e.client()
	r := cl.loginClient("+998901234520")
	uid := r.data()["user"].(map[string]any)["id"].(string)
	_, _ = f.e.d.DB.Exec(ctx, `INSERT INTO generations (user_id, status, cost, product_ids, photo_keys) VALUES
		($1, 'succeeded', 20, ARRAY[$2::uuid], '{}'), ($1, 'failed', 20, ARRAY[$2::uuid], '{}'), ($1, 'succeeded', 20, ARRAY[$3::uuid], '{}')`,
		uid, f.prodA, f.prodB)
	o := cl.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 2), item(f.prodB, 1)), idem(uuid.NewString()))
	_ = o

	st := f.storeA.must(200, "GET", "/v1/store/stats", nil).data()
	want := map[string]float64{"products_total": 2, "products_published": 2, "views": 4, "fitting_adds": 3, "tryons": 1, "orders": 1, "revenue": 0}
	for k, v := range want {
		if st[k].(float64) != v {
			t.Errorf("%s = %v, want %v", k, st[k], v)
		}
	}
	top := st["top_products"].([]any)
	if len(top) != 1 || top[0].(map[string]any)["id"] != f.prodA || top[0].(map[string]any)["ordered"].(float64) != 2 {
		t.Fatalf("top: %v", top)
	}
	if n := len(st["daily"].([]any)); n != 30 {
		t.Fatalf("daily points: %d", n)
	}
	f.storeA.must(400, "GET", "/v1/store/stats?from=2026-01-10&to=2026-01-01", nil)
	f.storeA.must(400, "GET", "/v1/store/stats?from=2020-01-01&to=2026-01-01", nil)
	f.storeA.must(400, "GET", "/v1/store/stats?from=bad", nil)
	// магазин B видит только своё
	stB := f.storeB.must(200, "GET", "/v1/store/stats", nil).data()
	if stB["views"].(float64) != 1 || stB["tryons"].(float64) != 1 {
		t.Fatalf("store B stats: %v", stB)
	}
}

func TestDeleteAccount(t *testing.T) {
	f := newOrderFixture(t)
	cl := f.e.client()
	r := cl.loginClient("+998901234530")
	uid := r.data()["user"].(map[string]any)["id"].(string)
	cl.must(200, "POST", "/v1/me/complete-registration", map[string]any{"gender": "female", "pd_consent": true})
	cl.must(200, "PUT", "/v1/me/cart/"+f.prodA, map[string]any{"qty": 1})
	cl.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodB, 1)), idem(uuid.NewString()))
	// фото тела для примерки: должно удалиться вместе с аккаунтом
	ctx := context.Background()
	bodyKey := "priv/u/" + uid + "/body_photo/front.jpg"
	if err := f.e.d.Storage.Put(ctx, bodyKey, []byte("x"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.d.DB.Exec(ctx, `INSERT INTO body_photos (user_id, angle, key) VALUES ($1, 'front', $2)`, uid, bodyKey); err != nil {
		t.Fatal(err)
	}

	cl.must(422, "DELETE", "/v1/me", map[string]any{"confirm": false})
	cl.must(204, "DELETE", "/v1/me", map[string]any{"confirm": true})
	if r := cl.do("GET", "/v1/me", nil); r.Status != 401 {
		t.Fatalf("deleted user still has access: %d", r.Status)
	}
	var phone *string
	var orders, cart, photos int
	_ = f.e.d.DB.QueryRow(ctx, `SELECT phone FROM users WHERE id = $1`, uid).Scan(&phone)
	_ = f.e.d.DB.QueryRow(ctx, `SELECT count(*) FROM orders WHERE user_id = $1`, uid).Scan(&orders)
	_ = f.e.d.DB.QueryRow(ctx, `SELECT count(*) FROM cart_items WHERE user_id = $1`, uid).Scan(&cart)
	_ = f.e.d.DB.QueryRow(ctx, `SELECT count(*) FROM body_photos WHERE user_id = $1`, uid).Scan(&photos)
	if phone != nil || orders != 1 || cart != 0 || photos != 0 {
		t.Fatalf("phone=%v orders=%d cart=%d body_photos=%d", phone, orders, cart, photos)
	}
	if _, err := f.e.d.Storage.Get(ctx, bodyKey); err == nil {
		t.Fatal("body photo object still in storage")
	}
	// тот же номер регистрируется заново как новый аккаунт
	again := f.e.client()
	r = again.loginClient("+998901234530")
	if r.data()["is_new_user"] != true || r.data()["user"].(map[string]any)["id"] == uid {
		t.Fatalf("re-register: %v", r.data())
	}
}
