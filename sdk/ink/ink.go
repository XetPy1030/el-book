// Package ink — рукописные штрихи: модель, кисти, сглаживание ввода пальца, распознавание
// фигур, попадание (ластик по штриху, лассо) и компактный двоичный формат. Рисует на
// gfx.Canvas и ничего не знает об экране и приложениях: на нём стоит рисовалка, его же
// могут взять заметки от руки.
//
// Штрих рисуется по точкам: DrawPoint(i) кладёт всё, что добавила точка i, поэтому след под
// пальцем рисуется по кусочку, а чистовая перерисовка (Draw) даёт ту же картинку целиком.
// Случайность процедурных кистей берётся из Seed штриха — картинка повторяется точно.
package ink

import (
	"image"
	"math"

	"el-book/sdk/gfx"
)

// Point — точка штриха.
type Point struct {
	X, Y float32 // пиксели листа
	P    float32 // толщина в точке относительно Stroke.Width; 1 — номинальная
}

// Brush — кисть.
type Brush uint8

const (
	Pen      Brush = iota // ровная линия
	Fountain              // перьевая ручка: быстрее движение — тоньше линия
	Marker                // маркер: широкий, только затемняет — не перекрывает чёрное
	Pencil                // карандаш: с зерном, как графит на бумаге
	Sketch                // набросок: тонкие штрихи к близким недавним точкам
	Web                   // паутина: связи со всеми близкими точками штриха
	Fur                   // мех: короткие ворсинки поперёк близких точек
	Eraser                // ластик: возвращает фон листа
	brushCount
)

// Brushes — кисти для рисования, в порядке показа (ластик — отдельный инструмент).
var Brushes = []Brush{Pen, Fountain, Marker, Pencil, Sketch, Web, Fur}

// Title — название кисти для меню.
func (b Brush) Title() string {
	switch b {
	case Fountain:
		return "Перо"
	case Marker:
		return "Маркер"
	case Pencil:
		return "Карандаш"
	case Sketch:
		return "Набросок"
	case Web:
		return "Паутина"
	case Fur:
		return "Мех"
	case Eraser:
		return "Ластик"
	}
	return "Ручка"
}

// Dynamic — толщина кисти зависит от скорости: точки нужно снимать со Smoother.Speed.
func (b Brush) Dynamic() bool { return b == Fountain }

// Stroke — штрих.
type Stroke struct {
	Brush Brush
	Width float32 // номинальная толщина, пиксели
	Tone  uint8   // цвет: 0 — чёрный, 255 — белый
	Seed  uint32  // зерно случайности процедурных кистей
	Pts   []Point

	box image.Rectangle // охват; пустой — пересчитать
}

// Bounds — охватывающий прямоугольник штриха с толщиной и ворсом.
func (s *Stroke) Bounds() image.Rectangle {
	if s.box.Empty() && len(s.Pts) > 0 {
		s.box = s.computeBounds()
	}
	return s.box
}

// Changed сбрасывает охват: точки штриха добавились или сдвинулись.
func (s *Stroke) Changed() { s.box = image.Rectangle{} }

func (s *Stroke) computeBounds() image.Rectangle {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	maxP := 0.0
	for _, p := range s.Pts {
		minX, minY = math.Min(minX, float64(p.X)), math.Min(minY, float64(p.Y))
		maxX, maxY = math.Max(maxX, float64(p.X)), math.Max(maxY, float64(p.Y))
		maxP = math.Max(maxP, float64(p.P))
	}
	pad := float64(s.Width)/2*math.Max(maxP, 1) + 2
	if s.Brush == Fur {
		pad += reach(s) / 2
	}
	return image.Rect(int(math.Floor(minX-pad)), int(math.Floor(minY-pad)),
		int(math.Ceil(maxX+pad))+1, int(math.Ceil(maxY+pad))+1)
}

// Clone — глубокая копия: для отмены и переноса.
func (s *Stroke) Clone() *Stroke {
	c := *s
	c.Pts = append([]Point(nil), s.Pts...)
	return &c
}

// Translate сдвигает штрих.
func (s *Stroke) Translate(dx, dy float32) {
	for i := range s.Pts {
		s.Pts[i].X += dx
		s.Pts[i].Y += dy
	}
	s.Changed()
}

// Draw рисует штрих целиком.
func (s *Stroke) Draw(c *gfx.Canvas, q Quality) {
	for i := range s.Pts {
		s.DrawPoint(c, i, q)
	}
}

// Seg — отрезок, из которых складывается штрих: всё, что рисует кисть, сводится к ним.
// Растр рисует отрезки капсулами, экспорт в SVG и PDF — линиями.
type Seg struct {
	X0, Y0, X1, Y1 float32
	R0, R1         float32 // радиусы в концах
	Tone           uint8
	Darken         bool   // только затемняет: маркер и связи процедурных кистей
	Grain          uint32 // не 0 — зерно карандаша (только в растре)
}

