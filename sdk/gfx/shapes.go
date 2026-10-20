package gfx

import (
	"image"
	"math"
)

// Capsule рисует отрезок от (x0, y0) до (x1, y1) толщиной 2r со скруглёнными концами —
// это штрих пера. aa = false даёт только чёрное и белое: такой штрих можно обновлять
// быстрыми двухцветными режимами. Возвращает затронутую область.
func (c *Canvas) Capsule(x0, y0, x1, y1, r float64, v uint8, aa bool) image.Rectangle {
	if r <= 0 {
		return image.Rectangle{}
	}
	pad := r + 1
	box := image.Rect(
		int(math.Floor(math.Min(x0, x1)-pad)), int(math.Floor(math.Min(y0, y1)-pad)),
		int(math.Ceil(math.Max(x0, x1)+pad))+1, int(math.Ceil(math.Max(y0, y1)+pad))+1,
	)
	area := box.Intersect(c.clip)
	if area.Empty() {
		return box.Intersect(c.img.Bounds())
	}
	dx, dy := x1-x0, y1-y0
	ll := dx*dx + dy*dy
	for y := area.Min.Y; y < area.Max.Y; y++ {
		py := float64(y) + 0.5
		i := c.img.PixOffset(area.Min.X, y)
		for x := area.Min.X; x < area.Max.X; x++ {
			px := float64(x) + 0.5
			t := 0.0
			if ll > 0 {
				t = ((px-x0)*dx + (py-y0)*dy) / ll
				t = math.Max(0, math.Min(1, t))
			}
			ex, ey := px-(x0+t*dx), py-(y0+t*dy)
			d := math.Sqrt(ex*ex + ey*ey)
			if aa {
				cov := r + 0.5 - d
				if cov >= 1 {
					c.img.Pix[i] = v
				} else if cov > 0 {
					blend(&c.img.Pix[i], v, uint32(cov*255))
				}
			} else if d <= r {
				c.img.Pix[i] = v
			}
			i++
		}
	}
	return box.Intersect(c.img.Bounds())
}

// Line рисует отрезок толщиной w со сглаживанием.
func (c *Canvas) Line(p0, p1 image.Point, w float64, v uint8) image.Rectangle {
	return c.Capsule(float64(p0.X)+0.5, float64(p0.Y)+0.5, float64(p1.X)+0.5, float64(p1.Y)+0.5, w/2, v, true)
}

// Disc рисует закрашенный круг.
func (c *Canvas) Disc(cx, cy, r float64, v uint8, aa bool) image.Rectangle {
	return c.Capsule(cx, cy, cx, cy, r, v, aa)
}

// Ring рисует окружность радиуса r линией толщиной w.
func (c *Canvas) Ring(cx, cy, r, w float64, v uint8) {
	box := image.Rect(int(cx-r-w), int(cy-r-w), int(cx+r+w)+2, int(cy+r+w)+2).Intersect(c.clip)
	for y := box.Min.Y; y < box.Max.Y; y++ {
		i := c.img.PixOffset(box.Min.X, y)
		for x := box.Min.X; x < box.Max.X; x++ {
			d := math.Abs(math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - r)
			cov := w/2 + 0.5 - d
			if cov >= 1 {
				c.img.Pix[i] = v
			} else if cov > 0 {
				blend(&c.img.Pix[i], v, uint32(cov*255))
			}
			i++
		}
	}
}

// Polyline рисует ломаную со скруглёнными стыками.
func (c *Canvas) Polyline(pts []Point, w float64, v uint8) {
	if len(pts) == 1 {
		c.Disc(pts[0].X, pts[0].Y, w/2, v, true)
	}
	for i := 1; i < len(pts); i++ {
		c.Capsule(pts[i-1].X, pts[i-1].Y, pts[i].X, pts[i].Y, w/2, v, true)
	}
}

// Point — точка с дробными координатами.
type Point struct{ X, Y float64 }

// Pt — короткая запись точки.
func Pt(x, y float64) Point { return Point{x, y} }

// Arc возвращает точки дуги окружности от угла a0 до a1 (радианы, 0 — вправо, по часовой).
func Arc(cx, cy, r, a0, a1 float64) []Point {
	n := int(math.Ceil(math.Abs(a1-a0) * r / 3))
	if n < 8 {
		n = 8
	}
	pts := make([]Point, n+1)
	for i := 0; i <= n; i++ {
		a := a0 + (a1-a0)*float64(i)/float64(n)
		pts[i] = Point{cx + r*math.Cos(a), cy + r*math.Sin(a)}
	}
	return pts
}

