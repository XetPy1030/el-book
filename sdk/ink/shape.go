package ink

import (
	"math"
)

// Shape — фигура, в которую превратился штрих.
type Shape int

const (
	NoShape Shape = iota
	Line
	Triangle
	Rectangle
	Ellipse
)

// Title — название фигуры для подсказки.
func (s Shape) Title() string {
	switch s {
	case Line:
		return "Линия"
	case Triangle:
		return "Треугольник"
	case Rectangle:
		return "Прямоугольник"
	case Ellipse:
		return "Эллипс"
	}
	return ""
}

// Recognize узнаёт в штрихе от руки фигуру и возвращает её точки: линию, треугольник,
// прямоугольник или эллипс. minSize — меньше этого (пиксели) фигуры не узнаются: короткий
// росчерк остаётся росчерком. Линии и стороны прямоугольника, близкие к горизонтали
// и вертикали, выравниваются.
func Recognize(pts []Point, minSize float64) (Shape, []Point) {
	if len(pts) < 2 {
		return NoShape, nil
	}
	x0, y0, x1, y1 := extent(pts)
	diag := math.Hypot(x1-x0, y1-y0)
	if diag < minSize {
		return NoShape, nil
	}
	first, last := pts[0], pts[len(pts)-1]
	length := pathLength(pts)
	gap := math.Hypot(float64(last.X-first.X), float64(last.Y-first.Y))

	// Линия: путь почти равен хорде, точки близко к ней.
	if gap > 0.8*length && maxDeviation(pts, first, last) < 0.06*gap+2 {
		a, b := SnapLine(first, last)
		return Line, []Point{a, b}
	}
	// Замкнутые фигуры: конец вернулся к началу.
	if gap > 0.25*length {
		return NoShape, nil
	}
	corners := simplify(pts, 0.07*diag)
	// Последняя вершина совпадает с первой — убираем повтор.
	if n := len(corners); n > 2 && dist(corners[0], corners[n-1]) < 0.2*diag {
		corners = corners[:n-1]
	}
	switch len(corners) {
	case 3:
		return Triangle, closePath(corners)
	case 4:
		if rect, ok := fitRect(corners); ok {
			return Rectangle, rect
		}
	}
	if e, ok := fitEllipse(pts, x0, y0, x1, y1); ok {
		return Ellipse, e
	}
	return NoShape, nil
}

func extent(pts []Point) (x0, y0, x1, y1 float64) {
	x0, y0 = math.Inf(1), math.Inf(1)
	x1, y1 = math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		x0, y0 = math.Min(x0, float64(p.X)), math.Min(y0, float64(p.Y))
		x1, y1 = math.Max(x1, float64(p.X)), math.Max(y1, float64(p.Y))
	}
	return
}

func dist(a, b Point) float64 { return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)) }

func pathLength(pts []Point) float64 {
	l := 0.0
	for i := 1; i < len(pts); i++ {
		l += dist(pts[i-1], pts[i])
	}
	return l
}

