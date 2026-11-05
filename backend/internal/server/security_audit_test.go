package server

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Регрессии по итогам проверки безопасности 2026-11-05.

// Скрытый модератором отзыв нельзя удалить и сразу оставить заново видимым.
func TestSecurity_HiddenReviewCannotBeReposted(t *testing.T) {
	f := newOrderFixture(t)
	buyer, anon := f.e.client(), f.e.client()
	buyer.loginClient("+998901239001")
	orders := buyer.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 1)), idem("hidden-0001")).data()["orders"].([]any)
	deliver(f, f.storeA, orders[0].(map[string]any)["id"].(string))
	path := "/v1/reviews/products/" + f.prodA
	rid := buyer.must(201, "PUT", path, map[string]any{"rating": 1, "body": "спам спам"}).data()["id"].(string)
	f.adm.must(200, "POST", "/v1/admin/reviews/"+rid+"/status", map[string]any{"status": "hidden", "reason": "Спам"})

	buyer.must(204, "DELETE", path, nil) // текст стирается, отзыв остаётся скрытым
	mine := buyer.must(200, "GET", path, nil).data()["review"].(map[string]any)
	if mine["status"] != "hidden" || mine["body"] != "" {
		t.Fatalf("hidden review after delete: %v", mine)
	}
	if r := buyer.do("PUT", path, map[string]any{"rating": 1, "body": "спам снова"}); r.errCode() != "review_hidden" {
		t.Fatalf("re-post after deleting hidden review: %d %s", r.Status, r.Raw)
	}
	if l := listAny(anon.must(200, "GET", "/v1/products/"+f.prodA+"/reviews", nil)); len(l) != 0 {
		t.Fatalf("hidden review is public: %v", l)
	}
	// обычный (видимый) отзыв по-прежнему удаляется полностью
	buyer2 := f.e.client()
	buyer2.loginClient("+998901239002")
	orders = buyer2.mustH(201, "POST", "/v1/orders", checkoutBody(item(f.prodA, 1)), idem("hidden-0002")).data()["orders"].([]any)
	deliver(f, f.storeA, orders[0].(map[string]any)["id"].(string))
	buyer2.must(201, "PUT", path, map[string]any{"rating": 5})
	buyer2.must(204, "DELETE", path, nil)
	buyer2.must(404, "DELETE", path, nil)
	buyer2.must(201, "PUT", path, map[string]any{"rating": 4})
}

// Сайт бутика выводится ссылкой на витрине: только http(s).
func TestSecurity_StoreWebsiteOnlyHTTP(t *testing.T) {
	f := newOrderFixture(t)
	for _, bad := range []string{"javascript:alert(document.cookie)", "data:text/html,<script>alert(1)</script>", "ftp://x.uz", "https://user:pw@x.uz"} {
		r := f.storeA.do("PATCH", "/v1/store/profile", map[string]any{"contacts": map[string]any{"website": bad}})
		if r.Status != 422 {
			t.Fatalf("website %q accepted: %d %s", bad, r.Status, r.Raw)
		}
	}
	f.storeA.must(200, "PATCH", "/v1/store/profile", map[string]any{"contacts": map[string]any{"website": "https://alpha.uz/shop"}})
}

// Параметры тела: те же лимиты, что в примерке (иначе 1 МБ мусора в users.body_params).
func TestSecurity_BodyParamsLimits(t *testing.T) {
	e := setup(t)
	c := e.client()
	c.loginClient("+998901239003")
	for _, bad := range []map[string]any{{strings.Repeat("x", 33): 1}, {"chest": 0}, {"chest": 1e9}, {"<b>": 90}} {
		if r := c.do("PATCH", "/v1/me", map[string]any{"body_params": bad}); r.Status != 422 {
			t.Fatalf("body_params %v accepted: %d", bad, r.Status)
		}
	}
	c.must(200, "PATCH", "/v1/me", map[string]any{"body_params": map[string]any{"chest": 92, "обхват_талии": 70}})
}

// Мусор в курсоре каталога — 400, а не 500 от Postgres.
func TestSecurity_BadCatalogCursor(t *testing.T) {
	e := setup(t)
	anon := e.client()
	for _, sort := range []string{"new", "price_asc", "rating", "popular"} {
		c := base64.RawURLEncoding.EncodeToString([]byte("abc|" + uuid.NewString()))
		if r := anon.do("GET", "/v1/products?sort="+sort+"&cursor="+c, nil); r.Status != 400 {
			t.Fatalf("sort=%s bad cursor: %d %s", sort, r.Status, r.Raw)
		}
	}
}

// Номер получателя заказа (на него уходят SMS) — только разрешённые страны.
func TestSecurity_OrderPhoneCountry(t *testing.T) {
	f := newOrderFixture(t)
	c := f.e.client()
	c.loginClient("+998901239004")
	body := checkoutBody(item(f.prodA, 1))
	body["customer_phone"] = "+447700900123"
	if r := c.doH("POST", "/v1/orders", body, idem("phone-0001")); r.Status != 422 {
		t.Fatalf("foreign customer_phone accepted: %d %s", r.Status, r.Raw)
	}
	body["customer_phone"] = "+998901239999"
	c.mustH(201, "POST", "/v1/orders", body, idem("phone-0002"))
}