// FillRoundRect заливает прямоугольник со скруглёнными углами. Середина заливается
// строками, сглаживание считается только в углах: кнопки рисуются без выделения памяти.
func (c *Canvas) FillRoundRect(r image.Rectangle, radius int, v uint8) {
	rad := min(radius, r.Dx()/2, r.Dy()/2)
	if rad <= 0 {
		c.Fill(r, v)
		return
	}
	c.Fill(image.Rect(r.Min.X, r.Min.Y+rad, r.Max.X, r.Max.Y-rad), v)
	c.Fill(image.Rect(r.Min.X+rad, r.Min.Y, r.Max.X-rad, r.Min.Y+rad), v)
	c.Fill(image.Rect(r.Min.X+rad, r.Max.Y-rad, r.Max.X-rad, r.Max.Y), v)
	outer := roundRect{r, float64(rad)}
	c.roundCorners(r, rad, v, func(px, py float64) float64 { return outer.coverage(px, py) })
}

// StrokeRoundRect рисует рамку со скруглёнными углами толщиной w внутрь прямоугольника.
func (c *Canvas) StrokeRoundRect(r image.Rectangle, radius, w int, v uint8) {
	if w <= 0 || r.Empty() {
		return
	}
	rad := min(radius, r.Dx()/2, r.Dy()/2)
	if rad <= 0 {
		c.StrokeRect(r, w, v)
		return
	}
	in := r.Inset(w)
	if in.Empty() {
		c.FillRoundRect(r, rad, v)
		return
	}
	c.Fill(image.Rect(r.Min.X+rad, r.Min.Y, r.Max.X-rad, r.Min.Y+w), v)
	c.Fill(image.Rect(r.Min.X+rad, r.Max.Y-w, r.Max.X-rad, r.Max.Y), v)
	c.Fill(image.Rect(r.Min.X, r.Min.Y+rad, r.Min.X+w, r.Max.Y-rad), v)
	c.Fill(image.Rect(r.Max.X-w, r.Min.Y+rad, r.Max.X, r.Max.Y-rad), v)
	outer := roundRect{r, float64(rad)}
	inner := roundRect{in, math.Max(float64(rad-w), 0)}
	c.roundCorners(r, rad, v, func(px, py float64) float64 {
		return outer.coverage(px, py) - inner.coverage(px, py)
	})
}

// roundRect — прямоугольник со скруглёнными углами в непрерывных координатах.
type roundRect struct {
	r   image.Rectangle
	rad float64
}

// coverage — доля пикселя с центром (px, py), закрытая фигурой: по расстоянию со знаком до контура.
func (rr roundRect) coverage(px, py float64) float64 {
	if rr.r.Empty() {
		return 0
	}
	x0, y0 := float64(rr.r.Min.X), float64(rr.r.Min.Y)
	x1, y1 := float64(rr.r.Max.X), float64(rr.r.Max.Y)
	qx := math.Abs(px-(x0+x1)/2) - ((x1-x0)/2 - rr.rad)
	qy := math.Abs(py-(y0+y1)/2) - ((y1-y0)/2 - rr.rad)
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - rr.rad
	return math.Max(0, math.Min(1, 0.5-d))
}

// roundCorners смешивает цвет v в четырёх угловых квадратах rad×rad прямоугольника r с покрытием cov.
func (c *Canvas) roundCorners(r image.Rectangle, rad int, v uint8, cov func(px, py float64) float64) {
	corners := [4]image.Rectangle{
		image.Rect(r.Min.X, r.Min.Y, r.Min.X+rad, r.Min.Y+rad),
		image.Rect(r.Max.X-rad, r.Min.Y, r.Max.X, r.Min.Y+rad),
		image.Rect(r.Min.X, r.Max.Y-rad, r.Min.X+rad, r.Max.Y),
		image.Rect(r.Max.X-rad, r.Max.Y-rad, r.Max.X, r.Max.Y),
	}
	for _, q := range corners {
		q = q.Intersect(c.clip)
		for y := q.Min.Y; y < q.Max.Y; y++ {
			i := c.img.PixOffset(q.Min.X, y)
			for x := q.Min.X; x < q.Max.X; x++ {
				if a := cov(float64(x)+0.5, float64(y)+0.5); a > 0 {
					blend(&c.img.Pix[i], v, uint32(a*255+0.5))
				}
				i++
			}
		}
	}
}
