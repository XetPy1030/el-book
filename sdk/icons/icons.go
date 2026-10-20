// Package icons — линейные иконки платформы, нарисованные кодом на сетке 24×24 и
// масштабируемые под любой размер. Иконка рисуется цветом v поверх уже залитого фона.
package icons

import (
	"image"
	"math"

	"el-book/sdk/gfx"
)

// Drawer рисует иконку в квадрате, вписанном в r по центру.
type Drawer func(c *gfx.Canvas, r image.Rectangle, v uint8)

// pen переводит координаты сетки 24×24 в пиксели и рисует линиями одной толщины.
type pen struct {
	c      *gfx.Canvas
	ox, oy float64
	s      float64 // пикселей в единице сетки
	w      float64 // толщина линии в пикселях
	v      uint8
}

func newPen(c *gfx.Canvas, r image.Rectangle, v uint8) pen {
	size := math.Min(float64(r.Dx()), float64(r.Dy()))
	s := size / 24
	w := math.Max(2*s, 1.5)
	return pen{
		c:  c,
		ox: float64(r.Min.X) + (float64(r.Dx())-size)/2,
		oy: float64(r.Min.Y) + (float64(r.Dy())-size)/2,
		s:  s, w: w, v: v,
	}
}

func (p pen) pt(x, y float64) gfx.Point { return gfx.Pt(p.ox+x*p.s, p.oy+y*p.s) }

func (p pen) line(x0, y0, x1, y1 float64) {
	a, b := p.pt(x0, y0), p.pt(x1, y1)
	p.c.Capsule(a.X, a.Y, b.X, b.Y, p.w/2, p.v, true)
}

// poly рисует ломаную по парам координат; closed замыкает её.
func (p pen) poly(closed bool, xy ...float64) {
	pts := make([]gfx.Point, 0, len(xy)/2+1)
	for i := 0; i+1 < len(xy); i += 2 {
		pts = append(pts, p.pt(xy[i], xy[i+1]))
	}
	if closed && len(pts) > 0 {
		pts = append(pts, pts[0])
	}
	p.c.Polyline(pts, p.w, p.v)
}

func (p pen) fill(xy ...float64) {
	var path gfx.Path
	pts := make([]gfx.Point, 0, len(xy)/2)
	for i := 0; i+1 < len(xy); i += 2 {
		pts = append(pts, p.pt(xy[i], xy[i+1]))
	}
	path.Polygon(pts...)
	p.c.FillPath(&path, p.v)
}

func (p pen) ring(cx, cy, r float64) {
	c := p.pt(cx, cy)
	p.c.Ring(c.X, c.Y, r*p.s, p.w, p.v)
}

func (p pen) disc(cx, cy, r float64) {
	c := p.pt(cx, cy)
	p.c.Disc(c.X, c.Y, r*p.s, p.v, true)
}

// arc рисует дугу; углы в градусах, 0 — вправо, по часовой стрелке.
func (p pen) arc(cx, cy, r, a0, a1 float64) {
	c := p.pt(cx, cy)
	p.c.Polyline(gfx.Arc(c.X, c.Y, r*p.s, a0*math.Pi/180, a1*math.Pi/180), p.w, p.v)
}

// arrowHead рисует залитый наконечник с центром в (x, y), смотрящий по направлению (dx, dy).
func (p pen) arrowHead(x, y, dx, dy, size float64) {
	l := math.Hypot(dx, dy)
	dx, dy = dx/l, dy/l
	nx, ny := -dy, dx
	p.fill(
		x+dx*size, y+dy*size,
		x-dx*size*0.6+nx*size, y-dy*size*0.6+ny*size,
		x-dx*size*0.6-nx*size, y-dy*size*0.6-ny*size,
	)
}

func (p pen) roundRect(x0, y0, x1, y1, rad float64) {
	a, b := p.pt(x0, y0), p.pt(x1, y1)
	hw := p.w / 2
	r := image.Rect(int(math.Round(a.X-hw)), int(math.Round(a.Y-hw)), int(math.Round(b.X+hw)), int(math.Round(b.Y+hw)))
	p.c.StrokeRoundRect(r, int(math.Round(rad*p.s+hw)), int(math.Round(p.w)), p.v)
}

// Apps — четыре квадрата: список приложений.
func Apps(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(3.5, 3.5, 10, 10, 1.5)
	p.roundRect(14, 3.5, 20.5, 10, 1.5)
	p.roundRect(3.5, 14, 10, 20.5, 1.5)
	p.roundRect(14, 14, 20.5, 20.5, 1.5)
}

// Settings — три ползунка.
func Settings(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	for i, knob := range []float64{15, 8, 13} {
		y := 6 + float64(i)*6
		p.line(3, y, 21, y)
		p.disc(knob, y, 2.6)
	}
}

