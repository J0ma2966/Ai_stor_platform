package orders

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Лимиты длины (в символах) совпадают с CHECK-ограничениями в 00001_init.sql.
const (
	MaxCustomerName = 120
	MaxStreet       = 200
	MaxHouse        = 30
	MaxEntrance     = 20
	MaxFloor        = 10
	MaxApartment    = 20
	MaxIntercom     = 30
	MaxAddrComment  = 1000
	MaxOrderComment = 1000
	MaxTimeComment  = 500
	MaxReason       = 500
	MaxItems        = 50
	MaxQty          = 99
)

// clean убирает управляющие символы и bidi-override, нормализует пробелы.
// Текст хранится как plain text; HTML-экранирование — при выводе (React экранирует сам).
func clean(s string, multiline bool) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' && multiline:
			b.WriteRune(r)
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(' ')
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			// управляющие и невидимые символы форматирования (в т.ч. U+202E)
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if multiline {
		// не больше двух переводов строки подряд
		for strings.Contains(out, "\n\n\n") {
			out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
		}
	}
	return out
}

func trimmedLen(s string) int { return utf8.RuneCountInString(strings.TrimSpace(s)) }

type fieldErrs map[string]string

func (f fieldErrs) text(name string, p *string, max int, required, multiline bool) {
	*p = clean(*p, multiline)
	n := utf8.RuneCountInString(*p)
	switch {
	case required && n == 0:
		f[name] = "required"
	case n > max:
		f[name] = "max"
	}
}

func validCoord(v, lim float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= -lim && v <= lim
}

// Normalize чистит ввод оформления заказа и возвращает ошибки по полям.
func (in *checkoutReq) Normalize() map[string]string {
	f := fieldErrs{}
	d := &in.Delivery
	f.text("customer_name", &in.CustomerName, MaxCustomerName, true, false)
	f.text("customer_comment", &in.CustomerComment, MaxOrderComment, false, true)
	f.text("delivery_time_comment", &in.DeliveryTimeComment, MaxTimeComment, false, true)
	f.text("delivery.street", &d.Street, MaxStreet, true, false)
	f.text("delivery.house", &d.House, MaxHouse, true, false)
	f.text("delivery.entrance", &d.Entrance, MaxEntrance, false, false)
	f.text("delivery.floor", &d.Floor, MaxFloor, false, false)
	f.text("delivery.apartment", &d.Apartment, MaxApartment, false, false)
	f.text("delivery.intercom", &d.Intercom, MaxIntercom, false, false)
	f.text("delivery.address_comment", &d.AddressComment, MaxAddrComment, false, true)
	in.CustomerPhone = strings.TrimSpace(in.CustomerPhone)

	if (d.Lat == nil) != (d.Lng == nil) {
		f["delivery.location"] = "incomplete"
	} else if d.Lat != nil && (!validCoord(*d.Lat, 90) || !validCoord(*d.Lng, 180)) {
		f["delivery.location"] = "out_of_range"
	}
	if len(in.Items) == 0 {
		f["items"] = "required"
	} else if len(in.Items) > MaxItems {
		f["items"] = "max"
	}
	seen := map[string]bool{}
	for i, it := range in.Items {
		if it.Qty < 1 || it.Qty > MaxQty {
			f[fmt.Sprintf("items[%d].qty", i)] = "range"
		}
		if seen[it.ProductID.String()] {
			f[fmt.Sprintf("items[%d].product_id", i)] = "duplicate"
		}
		seen[it.ProductID.String()] = true
	}
	if len(f) == 0 {
		return nil
	}
	return f
}
