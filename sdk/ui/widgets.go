package ui

import (
	"fmt"
	"image"
	"math"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
)

// Container — виджет с дочерними виджетами; Page раздаёт касания и детям.
type Container interface {
	Widget
	Children() []Widget
}

// Pressable позволяет виджету отказаться от инверсии при нажатии.
type Pressable interface {
	ShowsPress() bool
}

// Label — текст: одна строка с многоточием или несколько с переносом.
type Label struct {
	Box
	Text   string
	Face   func(*Theme) *gfx.Face // по умолчанию — основной шрифт
	Align  gfx.Align
	Color  uint8
	Wrap   bool
	VAlign int // 0 — по центру, -1 — сверху
}

// Height — высота подписи при ширине width: для переноса — по числу строк.
func (l *Label) Height(t *Theme, width int) int {
	face := t.Body()
	if l.Face != nil {
		face = l.Face(t)
	}
	if !l.Wrap {
		return face.Height
	}
	return len(face.Wrap(l.Text, width)) * face.Height
}

func (l *Label) Draw(c *gfx.Canvas, t *Theme) {
	face := t.Body()
	if l.Face != nil {
		face = l.Face(t)
	}
	r := l.Bounds()
	if !l.Wrap {
		face.DrawIn(c, l.Text, r, l.Align, l.Color)
		return
	}
	lines := face.Wrap(l.Text, r.Dx())
	total := len(lines) * face.Height
	y := r.Min.Y
	if l.VAlign == 0 && total < r.Dy() {
		y += (r.Dy() - total) / 2
	}
	for _, line := range lines {
		if y+face.Height > r.Max.Y+face.Descent {
			break
		}
		face.DrawIn(c, line, image.Rect(r.Min.X, y, r.Max.X, y+face.Height), l.Align, l.Color)
		y += face.Height
	}
}

// ButtonStyle — вид кнопки.
type ButtonStyle int

const (
	Secondary ButtonStyle = iota // рамка
	Primary                      // залитая чёрным
	Flat                         // без рамки: панели инструментов
)

// Button — кнопка с текстом, иконкой или и тем и другим.
type Button struct {
	Box
	Text     string
	Icon     icons.Drawer
	Style    ButtonStyle
	Vertical bool // иконка над подписью
	Active   bool // выбранный инструмент: показывается залитым
	Disabled bool
	OnTap    func()
	// Face — шрифт подписи у кнопки без иконки: крупные цифры клавиатуры калькулятора.
	// nil — основной текст, у главной кнопки полужирный.
	Face func(t *Theme) *gfx.Face
}

func (b *Button) Enabled() bool { return !b.Disabled && b.OnTap != nil }

func (b *Button) Tap(h Host, _ image.Point) {
	if b.Enabled() {
		b.OnTap()
	}
}

func (b *Button) Draw(c *gfx.Canvas, t *Theme) {
	r := b.Bounds()
	fg := gfx.Black
	switch {
	case b.Style == Primary || b.Active:
		c.FillRoundRect(r, t.Radius(), gfx.Black)
		fg = gfx.White
	case b.Style == Secondary:
		c.StrokeRoundRect(r, t.Radius(), t.Line(), gfx.Black)
	}
	if b.Disabled {
		fg = gfx.Gray16(9)
		if b.Style == Primary {
			c.FillRoundRect(r, t.Radius(), gfx.Gray16(9))
			fg = gfx.White
		}
	}
	icon := t.IconSize()
	switch {
	case b.Icon != nil && b.Text != "" && b.Vertical:
		face := t.Caption()
		block := icon + t.MM(1) + face.Height
		top := r.Min.Y + (r.Dy()-block)/2
		b.Icon(c, image.Rect(r.Min.X+(r.Dx()-icon)/2, top, r.Min.X+(r.Dx()+icon)/2, top+icon), fg)
		face.DrawIn(c, b.Text, image.Rect(r.Min.X+2, top+icon+t.MM(1), r.Max.X-2, top+block), gfx.AlignCenter, fg)
	case b.Icon != nil && b.Text != "":
		face := t.Body()
		gap := t.MM(1.5)
		w := icon + gap + face.Measure(b.Text)
		left := max(r.Min.X+(r.Dx()-w)/2, r.Min.X+t.MM(2))
		b.Icon(c, image.Rect(left, r.Min.Y+(r.Dy()-icon)/2, left+icon, r.Min.Y+(r.Dy()+icon)/2), fg)
		face.DrawIn(c, b.Text, image.Rect(left+icon+gap, r.Min.Y, r.Max.X-t.MM(1), r.Max.Y), gfx.AlignLeft, fg)
	case b.Icon != nil:
		b.Icon(c, image.Rect(r.Min.X+(r.Dx()-icon)/2, r.Min.Y+(r.Dy()-icon)/2, r.Min.X+(r.Dx()+icon)/2, r.Min.Y+(r.Dy()+icon)/2), fg)
	default:
		face := t.Body()
		if b.Style == Primary {
			face = t.Strong()
		}
		if b.Face != nil {
			face = b.Face(t)
		}
		face.DrawIn(c, b.Text, r.Inset(t.MM(1)), gfx.AlignCenter, fg)
	}
}

