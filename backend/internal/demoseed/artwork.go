package demoseed

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"

	"golang.org/x/image/vector"
)

// Запасные картинки товаров. Если фото с Unsplash не скачивается (нет доступа к сайту
// из сети пользователя или из Docker), товар получает нарисованный силуэт вещи нужного
// цвета: витрина, корзина и кабинет магазина работают и без интернета.

const artW, artH = 900, 1200

// Art — что рисовать и какими цветами (#rrggbb).
type Art struct {
	Shape  string // см. shapes
	Color  string
	Color2 string // второй цвет: низ комплекта, подошва, лента шляпы; пусто — темнее Color
}

type canvas struct {
	img *image.RGBA
	// аффинное преобразование для комплектов: верх и низ рисуются уменьшенными
	sx, sy, dx, dy float32
}

func (c *canvas) pt(x, y float32) (float32, float32) { return x*c.sx + c.dx, y*c.sy + c.dy }

// fill закрашивает многоугольник (точки парами x,y в координатах 900×1200).
// Растеризатор — только на габарит фигуры: так рисование в десятки раз быстрее.
func (c *canvas) fill(col color.Color, pts ...float32) {
	xs := make([]float32, 0, len(pts)/2)
	ys := make([]float32, 0, len(pts)/2)
	for i := 0; i+1 < len(pts); i += 2 {
		x, y := c.pt(pts[i], pts[i+1])
		xs, ys = append(xs, x), append(ys, y)
	}
	minX, minY, maxX, maxY := xs[0], ys[0], xs[0], ys[0]
	for i := range xs {
		minX, maxX = min(minX, xs[i]), max(maxX, xs[i])
		minY, maxY = min(minY, ys[i]), max(maxY, ys[i])
	}
	x0, y0 := int(math.Floor(float64(minX))), int(math.Floor(float64(minY)))
	w, h := int(math.Ceil(float64(maxX)))-x0+1, int(math.Ceil(float64(maxY)))-y0+1
	if w <= 0 || h <= 0 {
		return
	}
	r := vector.NewRasterizer(w, h)
	r.MoveTo(xs[0]-float32(x0), ys[0]-float32(y0))
	for i := 1; i < len(xs); i++ {
		r.LineTo(xs[i]-float32(x0), ys[i]-float32(y0))
	}
	r.ClosePath()
	r.Draw(c.img, image.Rect(x0, y0, x0+w, y0+h), image.NewUniform(col), image.Point{})
}

func (c *canvas) rect(col color.Color, x0, y0, x1, y1 float32) {
	c.fill(col, x0, y0, x1, y0, x1, y1, x0, y1)
}

// ellipse — многоугольник из 72 точек; from/to в градусах (0 — вправо, 90 — вниз).
func (c *canvas) ellipse(col color.Color, cx, cy, rx, ry float32, from, to float64) {
	pts := []float32{}
	if to-from < 360 {
		pts = append(pts, cx, cy)
	}
	for i := 0; i <= 72; i++ {
		a := (from + (to-from)*float64(i)/72) * math.Pi / 180
		pts = append(pts, cx+rx*float32(math.Cos(a)), cy+ry*float32(math.Sin(a)))
	}
	c.fill(col, pts...)
}

// line — полоска толщиной w (швы, стрелки, складки).
func (c *canvas) line(col color.Color, w, x0, y0, x1, y1 float32) {
	dx, dy := x1-x0, y1-y0
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l == 0 {
		return
	}
	nx, ny := -dy/l*w/2, dx/l*w/2
	c.fill(col, x0+nx, y0+ny, x1+nx, y1+ny, x1-nx, y1-ny, x0-nx, y0-ny)
}

func parseHex(s string) color.RGBA {
	if len(s) == 7 && s[0] == '#' {
		if b, err := hex.DecodeString(s[1:]); err == nil {
			return color.RGBA{b[0], b[1], b[2], 255}
		}
	}
	return color.RGBA{120, 120, 120, 255}
}

func shade(c color.RGBA, k float64) color.RGBA {
	f := func(v uint8) uint8 {
		x := float64(v) * k
		if k > 1 { // осветление: к белому
			x = float64(v) + (255-float64(v))*(k-1)
		}
		return uint8(math.Max(0, math.Min(255, x)))
	}
	return color.RGBA{f(c.R), f(c.G), f(c.B), 255}
}

type palette struct{ main, dark, light, second, second2 color.RGBA }

