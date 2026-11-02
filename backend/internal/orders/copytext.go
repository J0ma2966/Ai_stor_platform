package orders

import (
	"fmt"
	"strings"
)

// DeliveryView — данные доставки, которые видит магазин этого заказа.
type DeliveryView struct {
	Street         string   `json:"street"`
	House          string   `json:"house"`
	Entrance       string   `json:"entrance,omitempty"`
	Floor          string   `json:"floor,omitempty"`
	Apartment      string   `json:"apartment,omitempty"`
	Intercom       string   `json:"intercom,omitempty"`
	AddressComment string   `json:"address_comment,omitempty"`
	Lat            *float64 `json:"lat,omitempty"`
	Lng            *float64 `json:"lng,omitempty"`
	MapsURL        string   `json:"maps_url,omitempty"`
	PhotoURL       string   `json:"photo_url,omitempty"` // подписанная ссылка, живёт PhotoURLTTL
	HasPhoto       bool     `json:"has_photo"`
	photoKey       string
}

func mapsURL(lat, lng *float64) string {
	if lat == nil || lng == nil {
		return ""
	}
	return fmt.Sprintf("https://maps.google.com/?q=%.6f,%.6f", *lat, *lng)
}

// CopyText — текст для кнопки «Скопировать данные доставки» (передать курьеру).
// Значения уже очищены от управляющих символов; результат — plain text,
// фронт кладёт его в буфер обмена / textContent, никогда в innerHTML.
// Подписанная ссылка на фото в текст не попадает: она короткоживущая
// и не должна уходить в сторонние мессенджеры.
func CopyText(number int64, customerName, customerPhone, timeComment string, d DeliveryView) string {
	var b strings.Builder
	line := func(label, v string) {
		v = strings.TrimSpace(strings.ReplaceAll(v, "\n", " "))
		if v != "" {
			b.WriteString(label)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	line("Заказ", fmt.Sprintf("№%d", number))
	line("Получатель", customerName)
	line("Телефон", customerPhone)
	line("Адрес", d.Street+", дом "+d.House)
	line("Подъезд", d.Entrance)
	line("Этаж", d.Floor)
	line("Квартира", d.Apartment)
	line("Домофон", d.Intercom)
	line("Ориентир / комментарий", d.AddressComment)
	line("Время доставки", timeComment)
	line("Локация", d.MapsURL)
	if d.HasPhoto {
		line("Фото места", "есть, см. в заказе")
	}
	return strings.TrimRight(b.String(), "\n")
}