// Tile — плитка приложения на домашнем экране: крупная иконка и название.
type Tile struct {
	Box
	Title       string
	Icon        icons.Drawer
	OnTap       func()
	OnLongPress func()
}

func (tl *Tile) Enabled() bool { return tl.OnTap != nil }

func (tl *Tile) Tap(h Host, _ image.Point) {
	if tl.OnTap != nil {
		tl.OnTap()
	}
}

func (tl *Tile) LongPress(h Host, _ image.Point) {
	if tl.OnLongPress != nil {
		tl.OnLongPress()
	}
}

func (tl *Tile) Draw(c *gfx.Canvas, t *Theme) {
	r := tl.Bounds()
	face := t.Body()
	side := min(r.Dx()-t.MM(4), r.Dy()-face.Height-t.MM(3))
	box := image.Rect(r.Min.X+(r.Dx()-side)/2, r.Min.Y+t.MM(1), r.Min.X+(r.Dx()+side)/2, r.Min.Y+t.MM(1)+side)
	c.StrokeRoundRect(box, side/5, t.Line(), gfx.Black)
	if tl.Icon != nil {
		tl.Icon(c, box.Inset(side*22/100), gfx.Black)
	}
	face.DrawIn(c, tl.Title, image.Rect(r.Min.X, box.Max.Y+t.MM(1), r.Max.X, box.Max.Y+t.MM(1)+face.Height), gfx.AlignCenter, gfx.Black)
}

// Slider — ползунок значения от Min до Max: тап ставит значение, перетаскивание меняет его.
type Slider struct {
	Box
	Min, Max  int
	Value     int
	LeftIcon  icons.Drawer
	RightIcon icons.Drawer
	OnChange  func(v int) // во время перетаскивания
	OnCommit  func(v int) // когда палец отпущен
}

func (s *Slider) Enabled() bool    { return true }
func (s *Slider) ShowsPress() bool { return false }

func (s *Slider) track(t *Theme) (x0, x1, cy int) {
	r := s.Bounds()
	icon := t.IconSize()
	x0, x1 = r.Min.X+t.MM(2), r.Max.X-t.MM(2)
	if s.LeftIcon != nil {
		x0 += icon + t.MM(2)
	}
	if s.RightIcon != nil {
		x1 -= icon + t.MM(2)
	}
	return x0, x1, r.Min.Y + r.Dy()/2
}

func (s *Slider) valueAt(t *Theme, x int) int {
	x0, x1, _ := s.track(t)
	if x1 <= x0 {
		return s.Value
	}
	f := float64(x-x0) / float64(x1-x0)
	f = math.Max(0, math.Min(1, f))
	return s.Min + int(math.Round(f*float64(s.Max-s.Min)))
}

func (s *Slider) set(h Host, v int, final bool) {
	if v != s.Value {
		s.Value = v
		if s.OnChange != nil {
			s.OnChange(v)
		}
	}
	class := eink.Feedback
	if final {
		class = eink.UI
		if s.OnCommit != nil {
			s.OnCommit(v)
		}
	}
	h.Invalidate(s.Bounds(), class)
}

func (s *Slider) Tap(h Host, p image.Point) { s.set(h, s.valueAt(h.Theme(), p.X), true) }

func (s *Slider) Drag(h Host, g Gesture) {
	s.set(h, s.valueAt(h.Theme(), g.Pos.X), g.Kind == PanEnd)
}

