package tryon

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // провайдеры отдают PNG

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	composeHeight = 1536 // высота каждого ракурса в итоговой картинке
	composeGap    = 24
	maxDecodePx   = 40_000_000
)

// Compose склеивает ракурсы (спереди, сбоку, сзади) в одну картинку слева направо:
// по ТЗ результат — одно изображение с тремя ракурсами, а не три отдельных.
func Compose(views [][]byte) ([]byte, error) {
	if len(views) == 0 {
		return nil, errors.New("compose: no views")
	}
	imgs := make([]image.Image, len(views))
	width := composeGap
	for i, v := range views {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(v))
		if err != nil {
			return nil, err
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxDecodePx {
			return nil, errors.New("compose: bad view size")
		}
		img, _, err := image.Decode(bytes.NewReader(v))
		if err != nil {
			return nil, err
		}
		imgs[i] = img
		width += cfg.Width*composeHeight/cfg.Height + composeGap
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, composeHeight+2*composeGap))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{0xf5, 0xf3, 0xf0, 0xff}}, image.Point{}, draw.Src)
	x := composeGap
	for _, img := range imgs {
		b := img.Bounds()
		w := b.Dx() * composeHeight / b.Dy()
		r := image.Rect(x, composeGap, x+w, composeGap+composeHeight)
		draw.CatmullRom.Scale(dst, r, img, b, draw.Over, nil)
		x += w + composeGap
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
