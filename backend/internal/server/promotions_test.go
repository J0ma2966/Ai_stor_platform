package server

import (
	"context"
	"strings"
	"testing"
	"time"
)

func promoIDs(r resp) []string {
	var out []string
	for _, it := range listAny(r) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func notifCount(t *testing.T, f *orderFixture, kind string) int {
	t.Helper()
	var n int
	if err := f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE kind = $1`, kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPromotions_RequestApproveCarouselStop(t *testing.T) {
	f := newOrderFixture(t)
	anon := f.e.client()
	buyer := f.e.client()
	buyer.loginClient("+998901240001")

	if l := listAny(anon.must(200, "GET", "/v1/promotions", nil)); len(l) != 0 {
		t.Fatalf("carousel before any promotion: %v", l)
	}

	// роли: клиент и аноним не подают заявки, магазин не одобряет
	buyer.must(403, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 7})
	anon.must(401, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 7})
	f.storeA.must(403, "GET", "/v1/admin/promotions", nil)

	// валидация и изоляция магазинов
	f.storeA.must(422, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 0})
	f.storeA.must(422, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 91})
	f.storeA.must(422, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 7, "note": strings.Repeat("я", 501)})
	f.storeA.must(404, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodB, "days": 7}) // чужой товар
	f.storeA.must(400, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 7, "status": "approved"})

	req := f.storeA.must(201, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 7, "note": "  Новая коллекция\x00 "}).data()
	if req["state"] != "pending" || req["store_note"] != "Новая коллекция" || req["product_title"] != "Платье" || req["starts_at"] != nil {
		t.Fatalf("created request: %v", req)
	}
	pid := req["id"].(string)
	// вторая заявка на тот же товар — конфликт
	if r := f.storeA.do("POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 3}); r.errCode() != "promotion_exists" {
		t.Fatalf("duplicate request: %s", r.Raw)
	}
	// заявка ещё не одобрена — на главной её нет
	if l := listAny(anon.must(200, "GET", "/v1/promotions", nil)); len(l) != 0 {
		t.Fatal("pending promotion shown")
	}
	if ids := promoIDs(f.storeB.must(200, "GET", "/v1/store/promotions", nil)); len(ids) != 0 {
		t.Fatalf("store B sees store A requests: %v", ids)
	}
	f.storeB.must(404, "DELETE", "/v1/store/promotions/"+pid, nil)

	// админ видит заявку в фильтре «на рассмотрении»
	if ids := promoIDs(f.adm.must(200, "GET", "/v1/admin/promotions?state=pending", nil)); len(ids) != 1 || ids[0] != pid {
		t.Fatalf("admin pending list: %v", ids)
	}
	f.adm.must(400, "GET", "/v1/admin/promotions?state=bogus", nil)
	// неверный период
	f.adm.must(422, "POST", "/v1/admin/promotions/"+pid+"/approve", map[string]any{
		"starts_at": time.Now().Add(48 * time.Hour), "ends_at": time.Now().Add(24 * time.Hour)})
	f.adm.must(422, "POST", "/v1/admin/promotions/"+pid+"/approve", map[string]any{"starts_at": time.Now().Add(-72 * time.Hour)})
	f.adm.must(422, "POST", "/v1/admin/promotions/"+pid+"/reject", map[string]any{"reason": " "})

	ap := f.adm.must(200, "POST", "/v1/admin/promotions/"+pid+"/approve", map[string]any{}).data()
	if ap["state"] != "active" || ap["status"] != "approved" {
		t.Fatalf("approved: %v", ap)
	}
	start, _ := time.Parse(time.RFC3339Nano, ap["starts_at"].(string))
	end, _ := time.Parse(time.RFC3339Nano, ap["ends_at"].(string))
	if d := end.Sub(start); d != 7*24*time.Hour {
		t.Fatalf("default period = request days: %v", d)
	}
	if notifCount(t, f, "promotion_approved") != 1 {
		t.Fatal("store not notified about approval")
	}
	// повторное решение по уже рассмотренной заявке
	if r := f.adm.do("POST", "/v1/admin/promotions/"+pid+"/approve", map[string]any{}); r.errCode() != "promotion_decided" {
		t.Fatalf("approve twice: %s", r.Raw)
	}
	if r := f.storeA.do("DELETE", "/v1/store/promotions/"+pid, nil); r.errCode() != "promotion_decided" {
		t.Fatalf("store cancels approved: %s", r.Raw)
	}

	// карусель на главной: товар в той же форме, что в каталоге
	car := listAny(anon.must(200, "GET", "/v1/promotions", nil))
	if len(car) != 1 {
		t.Fatalf("carousel: %v", car)
	}
	card := car[0].(map[string]any)
	if card["id"] != f.prodA || card["store_name"] != "Бутик alpha" || card["price"].(float64) != 100000 {
		t.Fatalf("carousel card: %v", card)
	}
	if _, leaked := card["store_note"]; leaked {
		t.Fatal("public carousel exposes request details")
	}

	// администратор сам запускает рекламу товара магазина B, с запланированным началом
	sched := f.adm.must(201, "POST", "/v1/admin/promotions", map[string]any{
		"product_id": f.prodB, "starts_at": time.Now().Add(time.Hour), "ends_at": time.Now().Add(5 * 24 * time.Hour)}).data()
	if sched["state"] != "scheduled" {
		t.Fatalf("scheduled: %v", sched)
	}
	if r := f.adm.do("POST", "/v1/admin/promotions", map[string]any{"product_id": f.prodB}); r.errCode() != "promotion_exists" {
		t.Fatalf("second admin promotion: %s", r.Raw)
	}
	if l := listAny(anon.must(200, "GET", "/v1/promotions", nil)); len(l) != 1 {
		t.Fatalf("scheduled promotion shown early: %v", l)
	}
	// старт наступил
	if _, err := f.e.d.DB.Exec(context.Background(), `UPDATE product_promotions SET starts_at = now() - interval '1 minute' WHERE id = $1`, sched["id"]); err != nil {
		t.Fatal(err)
	}
	if l := listAny(anon.must(200, "GET", "/v1/promotions", nil)); len(l) != 2 {
		t.Fatalf("both promotions live: %v", l)
	}

	// заблокированный магазин и снятый с витрины товар выпадают из карусели
	f.adm.must(200, "POST", "/v1/admin/stores/"+f.storeB2+"/status", map[string]any{"status": "blocked", "reason": "проверка"})
	if l := listAny(anon.must(200, "GET", "/v1/promotions", nil)); len(l) != 1 {
		t.Fatalf("blocked store still promoted: %v", l)
	}

	// остановка администратором
	f.adm.must(200, "POST", "/v1/admin/promotions/"+pid+"/stop", map[string]any{"reason": "Жалобы покупателей"})
	if l := listAny(anon.must(200, "GET", "/v1/promotions", nil)); len(l) != 0 {
		t.Fatalf("stopped promotion shown: %v", l)
	}
	if r := f.adm.do("POST", "/v1/admin/promotions/"+pid+"/stop", map[string]any{}); r.errCode() != "promotion_not_running" {
		t.Fatalf("stop twice: %s", r.Raw)
	}
	if notifCount(t, f, "promotion_stopped") != 1 {
		t.Fatal("store not notified about stop")
	}
	mine := listAny(f.storeA.must(200, "GET", "/v1/store/promotions", nil))
	if len(mine) != 1 || mine[0].(map[string]any)["state"] != "stopped" || mine[0].(map[string]any)["admin_reason"] != "Жалобы покупателей" {
		t.Fatalf("store sees stopped: %v", mine)
	}

	// после остановки можно подать новую заявку; её можно отозвать или отклонить
	again := f.storeA.must(201, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 3}).data()["id"].(string)
	f.storeA.must(204, "DELETE", "/v1/store/promotions/"+again, nil)
	again = f.storeA.must(201, "POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA, "days": 3}).data()["id"].(string)
	rej := f.adm.must(200, "POST", "/v1/admin/promotions/"+again+"/reject", map[string]any{"reason": "Фото не по правилам"}).data()
	if rej["state"] != "rejected" || notifCount(t, f, "promotion_rejected") != 1 {
		t.Fatalf("rejected: %v", rej)
	}
	if ids := promoIDs(f.adm.must(200, "GET", "/v1/admin/promotions?store="+f.storeAID, nil)); len(ids) != 2 {
		t.Fatalf("admin list by store: %v", ids)
	}

	// неопубликованный товар продвигать нельзя
	f.storeA.must(200, "PATCH", "/v1/store/products/"+f.prodA2+"/status", map[string]any{"status": "draft"})
	if r := f.storeA.do("POST", "/v1/store/promotions", map[string]any{"product_id": f.prodA2, "days": 3}); r.errCode() != "product_not_published" {
		t.Fatalf("hidden product promoted: %s", r.Raw)
	}

	// истёкшая реклама не мешает новой заявке
	if _, err := f.e.d.DB.Exec(context.Background(), `INSERT INTO product_promotions (product_id, store_id, status, days, starts_at, ends_at)
		VALUES ($1, $2, 'approved', 1, now() - interval '3 days', now() - interval '2 days')`, f.prodA2, f.storeAID); err != nil {
		t.Fatal(err)
	}
	if ids := promoIDs(f.adm.must(200, "GET", "/v1/admin/promotions?state=ended", nil)); len(ids) != 1 {
		t.Fatalf("ended filter: %v", ids)
	}
}