// Moon — полумесяц: сон.
func Moon(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	// Полумесяц — круг (12, 12, 8,5) без круга-выреза (16, 8, 7). Контур — дуга внешнего круга,
	// лежащая вне выреза, и дуга выреза, лежащая внутри внешнего круга.
	const R, cx, cy, rr, ix, iy = 8.5, 12.0, 12.0, 7.0, 16.0, 8.0
	d := math.Hypot(ix-cx, iy-cy)
	a := (R*R - rr*rr + d*d) / (2 * d)
	h := math.Sqrt(R*R - a*a)
	mx, my := cx+a*(ix-cx)/d, cy+a*(iy-cy)/d
	p1x, p1y := mx+h*(iy-cy)/d, my-h*(ix-cx)/d
	p2x, p2y := mx-h*(iy-cy)/d, my+h*(ix-cx)/d
	// arcBetween выбирает из двух дуг между точками ту, середина которой ближе к (tx, ty) (near)
	// или дальше от неё (!near).
	arcBetween := func(ox, oy, rad, tx, ty float64, near bool) {
		a1 := math.Atan2(p1y-oy, p1x-ox) * 180 / math.Pi
		a2 := math.Atan2(p2y-oy, p2x-ox) * 180 / math.Pi
		for a2 < a1 {
			a2 += 360
		}
		mid := (a1 + a2) / 2 * math.Pi / 180
		dist := math.Hypot(ox+rad*math.Cos(mid)-tx, oy+rad*math.Sin(mid)-ty)
		other := math.Hypot(ox-rad*math.Cos(mid)-tx, oy-rad*math.Sin(mid)-ty)
		if (dist < other) == near {
			p.arc(ox, oy, rad, a1, a2)
		} else {
			p.arc(ox, oy, rad, a2, a1+360)
		}
	}
	arcBetween(cx, cy, R, ix, iy, false)
	arcBetween(ix, iy, rr, cx, cy, true)
}

// Exit — скобка и стрелка наружу: выход в Nickel.
func Exit(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 12, 3.5, 4.5, 3.5, 4.5, 20.5, 12, 20.5)
	p.line(9.5, 12, 20.5, 12)
	p.poly(false, 16.5, 8, 20.5, 12, 16.5, 16)
}

// Sun — солнце: яркость подсветки.
func Sun(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.ring(12, 12, 4)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		p.line(12+7*math.Cos(a), 12+7*math.Sin(a), 12+9.5*math.Cos(a), 12+9.5*math.Sin(a))
	}
}

// Warmth — термометр: теплота подсветки.
func Warmth(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(9.5, 2.5, 14.5, 15, 2.5)
	p.disc(12, 18, 4)
	p.line(12, 8, 12, 17)
	p.line(17, 6, 19, 6)
	p.line(17, 10, 19, 10)
}

// Back — стрелка влево.
func Back(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(20, 12, 4.5, 12)
	p.poly(false, 10.5, 6, 4.5, 12, 10.5, 18)
}

// Home — дом.
func Home(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 3, 11.5, 12, 3.5, 21, 11.5)
	p.poly(false, 5.5, 9.5, 5.5, 20.5, 18.5, 20.5, 18.5, 9.5)
	p.poly(false, 10, 20.5, 10, 15, 14, 15, 14, 20.5)
}

// Close — крест.
func Close(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(5.5, 5.5, 18.5, 18.5)
	p.line(18.5, 5.5, 5.5, 18.5)
}

// Check — галочка.
func Check(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 4, 12.5, 9.5, 18, 20, 6)
}

// ChevronLeft и ChevronRight — уголки листания.
func ChevronLeft(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).poly(false, 15, 4.5, 7.5, 12, 15, 19.5)
}

func ChevronRight(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).poly(false, 9, 4.5, 16.5, 12, 9, 19.5)
}

// Plus — плюс.
func Plus(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(12, 4.5, 12, 19.5)
	p.line(4.5, 12, 19.5, 12)
}

// Trash — корзина.
func Trash(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(3.5, 6, 20.5, 6)
	p.poly(false, 9, 6, 9, 3.5, 15, 3.5, 15, 6)
	p.poly(false, 5.5, 6, 7, 20.5, 17, 20.5, 18.5, 6)
	p.line(10, 10, 10, 16.5)
	p.line(14, 10, 14, 16.5)
}

// Pencil — карандаш: перо.
func Pencil(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 3.5, 20.5, 5, 15, 15.5, 4.5, 19.5, 8.5, 9, 19)
	p.line(5, 15, 9, 19)
	p.line(13, 7, 17, 11)
}

// Eraser — ластик.
func Eraser(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 8.5, 20.5, 3.5, 15.5, 14, 5, 20.5, 11.5, 11.5, 20.5)
	p.line(8, 11, 14, 17)
	p.line(11.5, 20.5, 20.5, 20.5)
}

