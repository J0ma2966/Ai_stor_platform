package orders

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/boutique-market/backend/internal/auth"
)

func TestTransitions(t *testing.T) {
	cases := []struct {
		role     auth.Role
		from, to Status
		reason   string
		want     error
	}{
		{auth.RoleStore, StatusPending, StatusConfirmed, "", nil},
		{auth.RoleStore, StatusPending, StatusRejected, "", ErrReasonRequired},
		{auth.RoleStore, StatusPending, StatusRejected, "нет в наличии", nil},
		{auth.RoleStore, StatusConfirmed, StatusAssembling, "", nil},
		{auth.RoleStore, StatusAssembling, StatusInDelivery, "", nil},
		{auth.RoleAdmin, StatusInDelivery, StatusDelivered, "", nil},
		{auth.RoleStore, StatusPending, StatusDelivered, "", ErrInvalidTransition},
		{auth.RoleStore, StatusDelivered, StatusAssembling, "", ErrInvalidTransition},
		{auth.RoleStore, StatusRejected, StatusConfirmed, "", ErrInvalidTransition},
		{auth.RoleStore, StatusInDelivery, StatusCancelled, "x", ErrInvalidTransition},
		{auth.RoleClient, StatusPending, StatusConfirmed, "", ErrForbidden},
		{"", StatusPending, StatusConfirmed, "", ErrForbidden},
		{auth.RoleStore, Status("hacked"), StatusConfirmed, "", ErrInvalidTransition},
		{auth.RoleStore, StatusConfirmed, StatusCancelled, "   ", ErrReasonRequired},
	}
	for _, c := range cases {
		if got := CheckTransition(c.role, c.from, c.to, c.reason); !errors.Is(got, c.want) {
			t.Errorf("%s %s->%s: got %v want %v", c.role, c.from, c.to, got, c.want)
		}
	}
	for _, s := range []Status{StatusRejected, StatusDelivered, StatusCancelled} {
		if !s.Terminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	if CustomerStep(StatusInDelivery) != 3 || CustomerStep(StatusRejected) != 0 {
		t.Error("customer steps")
	}
}

func f(v float64) *float64 { return &v }

func TestNormalize(t *testing.T) {
	in := checkoutReq{
		Items:           []checkoutItem{{ProductID: uuid.New(), Qty: 1}},
		CustomerName:    " Азиз ",
		Delivery:        deliveryReq{Street: "  Амира Темура\t", House: "12a", Entrance: "3\u202e", Lat: f(41.3), Lng: f(69.2)},
		CustomerComment: "Позвоните\n\n\n\nзаранее\x00",
	}
	if errs := in.Normalize(); errs != nil {
		t.Fatal(errs)
	}
	if in.Delivery.Street != "Амира Темура" || in.Delivery.Entrance != "3" || in.CustomerComment != "Позвоните\n\nзаранее" || in.CustomerName != "Азиз" {
		t.Fatalf("bad clean: %+v", in)
	}

	id := uuid.New()
	bad := checkoutReq{
		Items:    []checkoutItem{{ProductID: id, Qty: 0}, {ProductID: id, Qty: 1}},
		Delivery: deliveryReq{Street: strings.Repeat("я", MaxStreet+1), Lat: f(91), Lng: f(0)},
	}
	errs := bad.Normalize()
	want := map[string]string{"customer_name": "required", "delivery.street": "max", "delivery.house": "required",
		"delivery.location": "out_of_range", "items[0].qty": "range", "items[1].product_id": "duplicate"}
	for k, v := range want {
		if errs[k] != v {
			t.Errorf("%s: got %q want %q (all: %v)", k, errs[k], v, errs)
		}
	}
	half := checkoutReq{Items: []checkoutItem{{ProductID: uuid.New(), Qty: 1}}, CustomerName: "a",
		Delivery: deliveryReq{Street: "a", House: "1", Lat: f(1)}}
	if errs := half.Normalize(); errs["delivery.location"] != "incomplete" {
		t.Errorf("half coords: %v", errs)
	}
	if errs := (&checkoutReq{}).Normalize(); errs["items"] != "required" {
		t.Errorf("empty items: %v", errs)
	}
}

func TestCopyText(t *testing.T) {
	d := DeliveryView{Street: "Навои", House: "5", Entrance: "2", Floor: "7", AddressComment: "код 12\nвторой вход",
		Lat: f(41.311081), Lng: f(69.240562), HasPhoto: true, PhotoURL: "https://secret.example/signed"}
	d.MapsURL = mapsURL(d.Lat, d.Lng)
	got := CopyText(100001, "Азиз", "+998901234567", "после 18:00", d)
	for _, want := range []string{"Заказ: №100001", "Получатель: Азиз", "Телефон: +998901234567", "Адрес: Навои, дом 5",
		"Подъезд: 2", "Этаж: 7", "Ориентир / комментарий: код 12 второй вход", "Время доставки: после 18:00",
		"Локация: https://maps.google.com/?q=41.311081,69.240562", "Фото места: есть"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Квартира") || strings.Contains(got, "Домофон") {
		t.Errorf("empty fields must be skipped:\n%s", got)
	}
	if strings.Contains(got, "secret.example") {
		t.Error("signed photo URL must not leak into copy text")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	if _, _, err := decodeCursor("!!"); err == nil {
		t.Error("bad cursor accepted")
	}
}
