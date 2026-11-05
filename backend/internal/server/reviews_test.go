package server

import (
	"context"
	"strings"
	"testing"
)

// deliver проводит заказ магазина через все статусы до «доставлен».
func deliver(f *orderFixture, store *client, orderID string) {
	for _, s := range []string{"confirmed", "assembling", "in_delivery", "delivered"} {
		store.must(200, "POST", "/v1/store/orders/"+orderID+"/status", map[string]any{"status": s})
	}
}

func TestReviews_OnlyBuyersRatingAndModeration(t *testing.T) {
	f := newOrderFixture(t)
	anon := f.e.client()
	buyer := f.e.client()
	buyer.loginClient("+998901230001")
	other := f.e.client()
	other.loginClient("+998901230002")

	path := "/v1/reviews/products/" + f.prodA

	// без доставленного заказа отзыв оставить нельзя
	if !(buyer.must(200, "GET", path, nil).data()["can_review"] == false) {
		t.Fatal("can_review before order")
	}
	if r := buyer.do("PUT", path, map[string]any{"rating": 5, "body": "Супер"}); r.errCode() != "review_not_allowed" {
		t.Fatalf("review without order: %s", r.Raw)
	}
	orders := buyer.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 1)), idem("review-0001")).data()["orders"].([]any)
	oid := orders[0].(map[string]any)["id"].(string)
	// заказ ещё не доставлен
	if r := buyer.do("PUT", path, map[string]any{"rating": 5}); r.errCode() != "review_not_allowed" {
		t.Fatalf("review before delivery: %s", r.Raw)
	}
	deliver(f, f.storeA, oid)
	if buyer.must(200, "GET", path, nil).data()["can_review"] != true {
		t.Fatal("can_review after delivery")
	}

	// валидация оценки
	for _, bad := range []int{0, 6, -1} {
		if r := buyer.do("PUT", path, map[string]any{"rating": bad}); r.Status != 422 {
			t.Fatalf("rating %d accepted: %s", bad, r.Raw)
		}
	}
	buyer.must(422, "PUT", path, map[string]any{"rating": 5, "body": strings.Repeat("я", 2001)})
	buyer.must(400, "PUT", path, map[string]any{"rating": 5, "author_name": "Хакер"}) // подпись не задаётся клиентом

	rv := buyer.must(201, "PUT", path, map[string]any{"rating": 4, "body": "  Хорошее платье\x00  "}).data()
	if rv["body"] != "Хорошее платье" || rv["author_name"] != "Азиз К." || rv["status"] != "visible" {
		t.Fatalf("created review: %v", rv)
	}
	rid := rv["id"].(string)
	// повторный PUT — правка, а не второй отзыв
	buyer.must(200, "PUT", path, map[string]any{"rating": 2, "body": "Передумал"})

	// чужой клиент не может оставить отзыв на этот товар
	other.must(403, "PUT", path, map[string]any{"rating": 1})

	// средняя оценка в карточке и каталоге
	card := anon.must(200, "GET", "/v1/products/"+f.prodA, nil).data()
	if card["rating_count"].(float64) != 1 || card["rating_avg"].(float64) != 2 {
		t.Fatalf("card rating: %v %v", card["rating_avg"], card["rating_count"])
	}
	dist := card["rating_dist"].([]any)
	if len(dist) != 5 || dist[1].(float64) != 1 || dist[3].(float64) != 0 {
		t.Fatalf("rating dist: %v", dist)
	}
	pub := listAny(anon.must(200, "GET", "/v1/products/"+f.prodA+"/reviews", nil))
	if len(pub) != 1 || pub[0].(map[string]any)["body"] != "Передумал" {
		t.Fatalf("public reviews: %v", pub)
	}
	if _, leaked := pub[0].(map[string]any)["user_id"]; leaked {
		t.Fatal("public review exposes user id")
	}
	if strings.Contains(fmtAny(pub), "+998") || strings.Contains(fmtAny(pub), "Каримов") {
		t.Fatal("public review exposes buyer's phone or surname")
	}
	if n := len(listAny(anon.must(200, "GET", "/v1/products/"+f.prodA+"/reviews?rating=5", nil))); n != 0 {
		t.Fatalf("rating filter: %d", n)
	}
	anon.must(400, "GET", "/v1/products/"+f.prodA+"/reviews?rating=9", nil)
	top := listAny(anon.must(200, "GET", "/v1/products?sort=rating", nil))
	if top[0].(map[string]any)["id"] != f.prodA {
		t.Fatalf("sort by rating: %v", top[0])
	}

	// магазину пришло уведомление; он видит отзыв и отвечает
	var n int
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE kind = 'review_created'`).Scan(&n)
	if n != 1 {
		t.Fatalf("review_created notifications: %d", n)
	}
	if l := listAny(f.storeA.must(200, "GET", "/v1/store/reviews?unanswered=true", nil)); len(l) != 1 {
		t.Fatalf("store reviews: %v", l)
	}
	if l := listAny(f.storeB.must(200, "GET", "/v1/store/reviews", nil)); len(l) != 0 {
		t.Fatalf("store B sees foreign reviews: %v", l)
	}
	f.storeB.must(404, "PUT", "/v1/store/reviews/"+rid+"/reply", map[string]any{"text": "чужой"})
	f.storeA.must(422, "PUT", "/v1/store/reviews/"+rid+"/reply", map[string]any{"text": "   "})
	f.storeA.must(200, "PUT", "/v1/store/reviews/"+rid+"/reply", map[string]any{"text": "Спасибо! Обменяем размер."})
	f.storeA.must(200, "PUT", "/v1/store/reviews/"+rid+"/reply", map[string]any{"text": "Спасибо! Обменяем размер бесплатно."})
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE kind = 'review_reply'`).Scan(&n)
	if n != 1 {
		t.Fatalf("review_reply notifications (only first reply notifies): %d", n)
	}
	if l := listAny(f.storeA.must(200, "GET", "/v1/store/reviews?unanswered=true", nil)); len(l) != 0 {
		t.Fatal("answered review still in unanswered filter")
	}
	pub = listAny(anon.must(200, "GET", "/v1/products/"+f.prodA+"/reviews", nil))
	if pub[0].(map[string]any)["reply"] != "Спасибо! Обменяем размер бесплатно." {
		t.Fatalf("reply not public: %v", pub[0])
	}
	// роли: магазин не может модерировать, клиент — отвечать
	f.storeA.must(403, "POST", "/v1/admin/reviews/"+rid+"/status", map[string]any{"status": "hidden", "reason": "плохой"})
	buyer.must(403, "PUT", "/v1/store/reviews/"+rid+"/reply", map[string]any{"text": "x"})
	f.storeA.must(403, "PUT", path, map[string]any{"rating": 5})

	// модерация: скрытие с причиной убирает отзыв и из средней оценки
	f.adm.must(422, "POST", "/v1/admin/reviews/"+rid+"/status", map[string]any{"status": "hidden"})
	f.adm.must(200, "POST", "/v1/admin/reviews/"+rid+"/status", map[string]any{"status": "hidden", "reason": "Оскорбления"})
	if l := listAny(anon.must(200, "GET", "/v1/products/"+f.prodA+"/reviews", nil)); len(l) != 0 {
		t.Fatal("hidden review is public")
	}
	card = anon.must(200, "GET", "/v1/products/"+f.prodA, nil).data()
	if card["rating_count"].(float64) != 0 || card["rating_avg"].(float64) != 0 {
		t.Fatalf("hidden review counted: %v", card)
	}
	if l := listAny(f.adm.must(200, "GET", "/v1/admin/reviews?status=hidden", nil)); len(l) != 1 {
		t.Fatalf("admin hidden list: %v", l)
	}
	mine := buyer.must(200, "GET", path, nil).data()["review"].(map[string]any)
	if mine["status"] != "hidden" || mine["moderation_reason"] != "Оскорбления" {
		t.Fatalf("own hidden review: %v", mine)
	}
	if r := buyer.do("PUT", path, map[string]any{"rating": 5}); r.errCode() != "review_hidden" {
		t.Fatalf("edit of hidden review: %s", r.Raw)
	}
	f.adm.must(200, "POST", "/v1/admin/reviews/"+rid+"/status", map[string]any{"status": "visible"})
	if anon.must(200, "GET", "/v1/products/"+f.prodA, nil).data()["rating_count"].(float64) != 1 {
		t.Fatal("restored review not counted")
	}
	_ = f.e.d.DB.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'review.moderate'`).Scan(&n)
	if n != 2 {
		t.Fatalf("audit: %d", n)
	}

	// удаление своего отзыва
	buyer.must(204, "DELETE", path, nil)
	buyer.must(404, "DELETE", path, nil)
	if anon.must(200, "GET", "/v1/products/"+f.prodA, nil).data()["rating_count"].(float64) != 0 {
		t.Fatal("deleted review counted")
	}
}

func TestReviews_AverageAndAccountDeletion(t *testing.T) {
	f := newOrderFixture(t)
	anon := f.e.client()
	var clients []*client
	for i, rating := range []int{5, 4, 4} {
		c := f.e.client()
		c.loginClient("+99890123100" + string(rune('1'+i)))
		orders := c.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 1)), idem("avg-000"+string(rune('1'+i)))).data()["orders"].([]any)
		deliver(f, f.storeA, orders[0].(map[string]any)["id"].(string))
		c.must(201, "PUT", "/v1/reviews/products/"+f.prodA, map[string]any{"rating": rating})
		clients = append(clients, c)
	}
	card := anon.must(200, "GET", "/v1/products/"+f.prodA, nil).data()
	if card["rating_count"].(float64) != 3 || card["rating_avg"].(float64) != 4.33 {
		t.Fatalf("avg: %v %v", card["rating_avg"], card["rating_count"])
	}
	// пагинация публичного списка
	p1 := anon.must(200, "GET", "/v1/products/"+f.prodA+"/reviews?limit=2", nil)
	cur, _ := p1.Body["next_cursor"].(string)
	if len(listAny(p1)) != 2 || cur == "" {
		t.Fatalf("page 1: %s", p1.Raw)
	}
	if l := listAny(anon.must(200, "GET", "/v1/products/"+f.prodA+"/reviews?limit=2&cursor="+cur, nil)); len(l) != 1 {
		t.Fatalf("page 2: %v", l)
	}
	// удаление аккаунта убирает отзыв и пересчитывает среднюю
	clients[0].must(204, "DELETE", "/v1/me", map[string]any{"confirm": true})
	card = anon.must(200, "GET", "/v1/products/"+f.prodA, nil).data()
	if card["rating_count"].(float64) != 2 || card["rating_avg"].(float64) != 4 {
		t.Fatalf("after account deletion: %v %v", card["rating_avg"], card["rating_count"])
	}
}