func (s *Slider) Draw(c *gfx.Canvas, t *Theme) {
	r := s.Bounds()
	icon := t.IconSize()
	if s.LeftIcon != nil {
		x := r.Min.X + t.MM(2)
		s.LeftIcon(c, image.Rect(x, r.Min.Y+(r.Dy()-icon)/2, x+icon, r.Min.Y+(r.Dy()+icon)/2), gfx.Black)
	}
	if s.RightIcon != nil {
		x := r.Max.X - t.MM(2) - icon
		s.RightIcon(c, image.Rect(x, r.Min.Y+(r.Dy()-icon)/2, x+icon, r.Min.Y+(r.Dy()+icon)/2), gfx.Black)
	}
	x0, x1, cy := s.track(t)
	lw := float64(t.Line())
	c.Capsule(float64(x0), float64(cy), float64(x1), float64(cy), lw/2, gfx.Gray16(8), true)
	span := max(s.Max-s.Min, 1)
	kx := float64(x0) + float64(x1-x0)*float64(s.Value-s.Min)/float64(span)
	c.Capsule(float64(x0), float64(cy), kx, float64(cy), lw, gfx.Black, true)
	rad := float64(t.MM(2.6))
	c.Disc(kx, float64(cy), rad, gfx.Black, true)
	c.Disc(kx, float64(cy), rad-lw*1.5, gfx.White, true)
}

// Toggle — переключатель с подписью слева.
type Toggle struct {
	Box
	Text     string
	On       bool
	OnChange func(on bool)
}

func (tg *Toggle) Enabled() bool { return true }

func (tg *Toggle) Tap(h Host, _ image.Point) {
	tg.On = !tg.On
	if tg.OnChange != nil {
		tg.OnChange(tg.On)
	}
	h.Invalidate(tg.Bounds(), eink.UI)
}

func (tg *Toggle) Draw(c *gfx.Canvas, t *Theme) {
	r := tg.Bounds()
	w, hgt := t.MM(10), t.MM(5.5)
	sw := image.Rect(r.Max.X-t.MM(2)-w, r.Min.Y+(r.Dy()-hgt)/2, r.Max.X-t.MM(2), r.Min.Y+(r.Dy()+hgt)/2)
	t.Body().DrawIn(c, tg.Text, image.Rect(r.Min.X+t.MM(2), r.Min.Y, sw.Min.X-t.MM(2), r.Max.Y), gfx.AlignLeft, gfx.Black)
	rad := float64(hgt) / 2
	cy := float64(sw.Min.Y) + rad
	if tg.On {
		c.FillRoundRect(sw, hgt/2, gfx.Black)
		c.Disc(float64(sw.Max.X)-rad, cy, rad-float64(t.Line())*1.5, gfx.White, true)
	} else {
		c.StrokeRoundRect(sw, hgt/2, t.Line(), gfx.Black)
		c.Disc(float64(sw.Min.X)+rad, cy, rad-float64(t.Line())*1.5, gfx.Black, true)
	}
}

// Segmented — ряд вариантов, из которых выбран один.
type Segmented struct {
	Box
	Options  []string
	Selected int
	OnChange func(i int)
	edges    []int // границы сегментов неравной ширины; пусто — доли равные
}

func (s *Segmented) Enabled() bool    { return len(s.Options) > 0 }
func (s *Segmented) ShowsPress() bool { return false }

func (s *Segmented) SetBounds(r image.Rectangle) {
	s.R = r
	s.edges = s.edges[:0]
}

func (s *Segmented) cell(i int) image.Rectangle {
	r := s.Bounds()
	if len(s.edges) == len(s.Options)+1 {
		return image.Rect(s.edges[i], r.Min.Y, s.edges[i+1], r.Max.Y)
	}
	n := max(len(s.Options), 1)
	return image.Rect(r.Min.X+r.Dx()*i/n, r.Min.Y, r.Min.X+r.Dx()*(i+1)/n, r.Max.Y)
}

// fit делит ширину между сегментами и выбирает шрифт. Надписи помещаются в равные доли —
// доли равные. Иначе каждому сегменту — ширина его надписи и поровну оставшегося места,
// чтобы длинная надпись не обрезалась. Не помещаются и так — шрифт мельче, а места не
// хватит и ему — делится пропорционально надписям.
func (s *Segmented) fit(t *Theme) *gfx.Face {
	r := s.Bounds()
	n := len(s.Options)
	s.edges = s.edges[:0]
	faces := []*gfx.Face{t.Body(), t.Caption()}
	for k, face := range faces {
		w := make([]int, n)
		sum, widest := 0, 0
		for i, o := range s.Options {
			w[i] = face.Measure(o) + 2*t.MM(1.5)
			sum += w[i]
			widest = max(widest, w[i])
		}
		if widest*n <= r.Dx() {
			return face
		}
		if sum > r.Dx() && k < len(faces)-1 {
			continue
		}
		s.edges = append(s.edges, r.Min.X)
		acc := 0
		for i := range w {
			acc += w[i]
			if sum <= r.Dx() {
				s.edges = append(s.edges, r.Min.X+acc+(r.Dx()-sum)*(i+1)/n)
			} else {
				s.edges = append(s.edges, r.Min.X+r.Dx()*acc/sum)
			}
		}
		return face
	}
	return t.Body()
}