// DrawPoint рисует то, что добавила точка i: отрезок от предыдущей точки и связи процедурной
// кисти. Возвращает задетую область холста.
func (s *Stroke) DrawPoint(c *gfx.Canvas, i int, q Quality) image.Rectangle {
	box := image.Rectangle{}
	s.Segs(i, func(g Seg) { box = box.Union(DrawSeg(c, g, q)) })
	return box
}

// DrawSeg рисует один отрезок: им же рисуется разметка листа. Возвращает задетую область.
func DrawSeg(c *gfx.Canvas, g Seg, q Quality) image.Rectangle {
	mode := normal
	if g.Darken {
		mode = darken
	}
	sp := newSpan(c, g.Tone, mode, q)
	sp.grain = g.Grain
	return sp.capsule(float64(g.X0), float64(g.Y0), float64(g.X1), float64(g.Y1), float64(g.R0), float64(g.R1))
}

// Segs отдаёт отрезки точки i: основной от предыдущей точки и связи процедурной кисти.
func (s *Stroke) Segs(i int, emit func(Seg)) {
	if i < 0 || i >= len(s.Pts) {
		return
	}
	base := Seg{Tone: s.Tone}
	w := float32(s.Width)
	switch s.Brush {
	case Marker:
		base.Darken = true
	case Pencil:
		base.Grain = s.Seed | 1
	case Sketch, Web, Fur:
		base.Darken = true
		w = max(w, 1.5)
	}
	p := s.Pts[i]
	o := p
	if i > 0 {
		o = s.Pts[i-1]
	}
	g := base
	g.X0, g.Y0, g.X1, g.Y1 = o.X, o.Y, p.X, p.Y
	g.R0, g.R1 = w/2*s.factor(max(i-1, 0)), w/2*s.factor(i)
	if i == 0 {
		g.R0 = g.R1
	}
	emit(g)
	switch s.Brush {
	case Sketch, Web, Fur:
		s.links(i, emit)
	}
}

// factor — множитель толщины в точке i: у пера — по скорости, у остальных — 1.
func (s *Stroke) factor(i int) float32 {
	if s.Brush != Fountain || s.Pts[i].P <= 0 {
		return 1
	}
	return s.Pts[i].P
}

// reach — на каком расстоянии точки штриха связываются процедурной кистью.
func reach(s *Stroke) float64 {
	return math.Max(float64(s.Width)*14, 40)
}

// lookback — с каким числом прежних точек связывается новая: дальше картинка почти не
// меняется, а время растёт квадратично.
func lookback(b Brush) int {
	switch b {
	case Sketch:
		return 60
	case Fur:
		return 120
	}
	return 400
}

// links — связи точки i с близкими прежними точками (идея кистей harmony).
func (s *Stroke) links(i int, emit func(Seg)) {
	p := s.Pts[i]
	lim := reach(s)
	lim2 := lim * lim
	// Связи светлее основной линии: из многих тонких линий складывается тон.
	tone := s.Tone + uint8(float64(255-int(s.Tone))*0.55)
	r := float32(math.Max(float64(s.Width)/3, 1.2)) / 2
	line := func(x0, y0, x1, y1 float64) {
		emit(Seg{X0: float32(x0), Y0: float32(y0), X1: float32(x1), Y1: float32(y1), R0: r, R1: r, Tone: tone, Darken: true})
	}
	px, py := float64(p.X), float64(p.Y)
	for j := max(0, i-lookback(s.Brush)); j < i-1; j++ {
		o := s.Pts[j]
		ox, oy := float64(o.X), float64(o.Y)
		dx, dy := ox-px, oy-py
		d2 := dx*dx + dy*dy
		if d2 >= lim2 {
			continue
		}
		chance := float64(hash(s.Seed, uint32(i), uint32(j)))
		switch s.Brush {
		case Sketch:
			if chance >= d2/lim2*65535 {
				line(px+dx*0.3, py+dy*0.3, ox-dx*0.3, oy-dy*0.3)
			}
		case Web:
			if chance < 65535*0.4 {
				line(px+dx*0.2, py+dy*0.2, ox-dx*0.2, oy-dy*0.2)
			}
		case Fur:
			if chance >= d2/lim2*65535 {
				line(px+dx*0.5, py+dy*0.5, px-dx*0.5, py-dy*0.5)
			}
		}
	}
}

// hash — детерминированная случайность 0…65535 от зерна и номеров точек.
func hash(seed, a, b uint32) uint32 {
	h := seed*0x9E3779B1 ^ a*0x85EBCA77 ^ b*0xC2B2AE3D
	h ^= h >> 15
	h *= 0x2C1B3C6D
	h ^= h >> 12
	h *= 0x297A2D39
	h ^= h >> 15
	return h & 0xFFFF
}
