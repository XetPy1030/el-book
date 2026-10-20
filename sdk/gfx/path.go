package gfx

import (
	"image"
	"math"

	"golang.org/x/image/vector"
)

type pathOp struct {
	kind byte // 'M', 'L', 'Q', 'C', 'Z'
	pts  [3]Point
}

// Path — контур из отрезков и кривых для заливки со сглаживанием.
// Самопересечения и вложенные контуры закрашиваются по правилу ненулевой обмотки.
type Path struct {
	ops []pathOp
}

func (p *Path) MoveTo(x, y float64) { p.ops = append(p.ops, pathOp{kind: 'M', pts: [3]Point{{x, y}}}) }
func (p *Path) LineTo(x, y float64) { p.ops = append(p.ops, pathOp{kind: 'L', pts: [3]Point{{x, y}}}) }
func (p *Path) QuadTo(cx, cy, x, y float64) {
	p.ops = append(p.ops, pathOp{kind: 'Q', pts: [3]Point{{cx, cy}, {x, y}}})
}
func (p *Path) CubeTo(c1x, c1y, c2x, c2y, x, y float64) {
	p.ops = append(p.ops, pathOp{kind: 'C', pts: [3]Point{{c1x, c1y}, {c2x, c2y}, {x, y}}})
}
func (p *Path) Close() { p.ops = append(p.ops, pathOp{kind: 'Z'}) }

// Polygon добавляет замкнутый многоугольник.
func (p *Path) Polygon(pts ...Point) {
	if len(pts) == 0 {
		return
	}
	p.MoveTo(pts[0].X, pts[0].Y)
	for _, q := range pts[1:] {
		p.LineTo(q.X, q.Y)
	}
	p.Close()
}

// kappa — плечо кубической кривой, приближающей четверть окружности.
const kappa = 0.5522847498

// Circle добавляет окружность (по часовой стрелке).
func (p *Path) Circle(cx, cy, r float64) {
	k := r * kappa
	p.MoveTo(cx+r, cy)
	p.CubeTo(cx+r, cy+k, cx+k, cy+r, cx, cy+r)
	p.CubeTo(cx-k, cy+r, cx-r, cy+k, cx-r, cy)
	p.CubeTo(cx-r, cy-k, cx-k, cy-r, cx, cy-r)
	p.CubeTo(cx+k, cy-r, cx+r, cy-k, cx+r, cy)
	p.Close()
}

// CircleReverse добавляет окружность против часовой — вырез внутри другой фигуры.
func (p *Path) CircleReverse(cx, cy, r float64) {
	k := r * kappa
	p.MoveTo(cx+r, cy)
	p.CubeTo(cx+r, cy-k, cx+k, cy-r, cx, cy-r)
	p.CubeTo(cx-k, cy-r, cx-r, cy-k, cx-r, cy)
	p.CubeTo(cx-r, cy+k, cx-k, cy+r, cx, cy+r)
	p.CubeTo(cx+k, cy+r, cx+r, cy+k, cx+r, cy)
	p.Close()
}

// RoundRect добавляет прямоугольник со скруглёнными углами (по часовой стрелке).
func (p *Path) RoundRect(x0, y0, x1, y1, r float64) {
	r = math.Min(r, math.Min((x1-x0)/2, (y1-y0)/2))
	if r <= 0 {
		p.Polygon(Pt(x0, y0), Pt(x1, y0), Pt(x1, y1), Pt(x0, y1))
		return
	}
	k := r * (1 - kappa)
	p.MoveTo(x0+r, y0)
	p.LineTo(x1-r, y0)
	p.CubeTo(x1-k, y0, x1, y0+k, x1, y0+r)
	p.LineTo(x1, y1-r)
	p.CubeTo(x1, y1-k, x1-k, y1, x1-r, y1)
	p.LineTo(x0+r, y1)
	p.CubeTo(x0+k, y1, x0, y1-k, x0, y1-r)
	p.LineTo(x0, y0+r)
	p.CubeTo(x0, y0+k, x0+k, y0, x0+r, y0)
	p.Close()
}

// RoundRectReverse — то же против часовой стрелки, чтобы вырезать внутренность.
func (p *Path) RoundRectReverse(x0, y0, x1, y1, r float64) {
	if x1 <= x0 || y1 <= y0 {
		return
	}
	r = math.Min(r, math.Min((x1-x0)/2, (y1-y0)/2))
	if r <= 0 {
		p.Polygon(Pt(x0, y0), Pt(x0, y1), Pt(x1, y1), Pt(x1, y0))
		return
	}
	k := r * (1 - kappa)
	p.MoveTo(x0+r, y0)
	p.CubeTo(x0+k, y0, x0, y0+k, x0, y0+r)
	p.LineTo(x0, y1-r)
	p.CubeTo(x0, y1-k, x0+k, y1, x0+r, y1)
	p.LineTo(x1-r, y1)
	p.CubeTo(x1-k, y1, x1, y1-k, x1, y1-r)
	p.LineTo(x1, y0+r)
	p.CubeTo(x1, y0+k, x1-k, y0, x1-r, y0)
	p.Close()
}

// bounds — охватывающий прямоугольник всех точек контура.
func (p *Path) bounds() image.Rectangle {
	if len(p.ops) == 0 {
		return image.Rectangle{}
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, op := range p.ops {
		n := map[byte]int{'M': 1, 'L': 1, 'Q': 2, 'C': 3}[op.kind]
		for _, q := range op.pts[:n] {
			minX, minY = math.Min(minX, q.X), math.Min(minY, q.Y)
			maxX, maxY = math.Max(maxX, q.X), math.Max(maxY, q.Y)
		}
	}
	return image.Rect(int(math.Floor(minX)), int(math.Floor(minY)), int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1)
}

// FillPath заливает контур цветом v со сглаживанием.
func (c *Canvas) FillPath(p *Path, v uint8) {
	box := p.bounds().Intersect(c.clip)
	if box.Empty() {
		return
	}
	full := p.bounds()
	z := vector.NewRasterizer(full.Dx(), full.Dy())
	ox, oy := float32(full.Min.X), float32(full.Min.Y)
	open := false
	for _, op := range p.ops {
		switch op.kind {
		case 'M':
			if open {
				z.ClosePath()
			}
			z.MoveTo(float32(op.pts[0].X)-ox, float32(op.pts[0].Y)-oy)
			open = true
		case 'L':
			z.LineTo(float32(op.pts[0].X)-ox, float32(op.pts[0].Y)-oy)
		case 'Q':
			z.QuadTo(float32(op.pts[0].X)-ox, float32(op.pts[0].Y)-oy, float32(op.pts[1].X)-ox, float32(op.pts[1].Y)-oy)
		case 'C':
			z.CubeTo(float32(op.pts[0].X)-ox, float32(op.pts[0].Y)-oy, float32(op.pts[1].X)-ox, float32(op.pts[1].Y)-oy,
				float32(op.pts[2].X)-ox, float32(op.pts[2].Y)-oy)
		case 'Z':
			z.ClosePath()
			open = false
		}
	}
	if open {
		z.ClosePath()
	}
	mask := image.NewAlpha(image.Rect(0, 0, full.Dx(), full.Dy()))
	z.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	c.DrawMask(mask, full.Min, v)
}
