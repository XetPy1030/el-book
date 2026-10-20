package ui

import (
	"image"
	"time"

	"el-book/sdk/device"
	"el-book/sdk/gfx"
)

// Overlay — элемент поверх экрана: панель, диалог, тост. Показывает его app.Context.
type Overlay interface {
	Layout(t *Theme, screen image.Rectangle)
	Bounds() image.Rectangle
	Draw(c *gfx.Canvas, t *Theme)
	Touch(h Host, ev device.TouchEvent)
	Gesture(h Host, g Gesture) bool
	// Modal: касания мимо оверлея не доходят до экрана под ним.
	Modal() bool
	// SetCloser получает функцию, которая убирает оверлей.
	SetCloser(close func())
}

// Anchor — где на экране стоит панель.
type Anchor int

const (
	AnchorCenter Anchor = iota
	AnchorTop
	AnchorBottom
)

// Row — строка панели: виджет и его высота в пикселях (0 — высота основной цели касания).
type Row struct {
	W Widget
	H int
}

// Panel — карточка поверх экрана со строками виджетов. Тап мимо закрывает её, если Dismissable.
type Panel struct {
	Title       string
	Rows        []Row
	Anchor      Anchor
	Dismissable bool
	OnClose     func()

	box   image.Rectangle
	page  Page
	close func()
}

func (p *Panel) Modal() bool             { return true }
func (p *Panel) SetCloser(close func())  { p.close = close }
func (p *Panel) Bounds() image.Rectangle { return p.box }
func (p *Panel) Touch(h Host, ev device.TouchEvent) {
	if ev.Phase == device.TouchDown && !ev.Point().In(p.box) {
		return
	}
	p.page.Touch(h, ev)
}

// Close убирает панель.
func (p *Panel) Close() {
	if p.close != nil {
		p.close()
	}
	if p.OnClose != nil {
		p.OnClose()
	}
}

func (p *Panel) Gesture(h Host, g Gesture) bool {
	if g.Kind == Tap && !g.Pos.In(p.box) {
		if p.Dismissable {
			p.Close()
		}
		return true
	}
	return p.page.Gesture(h, g)
}

func (p *Panel) Layout(t *Theme, screen image.Rectangle) {
	pad := t.MM(4)
	width := min(screen.Dx()-2*t.Pad(), t.MM(110))
	inner := width - 2*pad
	height := pad
	if p.Title != "" {
		height += t.Title().Height + t.MM(2)
	}
	for i, r := range p.Rows {
		if i > 0 {
			height += t.Gap()
		}
		height += p.rowHeight(t, r, inner)
	}
	height += pad
	x := screen.Min.X + (screen.Dx()-width)/2
	y := screen.Min.Y + (screen.Dy()-height)/2
	switch p.Anchor {
	case AnchorTop:
		y = screen.Min.Y + t.Pad()
	case AnchorBottom:
		y = screen.Max.Y - t.Pad() - height
	}
	p.box = image.Rect(x, y, x+width, y+height)
	p.page.Clear()
	cy := y + pad
	if p.Title != "" {
		cy += t.Title().Height + t.MM(2)
	}
	for i, r := range p.Rows {
		if i > 0 {
			cy += t.Gap()
		}
		h := p.rowHeight(t, r, inner)
		r.W.SetBounds(image.Rect(x+pad, cy, x+pad+inner, cy+h))
		p.page.Add(r.W)
		cy += h
	}
}

// Measurer — виджет, который сам знает свою высоту при заданной ширине.
type Measurer interface {
	Height(t *Theme, width int) int
}

func (p *Panel) rowHeight(t *Theme, r Row, width int) int {
	if r.H > 0 {
		return r.H
	}
	if m, ok := r.W.(Measurer); ok {
		return m.Height(t, width)
	}
	return t.Touch()
}