// segDist — расстояние от p до отрезка ab.
func segDist(p, a, b Point) float64 {
	ax, ay := float64(a.X), float64(a.Y)
	dx, dy := float64(b.X)-ax, float64(b.Y)-ay
	px, py := float64(p.X), float64(p.Y)
	ll := dx*dx + dy*dy
	t := 0.0
	if ll > 0 {
		t = math.Max(0, math.Min(1, ((px-ax)*dx+(py-ay)*dy)/ll))
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

func maxDeviation(pts []Point, a, b Point) float64 {
	m := 0.0
	for _, p := range pts {
		m = math.Max(m, segDist(p, a, b))
	}
	return m
}

// simplify — упрощение ломаной Дугласа — Пекера: остаются вершины, отстоящие больше eps.
func simplify(pts []Point, eps float64) []Point {
	if len(pts) < 3 {
		return append([]Point(nil), pts...)
	}
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	var rec func(i, j int)
	rec = func(i, j int) {
		best, at := 0.0, -1
		for k := i + 1; k < j; k++ {
			if d := segDist(pts[k], pts[i], pts[j]); d > best {
				best, at = d, k
			}
		}
		if at >= 0 && best > eps {
			keep[at] = true
			rec(i, at)
			rec(at, j)
		}
	}
	// Замкнутый путь: хорда первой и последней точки вырождена, поэтому сначала делим
	// путь в самой дальней от начала точке.
	far, at := 0.0, len(pts)/2
	for k := range pts {
		if d := dist(pts[k], pts[0]); d > far {
			far, at = d, k
		}
	}
	keep[at] = true
	rec(0, at)
	rec(at, len(pts)-1)
	var out []Point
	for k, p := range pts {
		if keep[k] {
			out = append(out, p)
		}
	}
	return out
}

func closePath(pts []Point) []Point {
	out := make([]Point, 0, len(pts)+1)
	for _, p := range pts {
		p.P = 1
		out = append(out, p)
	}
	return append(out, out[0])
}

// SnapLine выравнивает линию, близкую к горизонтали, вертикали или диагонали.
func SnapLine(a, b Point) (Point, Point) {
	a.P, b.P = 1, 1
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	angle := math.Atan2(dy, dx)
	const snap = 6 * math.Pi / 180
	step := math.Pi / 4
	nearest := math.Round(angle/step) * step
	if math.Abs(angle-nearest) < snap {
		l := math.Hypot(dx, dy)
		b.X = a.X + float32(l*math.Cos(nearest))
		b.Y = a.Y + float32(l*math.Sin(nearest))
	}
	return a, b
}

// fitRect проверяет, что четыре вершины — прямоугольник (углы около 90°), и выравнивает
// его по осям, если стороны почти горизонтальны и вертикальны.
func fitRect(c []Point) ([]Point, bool) {
	for i := range c {
		a, b, d := c[(i+3)%4], c[i], c[(i+1)%4]
		v1x, v1y := float64(a.X-b.X), float64(a.Y-b.Y)
		v2x, v2y := float64(d.X-b.X), float64(d.Y-b.Y)
		cos := (v1x*v2x + v1y*v2y) / (math.Hypot(v1x, v1y) * math.Hypot(v2x, v2y))
		if math.Abs(cos) > 0.35 { // угол дальше 70…110°
			return nil, false
		}
	}
	tilt := math.Atan2(float64(c[1].Y-c[0].Y), float64(c[1].X-c[0].X))
	nearest := math.Round(tilt/(math.Pi/2)) * (math.Pi / 2)
	if math.Abs(tilt-nearest) < 12*math.Pi/180 {
		x0, y0, x1, y1 := extent(c)
		return closePath([]Point{
			{X: float32(x0), Y: float32(y0)}, {X: float32(x1), Y: float32(y0)},
			{X: float32(x1), Y: float32(y1)}, {X: float32(x0), Y: float32(y1)},
		}), true
	}
	return closePath(c), true
}

// fitEllipse описывает эллипс по охвату штриха и проверяет, что точки лежат близко к нему.
// Почти равные оси — окружность.
func fitEllipse(pts []Point, x0, y0, x1, y1 float64) ([]Point, bool) {
	cx, cy := (x0+x1)/2, (y0+y1)/2
	rx, ry := (x1-x0)/2, (y1-y0)/2
	if rx < 1 || ry < 1 {
		return nil, false
	}
	if math.Abs(rx-ry) < 0.12*math.Max(rx, ry) {
		rx = (rx + ry) / 2
		ry = rx
	}
	errSum := 0.0
	for _, p := range pts {
		nx, ny := (float64(p.X)-cx)/rx, (float64(p.Y)-cy)/ry
		errSum += math.Abs(math.Hypot(nx, ny) - 1)
	}
	if errSum/float64(len(pts)) > 0.18 {
		return nil, false
	}
	n := 72
	out := make([]Point, 0, n+1)
	for i := 0; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		out = append(out, Point{X: float32(cx + rx*math.Cos(a)), Y: float32(cy + ry*math.Sin(a)), P: 1})
	}
	return out, true
}