func (s *Segmented) Tap(h Host, p image.Point) {
	for i := range s.Options {
		if p.In(s.cell(i)) && i != s.Selected {
			s.Selected = i
			if s.OnChange != nil {
				s.OnChange(i)
			}
			h.Invalidate(s.Bounds(), eink.UI)
			return
		}
	}
}

func (s *Segmented) Draw(c *gfx.Canvas, t *Theme) {
	r := s.Bounds()
	c.StrokeRoundRect(r, t.Radius(), t.Line(), gfx.Black)
	face := s.fit(t)
	for i, o := range s.Options {
		cell := s.cell(i)
		fg := gfx.Black
		if i == s.Selected {
			c.FillRoundRect(cell, t.Radius(), gfx.Black)
			fg = gfx.White
		} else if i > 0 && i-1 != s.Selected {
			c.Fill(image.Rect(cell.Min.X-t.Line()/2, r.Min.Y+t.MM(1.5), cell.Min.X+(t.Line()+1)/2, r.Max.Y-t.MM(1.5)), gfx.Black)
		}
		face.DrawIn(c, o, cell.Inset(t.MM(1)), gfx.AlignCenter, fg)
	}
}

// Grid раскладывает виджеты по страницам: Cols × Rows на странице, листание свайпом.
type Grid struct {
	Box
	Items      []Widget
	Cols, Rows int
	Gap        int
	page       int
	OnPage     func(page, pages int)
}

// Pages — сколько страниц.
func (g *Grid) Pages() int {
	per := max(g.Cols*g.Rows, 1)
	return max((len(g.Items)+per-1)/per, 1)
}

// Page — текущая страница с нуля.
func (g *Grid) Page() int { return g.page }

// PageText — подпись «2 / 5».
func (g *Grid) PageText() string { return fmt.Sprintf("%d / %d", g.page+1, g.Pages()) }

// SetBounds раскладывает текущую страницу.
func (g *Grid) SetBounds(r image.Rectangle) {
	g.R = r
	g.layout()
}

func (g *Grid) layout() {
	if g.page >= g.Pages() {
		g.page = g.Pages() - 1
	}
	per := max(g.Cols*g.Rows, 1)
	r := g.R
	cw := (r.Dx() - g.Gap*(g.Cols-1)) / max(g.Cols, 1)
	ch := (r.Dy() - g.Gap*(g.Rows-1)) / max(g.Rows, 1)
	for i, it := range g.Items {
		if i/per != g.page {
			it.SetBounds(image.Rectangle{})
			continue
		}
		k := i % per
		col, row := k%g.Cols, k/g.Cols
		x := r.Min.X + col*(cw+g.Gap)
		y := r.Min.Y + row*(ch+g.Gap)
		it.SetBounds(image.Rect(x, y, x+cw, y+ch))
	}
}

// SetPage переходит на страницу n и просит перерисовку.
func (g *Grid) SetPage(h Host, n int) bool {
	if n < 0 || n >= g.Pages() || n == g.page {
		return false
	}
	g.page = n
	g.layout()
	if g.OnPage != nil {
		g.OnPage(g.page, g.Pages())
	}
	h.Invalidate(g.R, eink.UI)
	return true
}

func (g *Grid) Swipe(h Host, dir Direction) bool {
	switch dir {
	case DirLeft, DirUp:
		return g.SetPage(h, g.page+1)
	case DirRight, DirDown:
		return g.SetPage(h, g.page-1)
	}
	return false
}

func (g *Grid) Children() []Widget {
	var out []Widget
	for _, it := range g.Items {
		if !it.Bounds().Empty() {
			out = append(out, it)
		}
	}
	return out
}

func (g *Grid) Draw(c *gfx.Canvas, t *Theme) {}

// Separator — горизонтальная линия.
type Separator struct{ Box }

func (s *Separator) Draw(c *gfx.Canvas, t *Theme) {
	r := s.Bounds()
	y := r.Min.Y + r.Dy()/2
	c.Fill(image.Rect(r.Min.X, y-t.Line()/4, r.Max.X, y+(t.Line()+3)/4), gfx.Gray16(6))
}

// Custom — виджет, который рисует функция; для разового оформления экрана.
type Custom struct {
	Box
	Paint func(c *gfx.Canvas, t *Theme, r image.Rectangle)
}

func (cu *Custom) Draw(c *gfx.Canvas, t *Theme) {
	if cu.Paint != nil {
		cu.Paint(c, t, cu.Bounds())
	}
}
