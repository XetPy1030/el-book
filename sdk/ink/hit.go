package ink

import (
	"image"
	"math"
)

// Hit сообщает, задевает ли штрих круг радиуса r с центром (x, y): для ластика по штриху.
func (s *Stroke) Hit(x, y, r float64) bool {
	b := s.Bounds()
	if x < float64(b.Min.X)-r || x > float64(b.Max.X)+r || y < float64(b.Min.Y)-r || y > float64(b.Max.Y)+r {
		return false
	}
	p := Point{X: float32(x), Y: float32(y)}
	half := float64(s.Width) / 2
	if len(s.Pts) == 1 {
		return dist(p, s.Pts[0]) <= r+half*float64(s.factor(0))
	}
	for i := 1; i < len(s.Pts); i++ {
		w := half * float64(max(s.factor(i-1), s.factor(i)))
		if segDist(p, s.Pts[i-1], s.Pts[i]) <= r+w {
			return true
		}
	}
	return false
}

// Lasso — замкнутый контур выделения.
type Lasso []Point

// Contains — лежит ли точка внутри контура (правило чёт-нечет).
func (l Lasso) Contains(x, y float64) bool {
	in := false
	for i, j := 0, len(l)-1; i < len(l); j, i = i, i+1 {
		xi, yi := float64(l[i].X), float64(l[i].Y)
		xj, yj := float64(l[j].X), float64(l[j].Y)
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// Selects — попадает ли штрих в выделение: внутри контура не меньше двух третей его точек.
func (l Lasso) Selects(s *Stroke) bool {
	if len(l) < 3 || len(s.Pts) == 0 {
		return false
	}
	lb := l.bounds()
	if !lb.Overlaps(s.Bounds()) {
		return false
	}
	inside := 0
	for _, p := range s.Pts {
		if l.Contains(float64(p.X), float64(p.Y)) {
			inside++
		}
	}
	return inside*3 >= len(s.Pts)*2
}

func (l Lasso) bounds() image.Rectangle {
	if len(l) == 0 {
		return image.Rectangle{}
	}
	x0, y0, x1, y1 := extent(l)
	return image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1))+1, int(math.Ceil(y1))+1)
}

// Union — общий охват штрихов.
func Union(strokes []*Stroke) image.Rectangle {
	var r image.Rectangle
	for _, s := range strokes {
		r = r.Union(s.Bounds())
	}
	return r
}
