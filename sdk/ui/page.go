package ui

import (
	"image"

	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
)

// Widget — прямоугольный элемент интерфейса.
type Widget interface {
	Bounds() image.Rectangle
	SetBounds(image.Rectangle)
	Draw(c *gfx.Canvas, t *Theme)
}

// Tappable — виджет, реагирующий на тап. Enabled = false — нажатие не принимается.
type Tappable interface {
	Widget
	Tap(h Host, p image.Point)
	Enabled() bool
}

// LongPressable — виджет с действием по долгому нажатию.
type LongPressable interface {
	Widget
	LongPress(h Host, p image.Point)
}

// Draggable — виджет, который перетаскивают: получает PanStart, PanMove, PanEnd.
type Draggable interface {
	Widget
	Drag(h Host, g Gesture)
}

// Swipeable — виджет, понимающий свайп внутри себя (листание страниц).
type Swipeable interface {
	Widget
	Swipe(h Host, dir Direction) bool
}

// Box — общая часть виджетов: прямоугольник и видимость.
type Box struct {
	R      image.Rectangle
	Hidden bool
}

// Bounds возвращает пустой прямоугольник для скрытого виджета.
func (b *Box) Bounds() image.Rectangle {
	if b.Hidden {
		return image.Rectangle{}
	}
	return b.R
}

func (b *Box) SetBounds(r image.Rectangle) { b.R = r }

// Page — набор виджетов экрана: рисует их, показывает нажатие инверсией и раздаёт жесты.
type Page struct {
	Widgets []Widget
	pressed Widget
	drag    Draggable
}

// Add добавляет виджеты.
func (p *Page) Add(ws ...Widget) { p.Widgets = append(p.Widgets, ws...) }

// Clear убирает все виджеты.
func (p *Page) Clear() {
	p.Widgets = nil
	p.pressed, p.drag = nil, nil
}

// Draw рисует виджеты, попадающие в отсечение; нажатый — инвертированным.
func (p *Page) Draw(c *gfx.Canvas, t *Theme) {
	for _, w := range p.Widgets {
		p.drawWidget(c, t, w)
	}
}

func (p *Page) drawWidget(c *gfx.Canvas, t *Theme, w Widget) {
	b := w.Bounds()
	if b.Empty() || !b.Overlaps(c.Clip()) {
		return
	}
	w.Draw(c.WithClip(b), t)
	if ct, ok := w.(Container); ok {
		for _, ch := range ct.Children() {
			p.drawWidget(c, t, ch)
		}
	}
	if w == p.pressed {
		c.Invert(b)
	}
}

// at возвращает верхний виджет под точкой, удовлетворяющий условию; дети контейнера
// проверяются раньше самого контейнера.
func (p *Page) at(pt image.Point, ok func(Widget) bool) Widget {
	return find(p.Widgets, pt, ok)
}

func find(ws []Widget, pt image.Point, ok func(Widget) bool) Widget {
	for i := len(ws) - 1; i >= 0; i-- {
		w := ws[i]
		if !pt.In(w.Bounds()) {
			continue
		}
		if ct, isCt := w.(Container); isCt {
			if found := find(ct.Children(), pt, ok); found != nil {
				return found
			}
		}
		if ok(w) {
			return w
		}
	}
	return nil
}

func tappable(w Widget) bool {
	t, ok := w.(Tappable)
	return ok && t.Enabled()
}

// Touch обрабатывает касание: опускание пальца нажимает виджет, подъём — отпускает.
func (p *Page) Touch(h Host, ev device.TouchEvent) {
	switch ev.Phase {
	case device.TouchDown:
		if p.pressed == nil {
			if w := p.at(ev.Point(), tappable); w != nil {
				if pr, ok := w.(Pressable); ok && !pr.ShowsPress() {
					return
				}
				p.pressed = w
				h.Invalidate(w.Bounds(), eink.Feedback)
			}
		}
	case device.TouchUp:
		p.release(h)
	}
}

func (p *Page) release(h Host) {
	if p.pressed != nil {
		h.Invalidate(p.pressed.Bounds(), eink.UI)
		p.pressed = nil
	}
}

// Gesture раздаёт жесты виджетам. Возвращает true, если жест кому-то пригодился.
func (p *Page) Gesture(h Host, g Gesture) bool {
	switch g.Kind {
	case Tap:
		p.release(h)
		if w := p.at(g.Pos, tappable); w != nil {
			w.(Tappable).Tap(h, g.Pos)
			return true
		}
	case LongPress:
		p.release(h)
		if w := p.at(g.Pos, func(w Widget) bool { _, ok := w.(LongPressable); return ok }); w != nil {
			w.(LongPressable).LongPress(h, g.Pos)
			return true
		}
	case PanStart:
		p.release(h)
		if w := p.at(g.Start, func(w Widget) bool { _, ok := w.(Draggable); return ok }); w != nil {
			p.drag = w.(Draggable)
			p.drag.Drag(h, g)
			return true
		}
	case PanMove:
		if p.drag != nil {
			p.drag.Drag(h, g)
			return true
		}
	case PanEnd:
		if p.drag != nil {
			p.drag.Drag(h, g)
			p.drag = nil
			return true
		}
	case Swipe:
		if w := p.at(g.Start, func(w Widget) bool { _, ok := w.(Swipeable); return ok }); w != nil {
			return w.(Swipeable).Swipe(h, g.Dir)
		}
	}
	return false
}