func (p *Panel) Draw(c *gfx.Canvas, t *Theme) {
	c.Fill(p.box, gfx.White)
	c.StrokeRoundRect(p.box, t.MM(2), t.Line()*2, gfx.Black)
	if p.Title != "" {
		pad := t.MM(4)
		face := t.Title()
		face.DrawIn(c, p.Title, image.Rect(p.box.Min.X+pad, p.box.Min.Y+pad, p.box.Max.X-pad, p.box.Min.Y+pad+face.Height), gfx.AlignLeft, gfx.Black)
	}
	p.page.Draw(c, t)
}

// DialogButton — кнопка диалога.
type DialogButton struct {
	Text    string
	Primary bool
	OnTap   func()
}

// NewDialog создаёт диалог: заголовок, текст и кнопки в ряд. Любая кнопка закрывает диалог.
func NewDialog(t *Theme, title, message string, buttons ...DialogButton) *Panel {
	p := &Panel{Title: title}
	if message != "" {
		p.Rows = append(p.Rows, Row{W: &Label{Text: message, Wrap: true, VAlign: -1}})
	}
	row := &HStack{Gap: t.Gap()}
	for _, b := range buttons {
		b := b
		style := Secondary
		if b.Primary {
			style = Primary
		}
		row.Items = append(row.Items, &Button{Text: b.Text, Style: style, OnTap: func() {
			p.Close()
			if b.OnTap != nil {
				b.OnTap()
			}
		}})
	}
	p.Rows = append(p.Rows, Row{W: row})
	return p
}

// HStack раскладывает детей в ряд равной ширины.
type HStack struct {
	Box
	Items []Widget
	Gap   int
}

func (s *HStack) SetBounds(r image.Rectangle) {
	s.R = r
	n := len(s.Items)
	if n == 0 {
		return
	}
	w := (r.Dx() - s.Gap*(n-1)) / n
	for i, it := range s.Items {
		x := r.Min.X + i*(w+s.Gap)
		it.SetBounds(image.Rect(x, r.Min.Y, x+w, r.Max.Y))
	}
}

func (s *HStack) Children() []Widget           { return s.Items }
func (s *HStack) Draw(c *gfx.Canvas, t *Theme) {}

// Toast — короткое сообщение внизу экрана; исчезает само через Duration.
type Toast struct {
	Text     string
	Duration time.Duration
	box      image.Rectangle
}

// NewToast создаёт сообщение на 2,5 секунды.
func NewToast(text string) *Toast { return &Toast{Text: text, Duration: 2500 * time.Millisecond} }

func (t *Toast) Modal() bool                   { return false }
func (t *Toast) SetCloser(func())              {}
func (t *Toast) Bounds() image.Rectangle       { return t.box }
func (t *Toast) Touch(Host, device.TouchEvent) {}
func (t *Toast) Gesture(Host, Gesture) bool    { return false }

func (t *Toast) Layout(th *Theme, screen image.Rectangle) {
	face := th.Body()
	maxW := screen.Dx() - 2*th.Pad()
	lines := face.Wrap(t.Text, maxW-2*th.MM(4))
	w := 0
	for _, l := range lines {
		w = max(w, face.Measure(l))
	}
	w += 2 * th.MM(4)
	h := len(lines)*face.Height + 2*th.MM(3)
	x := screen.Min.X + (screen.Dx()-w)/2
	y := screen.Max.Y - th.MM(14) - h
	t.box = image.Rect(x, y, x+w, y+h)
}

func (t *Toast) Draw(c *gfx.Canvas, th *Theme) {
	c.FillRoundRect(t.box, th.MM(2), gfx.Black)
	face := th.Body()
	lines := face.Wrap(t.Text, t.box.Dx()-2*th.MM(4))
	y := t.box.Min.Y + th.MM(3)
	for _, l := range lines {
		face.DrawIn(c, l, image.Rect(t.box.Min.X+th.MM(4), y, t.box.Max.X-th.MM(4), y+face.Height), gfx.AlignCenter, gfx.White)
		y += face.Height
	}
}