// Undo — стрелка назад по дуге: отменить.
func Undo(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 8.5, 4.5, 4.5, 8.5, 8.5, 12.5)
	p.line(4.5, 8.5, 14, 8.5)
	p.arc(14, 13.75, 5.25, -90, 90)
	p.line(14, 19, 7, 19)
}

// Redo — то же зеркально: повторить.
func Redo(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 15.5, 4.5, 19.5, 8.5, 15.5, 12.5)
	p.line(19.5, 8.5, 10, 8.5)
	p.arc(10, 13.75, 5.25, 90, 270)
	p.line(10, 19, 17, 19)
}

// Page — лист с загнутым углом: новый документ.
func Page(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 5.5, 3, 14, 3, 18.5, 7.5, 18.5, 21, 5.5, 21)
	p.poly(false, 14, 3, 14, 7.5, 18.5, 7.5)
}

// ClearPage — лист с крестом: очистить.
func ClearPage(c *gfx.Canvas, r image.Rectangle, v uint8) {
	Page(c, r, v)
	p := newPen(c, r, v)
	p.line(9.5, 11.5, 14.5, 16.5)
	p.line(14.5, 11.5, 9.5, 16.5)
}

// GridPaper — лист в клетку: фон.
func GridPaper(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(3.5, 3.5, 20.5, 20.5, 2)
	thin := p
	thin.w = p.w * 0.6
	for _, k := range []float64{9.2, 14.8} {
		thin.line(k, 4, k, 20)
		thin.line(4, k, 20, k)
	}
}

// Gallery — картинка с горами: галерея.
func Gallery(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(3, 4.5, 21, 19.5, 2)
	p.poly(false, 5.5, 17, 10, 11.5, 13.5, 15, 15.5, 13, 18.5, 16.5)
	p.disc(16, 8.8, 1.8)
}

// Export — лоток со стрелкой вверх: выгрузить.
func Export(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 4, 14, 4, 20, 20, 20, 20, 14)
	p.line(12, 16, 12, 4)
	p.poly(false, 8, 8, 12, 4, 16, 8)
}

// Power — кнопка питания.
func Power(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.arc(12, 13, 8, -55, 235)
	p.line(12, 3, 12, 11.5)
}

// Rotate — круговая стрелка: поворот экрана.
func Rotate(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	const a0, a1, rad = -40.0, 245.0, 7.5
	p.arc(12, 12, rad, a0, a1)
	end := a1 * math.Pi / 180
	p.arrowHead(12+rad*math.Cos(end), 12+rad*math.Sin(end), -math.Sin(end), math.Cos(end), 3.2)
}

// Laptop — ноутбук: подключение к компьютеру.
func Laptop(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(5, 5, 19, 15.5, 1.5)
	p.line(2.5, 19, 21.5, 19)
}

// Bolt — молния: зарядка.
func Bolt(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).fill(13.5, 2, 5, 13.5, 11, 13.5, 10, 22, 19, 10, 13, 10)
}

// Info — «i» в круге.
func Info(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.ring(12, 12, 9)
	p.disc(12, 7.5, 1.4)
	p.line(12, 11, 12, 17)
}

// Battery рисует батарею с уровнем level (0…100) и молнией при зарядке.
// Иконка шире обычной: она вписывается в r по высоте с пропорциями 2:1.
func Battery(level int, charging bool) Drawer {
	return func(c *gfx.Canvas, r image.Rectangle, v uint8) {
		h := r.Dy()
		w := 2 * h
		if w > r.Dx() {
			w = r.Dx()
			h = w / 2
		}
		min0 := image.Pt(r.Min.X+(r.Dx()-w)/2, r.Min.Y+(r.Dy()-h)/2)
		box := image.Rectangle{Min: min0, Max: min0.Add(image.Pt(w, h))}
		stroke := max(h/9, 1)
		nub := max(w/14, 2)
		body := image.Rect(box.Min.X, box.Min.Y+h/8, box.Max.X-nub, box.Max.Y-h/8)
		c.StrokeRoundRect(body, stroke*2, stroke, v)
		c.Fill(image.Rect(body.Max.X, body.Min.Y+body.Dy()/3, box.Max.X, body.Max.Y-body.Dy()/3), v)
		inner := body.Inset(stroke * 2)
		level = min(max(level, 0), 100)
		fill := inner
		fill.Max.X = inner.Min.X + inner.Dx()*level/100
		c.Fill(fill, v)
		if charging {
			bolt := image.Rect(inner.Min.X+inner.Dx()/2-inner.Dy(), inner.Min.Y-stroke, inner.Min.X+inner.Dx()/2+inner.Dy(), inner.Max.Y+stroke)
			back := 255 - v
			// Молния контрастом к заливке: над заполненной частью — светлая, над пустой — тёмная.
			Bolt(c.WithClip(fill), bolt, back)
			Bolt(c.WithClip(image.Rect(fill.Max.X, inner.Min.Y-stroke, inner.Max.X, inner.Max.Y+stroke)), bolt, v)
		}
	}
}