func newPalette(a Art) palette {
	m := parseHex(a.Color)
	s := shade(m, 0.6)
	if a.Color2 != "" {
		s = parseHex(a.Color2)
	}
	return palette{main: m, dark: shade(m, 0.72), light: shade(m, 1.18), second: s, second2: shade(s, 0.75)}
}

var shapes = map[string]func(c *canvas, p palette){
	"tee":      drawTee,
	"shirt":    drawShirt,
	"hoodie":   drawHoodie,
	"sweater":  drawSweater,
	"jacket":   drawJacket,
	"coat":     drawCoat,
	"blazer":   drawBlazer,
	"top":      drawFittedTop,
	"dress":    drawDress,
	"pants":    drawPants,
	"jeans":    drawJeans,
	"skirt":    drawSkirt,
	"suit":     drawSuit,     // пиджак + брюки
	"jeansset": drawJeansSet, // джинсовая куртка + джинсы
	"skirtset": drawSkirtSet, // блузка + юбка
	"sneakers": drawSneakers,
	"boots":    drawBoots,
	"heels":    drawHeels,
	"derby":    drawDerby,
	"cap":      drawCap,
	"beanie":   drawBeanie,
	"hat":      drawHat,
}

// RenderArt рисует картинку товара (JPEG 900×1200).
func RenderArt(a Art) ([]byte, error) {
	f, ok := shapes[a.Shape]
	if !ok {
		return nil, fmt.Errorf("unknown art shape %q", a.Shape)
	}
	img := image.NewRGBA(image.Rect(0, 0, artW, artH))
	// мягкий светлый фон с лёгкой виньеткой, как у студийных фото
	for y := 0; y < artH; y++ {
		t := float64(y) / artH
		bg := color.RGBA{uint8(246 - 10*t), uint8(244 - 10*t), uint8(240 - 8*t), 255}
		draw.Draw(img, image.Rect(0, y, artW, y+1), image.NewUniform(bg), image.Point{}, draw.Src)
	}
	c := &canvas{img: img, sx: 1, sy: 1}
	c.ellipse(color.RGBA{214, 210, 204, 255}, 450, 1130, 330, 34, 0, 360) // тень на полу
	f(c, newPalette(a))
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ---------- верх ----------

func shortSleeveBody(c *canvas, p palette, bottom float32) {
	c.fill(p.dark, 300, 230, 150, 330, 95, 520, 215, 565, 300, 430, 600, 430, 685, 565, 805, 520, 750, 330, 600, 230)
	c.fill(p.main, 300, 220, 160, 320, 110, 505, 215, 545, 300, 420, 300, bottom, 600, bottom, 600, 420, 685, 545, 790, 505, 740, 320, 600, 220)
}

func longSleeveBody(c *canvas, p palette, bottom float32, cuff color.Color) {
	c.fill(p.dark, 300, 230, 160, 320, 95, 900, 205, 915, 300, 470, 600, 470, 695, 915, 805, 900, 740, 320, 600, 230)
	c.fill(p.main, 300, 220, 170, 310, 110, 880, 210, 895, 300, 450, 300, bottom, 600, bottom, 600, 450, 690, 895, 790, 880, 730, 310, 600, 220)
	c.fill(cuff, 110, 880, 210, 895, 206, 925, 104, 912)
	c.fill(cuff, 690, 895, 790, 880, 796, 912, 694, 925)
}

func neck(c *canvas, col color.Color) { c.ellipse(col, 450, 222, 92, 58, 0, 180) }

func drawTee(c *canvas, p palette) {
	shortSleeveBody(c, p, 1010)
	neck(c, p.dark)
	c.ellipse(p.main, 450, 212, 78, 44, 0, 180)
	c.ellipse(shade(p.main, 0.82), 450, 205, 70, 34, 0, 180)
	c.line(p.dark, 4, 300, 990, 600, 990)
}

func drawShirt(c *canvas, p palette) {
	longSleeveBody(c, p, 1020, p.dark)
	c.fill(p.light, 360, 200, 450, 300, 400, 330, 340, 230) // воротник
	c.fill(p.light, 540, 200, 450, 300, 500, 330, 560, 230)
	c.line(p.dark, 6, 450, 300, 450, 1020)
	for y := float32(360); y < 1000; y += 110 {
		c.ellipse(p.light, 462, y, 7, 7, 0, 360)
	}
	c.rect(p.dark, 340, 420, 420, 425)
	c.rect(p.dark, 340, 425, 344, 500)
	c.rect(p.dark, 416, 425, 420, 500)
	c.rect(p.dark, 340, 500, 420, 504)
}

func drawHoodie(c *canvas, p palette) {
	c.ellipse(p.dark, 450, 230, 150, 120, 180, 360) // капюшон
	longSleeveBody(c, p, 990, p.dark)
	c.rect(p.dark, 300, 950, 600, 1010)
	c.ellipse(p.dark, 450, 225, 95, 70, 0, 180)
	c.line(p.light, 5, 420, 280, 410, 430)
	c.line(p.light, 5, 480, 280, 490, 430)
	c.fill(p.dark, 350, 700, 550, 700, 590, 880, 310, 880) // карман-кенгуру
	c.fill(p.main, 360, 712, 540, 712, 575, 868, 325, 868)
}

func drawSweater(c *canvas, p palette) {
	longSleeveBody(c, p, 980, p.dark)
	c.rect(p.dark, 300, 940, 600, 1000)
	for x := float32(310); x < 600; x += 22 {
		c.line(p.main, 4, x, 945, x, 995)
	}
	neck(c, p.dark)
	c.ellipse(p.main, 450, 214, 80, 42, 0, 180)
	for y := float32(320); y < 900; y += 60 { // косы вязки
		c.line(p.dark, 4, 430, y, 450, y+30)
		c.line(p.dark, 4, 470, y, 450, y+30)
	}
}

func drawJacket(c *canvas, p palette) {
	longSleeveBody(c, p, 960, p.dark)
	c.rect(p.dark, 300, 920, 600, 970)
	c.fill(p.light, 350, 200, 450, 270, 420, 320, 330, 240) // воротник
	c.fill(p.light, 550, 200, 450, 270, 480, 320, 570, 240)
	c.line(p.second, 6, 450, 270, 450, 960) // молния / планка
	c.rect(p.dark, 330, 420, 420, 500)
	c.rect(p.dark, 480, 420, 570, 500)
	c.line(p.light, 3, 310, 400, 590, 400) // кокетка
}

func drawCoat(c *canvas, p palette) {
	longSleeveBody(c, p, 1090, p.dark)
	c.fill(p.dark, 450, 280, 340, 220, 380, 520) // лацканы
	c.fill(p.dark, 450, 280, 560, 220, 520, 520)
	c.rect(p.second, 300, 600, 600, 640) // пояс
	c.rect(p.second2, 430, 595, 470, 645)
	for _, y := range []float32{460, 540, 720, 800} {
		c.ellipse(p.second, 395, y, 10, 10, 0, 360)
		c.ellipse(p.second, 505, y, 10, 10, 0, 360)
	}
	c.line(p.dark, 4, 450, 640, 450, 1090)
}

func drawBlazer(c *canvas, p palette) {
	longSleeveBody(c, p, 1000, p.dark)
	c.fill(color.RGBA{245, 245, 245, 255}, 400, 215, 500, 215, 450, 560) // рубашка в вырезе
	c.fill(p.dark, 400, 215, 340, 250, 430, 560, 450, 560)               // лацканы
	c.fill(p.dark, 500, 215, 560, 250, 470, 560, 450, 560)
	c.line(p.dark, 4, 450, 560, 450, 1000)
	c.ellipse(p.second, 462, 640, 9, 9, 0, 360)
	c.ellipse(p.second, 462, 760, 9, 9, 0, 360)
	c.rect(p.dark, 330, 800, 420, 808)
	c.rect(p.dark, 480, 800, 570, 808)
	c.rect(p.dark, 500, 420, 560, 426)
}

func drawFittedTop(c *canvas, p palette) {
	c.fill(p.dark, 340, 220, 300, 260, 320, 900, 580, 900, 600, 260, 560, 220, 450, 330)
	c.fill(p.main, 345, 210, 310, 250, 330, 890, 570, 890, 590, 250, 555, 210, 450, 315)
	c.ellipse(p.dark, 450, 212, 105, 105, 0, 180)
	c.ellipse(p.main, 450, 205, 95, 94, 0, 180)
	c.fill(color.RGBA{246, 244, 240, 255}, 360, 200, 540, 200, 540, 205, 360, 205)
}

func drawDress(c *canvas, p palette) {
	c.fill(p.dark, 360, 170, 380, 170, 395, 260, 375, 260)
	c.fill(p.dark, 520, 170, 540, 170, 525, 260, 505, 260)
	c.fill(p.main, 350, 250, 550, 250, 560, 560, 340, 560)
	c.ellipse(shade(p.main, 0.85), 450, 250, 80, 50, 0, 180)
	c.fill(p.dark, 340, 560, 560, 560, 720, 1100, 180, 1100)
	c.fill(p.main, 345, 570, 555, 570, 705, 1090, 195, 1090)
	c.rect(p.dark, 338, 545, 562, 575)
	for x := float32(260); x < 680; x += 70 {
		c.line(p.dark, 4, 450+(x-450)*0.35, 600, x, 1080)
	}
	for y := float32(300); y < 540; y += 60 {
		c.ellipse(p.light, 450, y, 8, 8, 0, 360)
	}
}

// ---------- низ ----------

func drawPants(c *canvas, p palette) {
	c.rect(p.dark, 290, 180, 610, 250)
	c.fill(p.main, 295, 245, 605, 245, 625, 1110, 470, 1110, 450, 470, 430, 1110, 275, 1110)
	c.line(p.dark, 4, 450, 250, 450, 470)
	c.line(p.dark, 3, 355, 260, 352, 1105) // стрелки
	c.line(p.dark, 3, 545, 260, 548, 1105)
	c.fill(p.dark, 300, 260, 380, 260, 300, 380)
	c.fill(p.dark, 600, 260, 520, 260, 600, 380)
	for _, x := range []float32{320, 400, 500, 580} {
		c.rect(p.main, x-6, 185, x+6, 250)
	}
}

func drawJeans(c *canvas, p palette) {
	stitch := color.RGBA{214, 160, 80, 255}
	c.rect(p.dark, 290, 180, 610, 250)
	c.fill(p.main, 295, 245, 605, 245, 620, 1110, 470, 1110, 450, 480, 430, 1110, 280, 1110)
	c.fill(p.light, 330, 300, 380, 300, 370, 700, 330, 700) // потёртости
	c.fill(p.light, 520, 300, 570, 300, 570, 700, 530, 700)
	c.line(stitch, 3, 450, 250, 450, 480)
	c.line(stitch, 3, 300, 255, 400, 255)
	c.fill(p.dark, 300, 262, 390, 262, 300, 360)
	c.fill(p.dark, 600, 262, 510, 262, 600, 360)
	c.line(stitch, 3, 300, 340, 385, 265)
	c.line(stitch, 3, 600, 340, 515, 265)
	c.ellipse(color.RGBA{190, 160, 90, 255}, 450, 215, 12, 12, 0, 360)
	c.rect(p.dark, 280, 1080, 430, 1110)
	c.rect(p.dark, 470, 1080, 620, 1110)
}

func drawSkirt(c *canvas, p palette) {
	c.rect(p.dark, 320, 260, 580, 320)
	c.fill(p.main, 322, 318, 578, 318, 700, 960, 200, 960)
	for x := float32(230); x < 690; x += 46 {
		c.line(p.dark, 4, 450+(x-450)*0.55, 330, x, 955)
	}
	c.ellipse(p.light, 560, 290, 9, 9, 0, 360)
}

// ---------- комплекты (двойки) ----------

func withTransform(c *canvas, sx, sy, dx, dy float32, f func()) {
	ox, oy, odx, ody := c.sx, c.sy, c.dx, c.dy
	c.sx, c.sy, c.dx, c.dy = sx, sy, dx, dy
	f()
	c.sx, c.sy, c.dx, c.dy = ox, oy, odx, ody
}

func setOf(c *canvas, p palette, top, bottom func(*canvas, palette), bottomPal palette) {
	withTransform(c, 0.75, 0.52, 112, 470, func() { bottom(c, bottomPal) })
	withTransform(c, 0.75, 0.52, 112, -60, func() { top(c, p) })
}

func drawSuit(c *canvas, p palette) {
	setOf(c, p, drawBlazer, drawPants, p)
}

func drawJeansSet(c *canvas, p palette) {
	setOf(c, p, drawJacket, drawJeans, p)
}

func drawSkirtSet(c *canvas, p palette) {
	bp := palette{main: p.second, dark: shade(p.second, 0.72), light: shade(p.second, 1.18), second: p.main, second2: p.dark}
	setOf(c, p, drawShirt, drawSkirt, bp)
}

// ---------- обувь ----------

func sneaker(c *canvas, p palette, x, y float32) {
	sole := p.second
	c.fill(sole, x, y+150, x+440, y+150, x+460, y+120, x+450, y+95, x-10, y+95, x-20, y+120)
	c.fill(p.main, x, y+100, x+20, y, x+150, y-20, x+250, y+30, x+400, y+50, x+455, y+100)
	c.fill(p.dark, x+20, y, x+150, y-20, x+165, y+20, x+40, y+60, x+5, y+90)
	for i := float32(0); i < 4; i++ {
		c.line(p.light, 6, x+180+i*35, y+25+i*6, x+200+i*35, y+60+i*6)
	}
	c.line(p.second2, 8, x+120, y+70, x+330, y+90)
}

func drawSneakers(c *canvas, p palette) {
	back := palette{main: shade(p.main, 0.88), dark: shade(p.dark, 0.88), light: shade(p.light, 0.9), second: shade(p.second, 0.88), second2: p.second2}
	sneaker(c, back, 280, 640)
	sneaker(c, p, 160, 840)
}

func drawBoots(c *canvas, p palette) {
	boot := func(pp palette, x, y float32) {
		c.fill(pp.second, x, y+420, x+330, y+420, x+340, y+385, x-5, y+385)
		c.fill(pp.main, x, y+390, x+10, y, x+170, y, x+175, y+230, x+300, y+280, x+340, y+390)
		c.line(pp.dark, 5, x+20, y+20, x+160, y+20)
		for i := float32(0); i < 5; i++ {
			c.line(pp.light, 5, x+60, y+60+i*40, x+150, y+60+i*40)
		}
	}
	boot(palette{main: shade(p.main, 0.85), dark: p.dark, light: shade(p.light, 0.85), second: shade(p.second, 0.85)}, 380, 520)
	boot(p, 200, 620)
}

func drawHeels(c *canvas, p palette) {
	heel := func(pp palette, x, y float32) {
		c.fill(pp.main, x, y+40, x+80, y+10, x+250, y+60, x+380, y+180, x+390, y+215, x+150, y+190, x+40, y+110)
		c.fill(pp.second, x+20, y+50, x+60, y+50, x+65, y+260, x+48, y+260) // каблук
		c.ellipse(pp.dark, x+130, y+80, 85, 32, 0, 360)
	}
	heel(palette{main: shade(p.main, 0.9), dark: shade(p.dark, 0.9), second: shade(p.second, 0.9)}, 360, 620)
	heel(p, 200, 760)
}

func drawDerby(c *canvas, p palette) {
	shoe := func(pp palette, x, y float32) {
		c.fill(pp.second, x-10, y+150, x+450, y+150, x+460, y+125, x-10, y+125)
		c.fill(pp.main, x, y+130, x+10, y+20, x+120, y, x+260, y+40, x+410, y+70, x+455, y+130)
		c.ellipse(pp.dark, x+210, y+50, 70, 22, 0, 360)
		for i := float32(0); i < 4; i++ {
			c.line(pp.light, 4, x+170+i*25, y+38, x+175+i*25, y+62)
		}
		c.line(pp.dark, 4, x+20, y+110, x+440, y+118)
	}
	shoe(palette{main: shade(p.main, 0.85), dark: shade(p.dark, 0.85), light: p.light, second: shade(p.second, 0.85)}, 300, 640)
	shoe(p, 160, 840)
}

// ---------- головные уборы ----------

func drawCap(c *canvas, p palette) {
	c.fill(p.second, 470, 700, 820, 760, 800, 800, 470, 770) // козырёк
	c.ellipse(p.main, 420, 720, 240, 260, 180, 360)
	c.ellipse(p.dark, 420, 462, 18, 12, 0, 360)
	for _, x := range []float32{330, 420, 510} {
		c.line(p.dark, 4, 420, 470, x, 720)
	}
	c.rect(p.dark, 180, 705, 660, 725)
}

func drawBeanie(c *canvas, p palette) {
	c.ellipse(p.light, 450, 380, 70, 60, 0, 360) // помпон
	c.ellipse(p.main, 450, 720, 230, 300, 180, 360)
	for x := float32(250); x <= 650; x += 40 {
		c.line(p.dark, 6, 450+(x-450)*0.4, 440, x, 720)
	}
	c.rect(p.dark, 215, 700, 685, 860)
	for x := float32(225); x < 685; x += 30 {
		c.line(p.main, 8, x, 705, x, 855)
	}
}

func drawHat(c *canvas, p palette) {
	c.ellipse(p.dark, 450, 760, 380, 110, 0, 360) // поля
	c.ellipse(p.main, 450, 750, 370, 100, 0, 360)
	c.fill(p.main, 300, 760, 320, 520, 580, 520, 600, 760)
	c.ellipse(p.main, 450, 522, 130, 40, 0, 360)
	c.rect(p.second, 305, 690, 595, 740)       // лента
	for x := float32(120); x <= 780; x += 60 { // плетение соломы
		c.line(p.dark, 3, x, 720, 450+(x-450)*1.05, 800)
	}
}
