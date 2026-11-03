package tryon

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/draw"
)

// Mock — демо-провайдер без AI и без ключей (dev, тесты, демонстрация интерфейса).
// Берёт фото клиента и раскладывает фото товаров по зонам справа, сверху — полоса
// «DEMO». В prod запрещён (config.validateTryOn).
type Mock struct {
	// Fail, если задан, вызывается перед генерацией (тесты ошибок).
	Fail func(req Request) error
}

func (Mock) Name() string { return "mock" }

var zoneY = map[string]float64{ZoneHeadwear: 0.02, ZoneTop: 0.25, ZoneBottom: 0.5, ZoneShoes: 0.76}

func (m Mock) Generate(ctx context.Context, req Request) ([]byte, error) {
	if m.Fail != nil {
		if err := m.Fail(req); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	person, _, err := image.Decode(bytes.NewReader(req.Person))
	if err != nil {
		return nil, permanent(CodePhotoRejected, "person photo: %v", err)
	}
	const h = 1024
	pb := person.Bounds()
	pw := max(pb.Dx()*h/max(pb.Dy(), 1), 1)
	card := h / 5
	dst := image.NewRGBA(image.Rect(0, 0, pw+card+16, h))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(dst, image.Rect(0, 0, pw, h), person, pb, draw.Over, nil)
	for _, g := range req.Garments {
		img, _, err := image.Decode(bytes.NewReader(g.Image))
		if err != nil {
			continue
		}
		y := int(zoneY[g.Zone] * h)
		draw.ApproxBiLinear.Scale(dst, image.Rect(pw+8, y, pw+8+card, y+card), img, img.Bounds(), draw.Over, nil)
	}
	// полоса DEMO: чередующиеся полосы сверху
	for x := 0; x < dst.Bounds().Dx(); x++ {
		for y := 0; y < 12; y++ {
			if (x/12)%2 == 0 {
				dst.Set(x, y, color.RGBA{0xe8, 0x3e, 0x8c, 0xff})
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
