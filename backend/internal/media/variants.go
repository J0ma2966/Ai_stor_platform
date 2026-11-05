package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
)

// Уменьшенные копии фото товара. Оригинал (до 4096 px, 1–3 МБ) нужен AI-примерке,
// а витрине он слишком тяжёлый: 24 карточки каталога весили бы десятки мегабайт.
const (
	VariantCard  = 800  // карточка каталога, корзина, миниатюры (300×400 CSS на экранах 2x)
	VariantLarge = 1600 // большое фото на странице товара (600×800 CSS на экранах 2x)
	// качество копий ниже, чем у оригинала: на уменьшенном фото разницы не видно
	VariantQuality = 80
)

// ProductVariants — какие копии строятся для каждого фото товара.
var ProductVariants = []int{VariantCard, VariantLarge}

// VariantKey — ключ копии рядом с оригиналом: pub/u/<owner>/product/<hex>_800.jpg.
// Такой ключ не проходит OwnsKey, поэтому его нельзя привязать к товару вместо оригинала.
func VariantKey(key string, side int) string {
	return fmt.Sprintf("%s_%d.jpg", strings.TrimSuffix(key, ".jpg"), side)
}

// MakeVariants строит копии из уже очищенного оригинала (результат Process).
// Если оригинал меньше копии, копия — тот же размер, но перекодированная с VariantQuality.
func MakeVariants(original []byte, sides []int) (map[int][]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return nil, ErrCorrupt
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(original))
	if err != nil {
		return nil, ErrCorrupt
	}
	out := make(map[int][]byte, len(sides))
	// от большей копии к меньшей: каждая следующая уменьшается из предыдущей (быстрее)
	src := img
	for i := len(sides) - 1; i >= 0; i-- {
		src = fit(src, sides[i])
		var b bytes.Buffer
		if err := jpeg.Encode(&b, src, &jpeg.Options{Quality: VariantQuality}); err != nil {
			return nil, err
		}
		out[sides[i]] = b.Bytes()
	}
	return out, nil
}
