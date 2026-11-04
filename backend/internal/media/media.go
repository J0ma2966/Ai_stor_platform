// Package media — единый модуль загрузки изображений (фото товаров, фото клиента,
// фото места доставки). Всё, что загружают пользователи, проходит через Process:
// проверка сигнатуры (magic bytes), лимиты размера и разрешения, перекодирование
// в JPEG — это удаляет EXIF/GPS и любые встроенные данные (поворот из EXIF Orientation
// применяется до этого). SVG/HTML не принимаются.
package media

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // декодер PNG
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // декодер WebP
)

type Purpose string

const (
	PurposeProductImage  Purpose = "product"
	PurposeBodyPhoto     Purpose = "body"     // фото клиента для примерки (приватно)
	PurposeDeliveryPhoto Purpose = "delivery" // фото места доставки
	PurposeStoreBranding Purpose = "store"    // логотип/обложка
	// PurposeTryOnResult — результат AI-примерки. Создаётся только воркером, загрузить нельзя.
	PurposeTryOnResult Purpose = "tryon"
)

// Biometric: фото тела клиента и результаты примерки — биометрические персональные
// данные. Они хранятся только в хранилище с локализацией в Узбекистане (см. Routed).
func (p Purpose) Biometric() bool { return p == PurposeBodyPhoto || p == PurposeTryOnResult }

// IsBiometricKey определяет по ключу (<vis>/u/<owner>/<purpose>/<rand>.jpg), что объект биометрический.
func IsBiometricKey(key string) bool {
	parts := strings.Split(key, "/")
	return len(parts) == 5 && parts[1] == "u" && Purpose(parts[3]).Biometric()
}

// Public: фото товаров и оформление магазина публичны — отдаются по постоянному URL
// через CDN (кэшируются). Фото клиента и места доставки — только по подписанной ссылке.
func (p Purpose) Public() bool { return p == PurposeProductImage || p == PurposeStoreBranding }

func visibility(p Purpose) string {
	if p.Public() {
		return "pub"
	}
	return "priv"
}

func (p Purpose) Valid() bool {
	switch p {
	case PurposeProductImage, PurposeBodyPhoto, PurposeDeliveryPhoto, PurposeStoreBranding, PurposeTryOnResult:
		return true
	}
	return false
}

const (
	// Защита от «декомпрессионной бомбы»: 20 Мп покрывают фото любого телефона (обычно 12 Мп),
	// память на одно декодирование ~80 МБ (RGBA) + до ~67 МБ на уменьшение.
	MaxPixels   = 20_000_000
	MaxSide     = 4096 // длинная сторона после обработки
	JPEGQuality = 85
)

var (
	ErrTooLarge    = errors.New("file too large")
	ErrUnsupported = errors.New("unsupported image type")
	ErrCorrupt     = errors.New("image is corrupt")
)

var allowed = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// Process читает не больше maxBytes, проверяет тип и возвращает очищенный JPEG.
func Process(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrTooLarge
	}
	if !allowed[http.DetectContentType(data)] {
		return nil, ErrUnsupported
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrCorrupt
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return nil, ErrTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrCorrupt
	}
	img = orient(fit(img, MaxSide), jpegOrientation(data))
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: JPEGQuality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func fit(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return src
	}
	if w >= h {
		h, w = h*max/w, max
	} else {
		w, h = w*max/h, max
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// NewKey — случайный непредсказуемый ключ, привязанный к владельцу:
// <pub|priv>/u/<owner>/<purpose>/<random>.jpg. Владелец проверяется через OwnsKey.
// Публичным в бакете делается только префикс pub/.
func NewKey(owner uuid.UUID, p Purpose) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s/u/%s/%s/%s.jpg", visibility(p), owner, p, hex.EncodeToString(b[:]))
}

// PublicURL — постоянный URL публичного файла (CDN). Для приватных ключей — пусто.
func PublicURL(base, key string) string {
	if !strings.HasPrefix(key, "pub/") {
		return ""
	}
	return strings.TrimRight(base, "/") + "/" + key
}

// OwnsKey: ключ загружен этим владельцем и для этой цели. Использовать при
// привязке ключа к сущности (заказ, товар), чтобы нельзя было подставить чужой файл.
func OwnsKey(owner uuid.UUID, p Purpose, key string) bool {
	prefix := fmt.Sprintf("%s/u/%s/%s/", visibility(p), owner, p)
	rest, ok := strings.CutPrefix(key, prefix)
	return ok && len(rest) == 32+4 && strings.HasSuffix(rest, ".jpg") && isHex(rest[:32])
}

func isHex(s string) bool {
	_, err := hex.DecodeString(s)
	return err == nil
}
