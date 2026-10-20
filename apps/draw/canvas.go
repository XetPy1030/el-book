package draw

import (
	"context"
	"fmt"
	"image"
	"math"
	"slices"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ink"
	"el-book/sdk/ui"
)

// tool — что делает палец на листе.
type tool int

const (
	toolBrush  tool = iota // рисует выбранной кистью
	toolEraser             // стирает по площади или штрихами
	toolLasso              // выделяет штрихи контуром
)

// op — действие для отмены и повтора.
type op struct {
	added   []*ink.Stroke // штрихи добавлены в конец страницы
	removed []placed      // штрихи удалены со своих мест
	moved   []*ink.Stroke // штрихи сдвинуты на dx, dy
	dx, dy  float32
	tmpl    *[2]Template // разметка страницы: была, стала
}

// placed — удалённый штрих и его место на странице.
type placed struct {
	s  *ink.Stroke
	at int
}

// history — отмена и повтор одной страницы.
type history struct{ undo, redo []op }

// pen — касание одного пальца.
type pen struct {
	st     *ink.Stroke // рисуемый штрих; nil — ластик штрихами или лассо
	sm     ink.Smoother
	snap   ink.Shape // во что превратился штрих по удержанию
	holdX  float64   // где палец остановился
	holdY  float64
	hold   func() // таймер удержания
	erase  bool
	lasso  bool
	drag   bool    // тянет выделение
	startX float64 // откуда тянут
	startY float64
}

// cleanDelay — через сколько после последнего штриха готовые штрихи перерисовываются
// начисто. Медленное обновление сразу после штриха задержало бы следующий штрих поверх него.
const cleanDelay = 600 * time.Millisecond

// holdDelay — сколько держать палец в конце штриха, чтобы он стал фигурой.
const holdDelay = 650 * time.Millisecond

// editor — экран тетради: панель инструментов сверху, страница под ней.
type editor struct {
	a     *App
	book  *Book
	page  *PageRef
	layer *image.Gray // страница целиком: разметка и штрихи
	rule  []ink.Seg
	area  image.Rectangle // где страница лежит на экране
	ui    ui.Page
	bar   image.Rectangle

	brushBtn, eraserBtn, lassoBtn, shapesBtn *ui.Button
	prevBtn, nextBtn, pageBtn                *ui.Button
	selBar                                   *ui.HStack

	tool   tool
	hist   map[string]*history
	pens   map[int]*pen
	lasso  ink.Lasso
	erased []placed // стёртое текущим касанием: одно действие для отмены

	sel     []*ink.Stroke   // выделенные штрихи
	selBox  image.Rectangle // их охват на странице
	dragOff image.Point     // сдвиг выделения во время перетаскивания

	unclean    image.Rectangle // область, показанная быстрым режимом
	cleanTimer func()
	dirty      bool
	saveTimer  func()
	saving     chan struct{} // закрывается, когда фоновая запись кончилась; nil — запись не идёт
}

func newEditor(a *App, b *Book) *editor {
	return &editor{a: a, book: b, hist: map[string]*history{}, pens: map[int]*pen{}}
}

func (s *editor) prefs() *prefs { return &s.a.prefs.Value }

func (s *editor) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	barH := t.MM(12)
	s.bar = image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+barH)
	s.area = image.Rect(b.Min.X, s.bar.Max.Y+t.Line(), b.Max.X, b.Max.Y)
	if s.book.Width == 0 || s.book.Height == 0 {
		s.book.Width, s.book.Height = s.area.Dx(), s.area.Dy()
	}
	if s.book.DPI == 0 {
		s.book.DPI = t.DPI
	}
	if s.layer == nil {
		s.layer = image.NewGray(image.Rect(0, 0, s.book.Width, s.book.Height))
		s.openPage(ctx, s.book.Current)
	}

	s.ui.Clear()
	s.brushBtn = &ui.Button{Style: ui.Flat, OnTap: func() { s.pickTool(ctx, toolBrush) }}
	s.eraserBtn = &ui.Button{Icon: icons.Eraser, Style: ui.Flat, OnTap: func() { s.pickTool(ctx, toolEraser) }}
	s.lassoBtn = &ui.Button{Icon: icons.Lasso, Style: ui.Flat, OnTap: func() { s.pickTool(ctx, toolLasso) }}
	s.shapesBtn = &ui.Button{Icon: icons.Shapes, Style: ui.Flat, OnTap: func() { s.toggleShapes(ctx) }}
	s.prevBtn = &ui.Button{Icon: icons.ChevronLeft, Style: ui.Flat, OnTap: func() { s.goPage(ctx, s.book.Current-1) }}
	s.nextBtn = &ui.Button{Style: ui.Flat, OnTap: func() { s.nextPage(ctx) }}
	s.pageBtn = &ui.Button{Style: ui.Flat, OnTap: func() { s.pagesPanel(ctx) }}
	bar := &ui.HStack{Items: []ui.Widget{
		&ui.Button{Icon: icons.Back, Style: ui.Flat, OnTap: func() { s.close(ctx) }},
		s.brushBtn, s.eraserBtn, s.lassoBtn, s.shapesBtn,
		&ui.Button{Icon: icons.Undo, Style: ui.Flat, OnTap: func() { s.undoOp(ctx) }},
		&ui.Button{Icon: icons.Redo, Style: ui.Flat, OnTap: func() { s.redoOp(ctx) }},
		s.prevBtn, s.pageBtn, s.nextBtn,
		&ui.Button{Icon: icons.More, Style: ui.Flat, OnTap: func() { s.moreMenu(ctx) }},
	}}
	bar.SetBounds(s.bar)
	s.selBar = &ui.HStack{Gap: t.Gap(), Items: []ui.Widget{
		&ui.Button{Text: "Удалить", Icon: icons.Trash, OnTap: func() { s.deleteSelection(ctx) }},
		&ui.Button{Text: "Копия", Icon: icons.Copy, OnTap: func() { s.copySelection(ctx) }},
		&ui.Button{Text: "Готово", Icon: icons.Check, Style: ui.Primary, OnTap: func() { s.clearSelection(ctx) }},
	}}
	s.selBar.SetBounds(image.Rect(s.area.Min.X+t.Pad(), s.area.Max.Y-t.Touch()-t.MM(3), s.area.Max.X-t.Pad(), s.area.Max.Y-t.MM(3)))
	s.ui.Add(bar)
	s.styleBar(ctx)
}

// styleBar показывает выбранный инструмент, состояние фигур и номер страницы.
func (s *editor) styleBar(ctx *app.Context) {
	p := s.prefs()
	s.brushBtn.Icon = brushIcon(p.Brush)
	s.brushBtn.Active = s.tool == toolBrush
	s.eraserBtn.Active = s.tool == toolEraser
	s.lassoBtn.Active = s.tool == toolLasso
	s.shapesBtn.Active = p.Shapes
	s.prevBtn.Disabled = s.book.Current == 0
	s.nextBtn.Icon = icons.ChevronRight
	if s.book.Current == len(s.book.Pages)-1 {
		s.nextBtn.Icon = icons.PageAdd
	}
	s.pageBtn.Text = fmt.Sprintf("%d/%d", s.book.Current+1, len(s.book.Pages))
	ctx.Invalidate(s.bar, eink.UI)
}

// brushIcon — значок кисти на панели.
func brushIcon(b ink.Brush) icons.Drawer {
	switch b {
	case ink.Fountain:
		return icons.Edit
	case ink.Marker:
		return icons.Marker
	case ink.Sketch, ink.Web, ink.Fur:
		return icons.Brush
	}
	return icons.Pencil
}

func (s *editor) Draw(c *gfx.Canvas, t *ui.Theme) {
	if !c.Clip().Intersect(s.area).Empty() {
		c.DrawGray(s.area, s.layer, image.Point{})
		if s.sel != nil {
			r := s.selBox.Add(s.area.Min).Add(s.dragOff).Inset(-t.MM(1.5))
			dashedRect(c, r, t.Line()*2)
		}
	}
	c.Fill(image.Rect(s.area.Min.X, s.area.Min.Y-t.Line(), s.area.Max.X, s.area.Min.Y), gfx.Black)
	s.ui.Draw(c, t)
	if s.sel != nil && s.dragOff == (image.Point{}) {
		c.Fill(s.selBar.R.Inset(-t.MM(1.5)), gfx.White)
		for _, w := range s.selBar.Items {
			w.Draw(c, t)
		}
	}
}

// dashedRect — пунктирная рамка выделения.
func dashedRect(c *gfx.Canvas, r image.Rectangle, w int) {
	const dash = 14
	for x := r.Min.X; x < r.Max.X; x += 2 * dash {
		x1 := min(x+dash, r.Max.X)
		c.Fill(image.Rect(x, r.Min.Y, x1, r.Min.Y+w), gfx.Black)
		c.Fill(image.Rect(x, r.Max.Y-w, x1, r.Max.Y), gfx.Black)
	}
	for y := r.Min.Y; y < r.Max.Y; y += 2 * dash {
		y1 := min(y+dash, r.Max.Y)
		c.Fill(image.Rect(r.Min.X, y, r.Min.X+w, y1), gfx.Black)
		c.Fill(image.Rect(r.Max.X-w, y, r.Max.X, y1), gfx.Black)
	}
}

func (s *editor) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case device.TouchEvent:
		if len(s.pens) > 0 || (e.Phase == device.TouchDown && e.Point().In(s.area) && !s.onSelBar(e.Point())) {
			s.touch(ctx, e)
			return
		}
		if s.onSelBar(e.Point()) {
			s.routeSelBar(ctx, ev)
			return
		}
		s.ui.Touch(ctx, e)
	case ui.Gesture:
		switch {
		case s.onSelBar(e.Start):
			s.routeSelBar(ctx, ev)
		case !e.Start.In(s.area):
			s.ui.Gesture(ctx, e)
		}
	case device.KeyEvent:
		if !e.Pressed {
			return
		}
		switch e.Key {
		case device.KeyPageBack:
			s.undoOp(ctx)
		case device.KeyPageForward:
			s.redoOp(ctx)
		}
	case app.Lifecycle:
		switch e {
		case app.Shown:
			if !s.a.hinted {
				s.a.hinted = true
				ctx.Toast("Кнопки листания: назад — отмена, вперёд — повтор. Задержите палец — получится фигура")
			}
		case app.Paused, app.Hidden:
			s.save(ctx, true)
		}
	}
}

func (s *editor) onSelBar(p image.Point) bool { return s.sel != nil && p.In(s.selBar.R) }

// routeSelBar отдаёт касание кнопкам выделения.
func (s *editor) routeSelBar(ctx *app.Context, ev app.Event) {
	var pg ui.Page
	pg.Add(s.selBar)
	switch e := ev.(type) {
	case device.TouchEvent:
		pg.Touch(ctx, e)
	case ui.Gesture:
		pg.Gesture(ctx, e)
	}
}

// touch ведёт касания листа: каждый палец рисует свой штрих; ластик, лассо и перетаскивание
// выделения — первым пальцем.
func (s *editor) touch(ctx *app.Context, e device.TouchEvent) {
	t := ctx.Theme()
	x := float64(e.X-s.area.Min.X) + 0.5
	y := float64(e.Y-s.area.Min.Y) + 0.5
	p := s.pens[e.Slot]
	switch e.Phase {
	case device.TouchDown:
		if p != nil || !e.Point().In(s.area) {
			return
		}
		pt := image.Pt(int(x), int(y))
		if s.sel != nil {
			if e.Slot == 0 && pt.In(s.selBox.Inset(-t.MM(3))) {
				s.pens[e.Slot] = &pen{drag: true, startX: x, startY: y}
				return
			}
			s.clearSelection(ctx)
		}
		switch {
		case s.tool == toolLasso:
			if e.Slot != 0 {
				return
			}
			s.lasso = ink.Lasso{{X: float32(x), Y: float32(y)}}
			s.pens[e.Slot] = &pen{lasso: true}
		case s.tool == toolEraser && s.prefs().Eraser == eraseStroke:
			s.erased = nil
			s.pens[e.Slot] = &pen{erase: true}
			s.eraseAt(ctx, x, y)
		default:
			p = s.newPen(ctx, e.Time, x, y)
			s.pens[e.Slot] = p
			s.drawLast(ctx, p)
			s.armHold(ctx, e.Slot, p, x, y)
		}
	case device.TouchMove:
		if p == nil {
			return
		}
		switch {
		case p.drag:
			s.moveDrag(ctx, p, x, y)
		case p.lasso:
			last := s.lasso[len(s.lasso)-1]
			if math.Hypot(x-float64(last.X), y-float64(last.Y)) < 4 {
				return
			}
			s.lasso = append(s.lasso, ink.Point{X: float32(x), Y: float32(y)})
			c := gfx.NewCanvas(s.layer)
			r := ink.DrawSeg(c, ink.Seg{X0: last.X, Y0: last.Y, X1: float32(x), Y1: float32(y), R0: 1.5, R1: 1.5, Tone: 120}, ink.Fast)
			ctx.Invalidate(r.Add(s.area.Min), eink.Stroke)
		case p.erase:
			s.eraseAt(ctx, x, y)
		case p.snap == ink.Line:
			s.bendLine(ctx, p, x, y)
		case p.snap != ink.NoShape:
			// фигура уже готова: палец только держит её
		default:
			if pt, ok := p.sm.Move(x, y, e.Time); ok {
				p.st.Pts = append(p.st.Pts, pt)
				p.st.Changed()
				s.drawLast(ctx, p)
			}
			if math.Hypot(x-p.holdX, y-p.holdY) > float64(t.MM(1.5)) {
				s.armHold(ctx, e.Slot, p, x, y)
			}
		}
	case device.TouchUp:
		if p == nil {
			return
		}
		delete(s.pens, e.Slot)
		if p.hold != nil {
			p.hold()
		}
		switch {
		case p.drag:
			s.dropDrag(ctx, p, x, y)
		case p.lasso:
			s.finishLasso(ctx)
		case p.erase:
			s.finishErase(ctx)
		default:
			if p.snap == ink.NoShape && (e.X != 0 || e.Y != 0) {
				if pt, ok := p.sm.End(x, y, e.Time); ok {
					p.st.Pts = append(p.st.Pts, pt)
					p.st.Changed()
					s.drawLast(ctx, p)
				}
			}
			s.finishStroke(ctx, p)
		}
	}
}

// newPen начинает штрих выбранной кистью или ластиком по площади.
func (s *editor) newPen(ctx *app.Context, at time.Time, x, y float64) *pen {
	pr := s.prefs()
	mm := func(v float64) float32 { return float32(math.Max(v*float64(s.book.DPI)/25.4, 1.5)) }
	st := &ink.Stroke{Brush: pr.Brush, Width: mm(pr.Widths[pr.Brush]), Tone: pr.Tones[pr.Brush],
		Seed: strokeSeed(ctx.Now(), x, y, len(s.page.strokes))}
	if s.tool == toolEraser {
		st = &ink.Stroke{Brush: ink.Eraser, Width: mm(pr.EraserMM), Tone: gfx.White}
	}
	p := &pen{st: st, sm: ink.Smoother{Speed: st.Brush.Dynamic()}}
	st.Pts = append(st.Pts, p.sm.Begin(x, y, at))
	return p
}

// strokeSeed — зерно процедурных кистей: от времени и места начала штриха. Текстура у
// каждого штриха своя, а при замороженных часах UI-тестов рисунок повторяется точно.
func strokeSeed(now time.Time, x, y float64, n int) uint32 {
	return uint32(now.UnixNano()) ^ uint32(x)*73856093 ^ uint32(y)*19349663 ^ uint32(n)*83492791
}

// drawLast рисует последнюю точку штриха быстрым режимом.
func (s *editor) drawLast(ctx *app.Context, p *pen) {
	r := p.st.DrawPoint(gfx.NewCanvas(s.layer), len(p.st.Pts)-1, ink.Fast)
	ctx.Invalidate(r.Add(s.area.Min), eink.Stroke)
}

// armHold заводит таймер удержания: палец замер — штрих становится фигурой.
func (s *editor) armHold(ctx *app.Context, slot int, p *pen, x, y float64) {
	if p.st == nil || p.st.Brush == ink.Eraser {
		return
	}
	p.holdX, p.holdY = x, y
	if p.hold != nil {
		p.hold()
	}
	p.hold = ctx.After(holdDelay, func() {
		p.hold = nil
		if s.pens[slot] == p && p.snap == ink.NoShape {
			s.snapShape(ctx, p)
		}
	})
}

// snapShape превращает штрих в фигуру, если она узнаётся.
func (s *editor) snapShape(ctx *app.Context, p *pen) bool {
	kind, pts := ink.Recognize(p.st.Pts, float64(ctx.Theme().MM(8)))
	if kind == ink.NoShape {
		return false
	}
	old := p.st.Bounds()
	p.st.Pts = pts
	p.st.Changed()
	p.snap = kind
	r := old.Union(p.st.Bounds())
	s.renderArea(r)
	ctx.Invalidate(r.Add(s.area.Min), eink.UI)
	return true
}

// bendLine ведёт конец линии-фигуры за пальцем.
func (s *editor) bendLine(ctx *app.Context, p *pen, x, y float64) {
	st := p.st
	old := st.Bounds()
	a, b := ink.SnapLine(st.Pts[0], ink.Point{X: float32(x), Y: float32(y), P: 1})
	st.Pts = []ink.Point{a, b}
	st.Changed()
	r := old.Union(st.Bounds())
	s.renderArea(r)
	s.unclean = s.unclean.Union(r)
	ctx.Invalidate(r.Add(s.area.Min), eink.Stroke)
}

// finishStroke кладёт штрих на страницу. В слое он сразу сглажен, на экран чистовой вид
// попадает, когда пальцы замрут на cleanDelay.
func (s *editor) finishStroke(ctx *app.Context, p *pen) {
	st := p.st
	if len(st.Pts) == 0 {
		return
	}
	if p.snap == ink.NoShape && s.prefs().Shapes && st.Brush != ink.Eraser {
		if kind, pts := ink.Recognize(st.Pts, float64(ctx.Theme().MM(8))); kind != ink.NoShape {
			old := st.Bounds()
			st.Pts = pts
			st.Changed()
			s.unclean = s.unclean.Union(old)
		}
	}
	st.Changed()
	s.page.strokes = append(s.page.strokes, st)
	s.push(op{added: []*ink.Stroke{st}})
	s.renderArea(s.unclean.Union(st.Bounds()))
	s.markUnclean(ctx, st.Bounds())
}

// markUnclean запоминает область, показанную быстрым режимом, и перерисует её начисто,
// когда пальцы замрут.
func (s *editor) markUnclean(ctx *app.Context, r image.Rectangle) {
	s.unclean = s.unclean.Union(r)
	if s.cleanTimer != nil {
		s.cleanTimer()
	}
	s.cleanTimer = ctx.After(cleanDelay, func() {
		s.cleanTimer = nil
		if len(s.pens) == 0 && !s.unclean.Empty() { // иначе перерисует подъём следующего пальца
			ctx.Invalidate(s.unclean.Add(s.area.Min), eink.UI)
			s.unclean = image.Rectangle{}
		}
	})
}

// renderArea перерисовывает область слоя: готовые штрихи начисто, ведомые пальцами — быстро.
func (s *editor) renderArea(r image.Rectangle) {
	var live []*ink.Stroke
	for _, p := range s.pens {
		if p.st != nil {
			live = append(live, p.st)
		}
	}
	render(s.layer, r, s.page.strokes, s.rule, live...)
}

// eraseAt стирает штрихи, которых касается ластик.
func (s *editor) eraseAt(ctx *app.Context, x, y float64) {
	rad := s.prefs().EraserMM * float64(s.book.DPI) / 25.4 / 2
	var dirty image.Rectangle
	strokes := s.page.strokes
	for i := len(strokes) - 1; i >= 0; i-- {
		if strokes[i].Brush != ink.Eraser && strokes[i].Hit(x, y, rad) {
			dirty = dirty.Union(strokes[i].Bounds())
			s.erased = append(s.erased, placed{s: strokes[i], at: i})
			strokes = slices.Delete(strokes, i, i+1)
		}
	}
	s.page.strokes = strokes
	if dirty.Empty() {
		return
	}
	s.renderArea(dirty)
	ctx.Invalidate(dirty.Add(s.area.Min), eink.Stroke)
	s.markUnclean(ctx, dirty)
}

func (s *editor) finishErase(ctx *app.Context) {
	if len(s.erased) == 0 {
		return
	}
	s.push(op{removed: s.erased})
	s.erased = nil
}

// finishLasso выделяет штрихи внутри контура.
func (s *editor) finishLasso(ctx *app.Context) {
	trail := ink.Union([]*ink.Stroke{{Width: 4, Pts: s.lasso}})
	var sel []*ink.Stroke
	if len(s.lasso) >= 3 {
		for _, st := range s.page.strokes {
			if st.Brush != ink.Eraser && s.lasso.Selects(st) {
				sel = append(sel, st)
			}
		}
	}
	s.lasso = nil
	s.renderArea(trail)
	ctx.Invalidate(trail.Add(s.area.Min), eink.UI)
	if len(sel) == 0 {
		ctx.Toast("Обведите штрихи, чтобы выделить их")
		return
	}
	s.selectStrokes(ctx, sel)
}

func (s *editor) selectStrokes(ctx *app.Context, sel []*ink.Stroke) {
	s.sel = sel
	s.selBox = ink.Union(sel)
	s.dragOff = image.Point{}
	t := ctx.Theme()
	ctx.Invalidate(s.selBox.Add(s.area.Min).Inset(-t.MM(2)), eink.UI)
	ctx.Invalidate(s.selBar.R.Inset(-t.MM(2)), eink.UI)
}

func (s *editor) clearSelection(ctx *app.Context) {
	if s.sel == nil {
		return
	}
	t := ctx.Theme()
	ctx.Invalidate(s.selBox.Add(s.area.Min).Add(s.dragOff).Inset(-t.MM(2)), eink.UI)
	ctx.Invalidate(s.selBar.R.Inset(-t.MM(2)), eink.UI)
	s.sel, s.selBox, s.dragOff = nil, image.Rectangle{}, image.Point{}
}

// moveDrag показывает, куда переедет выделение: двигается только рамка, штрихи
// переезжают, когда палец отпустят, — так панели не приходится перерисовывать их на ходу.
func (s *editor) moveDrag(ctx *app.Context, p *pen, x, y float64) {
	t := ctx.Theme()
	old := s.selBox.Add(s.area.Min).Add(s.dragOff).Inset(-t.MM(2))
	s.dragOff = image.Pt(int(math.Round(x-p.startX)), int(math.Round(y-p.startY)))
	ctx.Invalidate(old, eink.Stroke)
	ctx.Invalidate(s.selBox.Add(s.area.Min).Add(s.dragOff).Inset(-t.MM(2)), eink.Stroke)
	s.unclean = s.unclean.Union(old.Sub(s.area.Min))
	if old.Overlaps(s.selBar.R) {
		ctx.Invalidate(s.selBar.R.Inset(-t.MM(2)), eink.Stroke)
		s.unclean = s.unclean.Union(s.selBar.R.Inset(-t.MM(2)).Sub(s.area.Min))
	}
}

func (s *editor) dropDrag(ctx *app.Context, p *pen, x, y float64) {
	off := s.dragOff
	s.dragOff = image.Point{}
	// Путь рамки показан быстрым режимом — начисто его перерисует таймер.
	s.markUnclean(ctx, image.Rectangle{})
	if off == (image.Point{}) {
		return
	}
	o := op{moved: s.sel, dx: float32(off.X), dy: float32(off.Y)}
	s.push(o)
	s.applyMove(o, 1)
	r := s.selBox.Union(s.selBox.Add(off))
	s.selBox = s.selBox.Add(off)
	s.renderArea(r)
	t := ctx.Theme()
	ctx.Invalidate(r.Add(s.area.Min).Inset(-t.MM(2)), eink.UI)
	ctx.Invalidate(s.selBar.R.Inset(-t.MM(2)), eink.UI)
}

func (s *editor) deleteSelection(ctx *app.Context) {
	if s.sel == nil {
		return
	}
	var removed []placed
	for _, st := range s.sel {
		if i := slices.Index(s.page.strokes, st); i >= 0 {
			removed = append(removed, placed{s: st, at: i})
			s.page.strokes = slices.Delete(s.page.strokes, i, i+1)
		}
	}
	box := s.selBox
	s.clearSelection(ctx)
	s.push(op{removed: removed})
	s.renderArea(box)
	ctx.Invalidate(box.Add(s.area.Min), eink.UI)
}

// copySelection кладёт копию выделения чуть ниже и правее и выделяет её.
func (s *editor) copySelection(ctx *app.Context) {
	if s.sel == nil {
		return
	}
	d := float32(ctx.Theme().MM(5))
	var copies []*ink.Stroke
	for _, st := range s.sel {
		c := st.Clone()
		c.Translate(d, d)
		copies = append(copies, c)
	}
	s.clearSelection(ctx)
	s.page.strokes = append(s.page.strokes, copies...)
	s.push(op{added: copies})
	box := ink.Union(copies)
	s.renderArea(box)
	ctx.Invalidate(box.Add(s.area.Min), eink.UI)
	s.selectStrokes(ctx, copies)
}

// push записывает действие в историю страницы и отмечает изменения.
func (s *editor) push(o op) {
	h := s.history()
	h.undo = append(h.undo, o)
	h.redo = nil
	s.page.dirty = true
	s.dirty = true
}

func (s *editor) history() *history {
	h := s.hist[s.page.ID]
	if h == nil {
		h = &history{}
		s.hist[s.page.ID] = h
	}
	return h
}

func (s *editor) undoOp(ctx *app.Context) {
	h := s.history()
	if len(h.undo) == 0 {
		ctx.Toast("Отменять нечего")
		return
	}
	o := h.undo[len(h.undo)-1]
	h.undo = h.undo[:len(h.undo)-1]
	h.redo = append(h.redo, o)
	s.apply(ctx, o, true)
}

func (s *editor) redoOp(ctx *app.Context) {
	h := s.history()
	if len(h.redo) == 0 {
		ctx.Toast("Повторять нечего")
		return
	}
	o := h.redo[len(h.redo)-1]
	h.redo = h.redo[:len(h.redo)-1]
	h.undo = append(h.undo, o)
	s.apply(ctx, o, false)
}

// apply выполняет (reverse = false) или отменяет (reverse = true) действие и перерисовывает
// затронутое.
func (s *editor) apply(ctx *app.Context, o op, reverse bool) {
	s.clearSelection(ctx)
	var dirty image.Rectangle
	switch {
	case o.tmpl != nil:
		t := o.tmpl[1]
		if reverse {
			t = o.tmpl[0]
		}
		s.page.Template = t
		s.rule = ruling(s.book.template(s.page), s.book.Width, s.book.Height, s.book.DPI)
		dirty = s.layer.Bounds()
	case o.added != nil && reverse:
		for _, st := range o.added {
			if i := slices.Index(s.page.strokes, st); i >= 0 {
				s.page.strokes = slices.Delete(s.page.strokes, i, i+1)
			}
			dirty = dirty.Union(st.Bounds())
		}
	case o.added != nil:
		s.page.strokes = append(s.page.strokes, o.added...)
		dirty = ink.Union(o.added)
	case o.removed != nil && reverse:
		for i := len(o.removed) - 1; i >= 0; i-- {
			p := o.removed[i]
			at := min(p.at, len(s.page.strokes))
			s.page.strokes = slices.Insert(s.page.strokes, at, p.s)
			dirty = dirty.Union(p.s.Bounds())
		}
	case o.removed != nil:
		for _, p := range o.removed {
			if i := slices.Index(s.page.strokes, p.s); i >= 0 {
				s.page.strokes = slices.Delete(s.page.strokes, i, i+1)
			}
			dirty = dirty.Union(p.s.Bounds())
		}
	case o.moved != nil:
		dirty = ink.Union(o.moved)
		sign := float32(1)
		if reverse {
			sign = -1
		}
		s.applyMove(o, sign)
		dirty = dirty.Union(ink.Union(o.moved))
	}
	s.page.dirty = true
	s.renderArea(dirty)
	cl := eink.UI
	if o.tmpl != nil {
		cl = eink.Content
	}
	ctx.Invalidate(dirty.Add(s.area.Min), cl)
	s.markDirty(ctx)
}

func (s *editor) applyMove(o op, sign float32) {
	for _, st := range o.moved {
		st.Translate(sign*o.dx, sign*o.dy)
	}
}

func (s *editor) pickTool(ctx *app.Context, tl tool) {
	if s.tool == tl {
		switch tl {
		case toolBrush:
			s.brushPanel(ctx)
		case toolEraser:
			s.eraserPanel(ctx)
		}
		return
	}
	s.clearSelection(ctx)
	s.tool = tl
	s.styleBar(ctx)
	switch tl {
	case toolEraser:
		if s.prefs().Eraser == eraseStroke {
			ctx.Toast("Ластик штрихами: касание удаляет штрих целиком")
		}
	case toolLasso:
		ctx.Toast("Обведите штрихи: их можно сдвинуть, скопировать или удалить")
	}
}

func (s *editor) toggleShapes(ctx *app.Context) {
	p := s.prefs()
	p.Shapes = !p.Shapes
	s.a.save(ctx)
	s.styleBar(ctx)
	if p.Shapes {
		ctx.Toast("Фигуры: линии, прямоугольники, треугольники и эллипсы выравниваются сами")
	} else {
		ctx.Toast("Фигуры выключены: задержите палец в конце штриха, чтобы выровнять")
	}
}

// openPage показывает страницу n: читает её штрихи и перерисовывает слой целиком.
func (s *editor) openPage(ctx *app.Context, n int) bool {
	n = min(max(n, 0), len(s.book.Pages)-1)
	p := s.book.Pages[n]
	if err := s.book.load(ctx.Storage(), p); err != nil {
		ctx.Fail("Не удалось открыть страницу", err, "book", s.book.ID, "page", p.ID)
		return false
	}
	s.book.Current = n
	s.page = p
	s.rule = ruling(s.book.template(p), s.book.Width, s.book.Height, s.book.DPI)
	s.renderArea(s.layer.Bounds())
	return true
}

// goPage листает тетрадь.
func (s *editor) goPage(ctx *app.Context, n int) {
	if n < 0 || n >= len(s.book.Pages) || n == s.book.Current || len(s.pens) > 0 {
		return
	}
	s.clearSelection(ctx)
	if !s.openPage(ctx, n) {
		return
	}
	s.dirty = true
	s.markDirty(ctx)
	s.styleBar(ctx)
	ctx.Invalidate(s.area, eink.Content)
}

// nextPage листает вперёд; на последней странице добавляет новую.
func (s *editor) nextPage(ctx *app.Context) {
	if s.book.Current < len(s.book.Pages)-1 {
		s.goPage(ctx, s.book.Current+1)
		return
	}
	s.addPage(ctx)
}

func (s *editor) addPage(ctx *app.Context) {
	n := s.book.addPage(ctx.Now(), s.book.Current, s.book.template(s.page))
	s.goPage(ctx, n)
	ctx.Toast(fmt.Sprintf("Страница %d добавлена", n+1))
}

// markDirty откладывает сохранение: тетрадь пишется через пару секунд после последнего
// изменения.
func (s *editor) markDirty(ctx *app.Context) {
	s.dirty = true
	if s.saveTimer != nil {
		s.saveTimer()
	}
	s.saveTimer = ctx.After(2*time.Second, func() { s.save(ctx, false) })
}

// save сохраняет тетрадь. Снимок делается в цикле, а запись с fsync на FAT — в фоне, чтобы
// не останавливать рисование. wait — записать сразу, дождавшись фоновой записи: перед сном,
// выходом и уходом с экрана.
func (s *editor) save(ctx *app.Context, wait bool) {
	if s.saveTimer != nil {
		s.saveTimer()
		s.saveTimer = nil
	}
	if s.saving != nil {
		if !wait {
			return // когда фоновая запись кончится, сохранение запланируется снова
		}
		<-s.saving
		s.saving = nil
	}
	if !s.dirty && !s.page.dirty {
		return
	}
	snap, err := s.book.snapshot(s.layer, ctx.Now())
	if err != nil {
		ctx.Fail("Не удалось сохранить тетрадь", err, "id", s.book.ID)
		return
	}
	s.dirty = false
	st := ctx.Storage()
	fail := func(err error) {
		s.dirty = true
		for _, p := range s.book.Pages {
			if _, ok := snap.pages[p.ID]; ok {
				p.dirty = true
			}
		}
		ctx.Fail("Не удалось сохранить тетрадь", err, "id", s.book.ID)
	}
	if wait {
		if err := snap.write(st); err != nil {
			fail(err)
		}
		return
	}
	done := make(chan struct{})
	s.saving = done
	ctx.Go(func(context.Context) {
		err := snap.write(st)
		close(done)
		ctx.Post(func() {
			if s.saving == done {
				s.saving = nil
			}
			if err != nil {
				fail(err)
			}
			if s.dirty {
				s.markDirty(ctx)
			}
		})
	})
}

func (s *editor) close(ctx *app.Context) {
	s.save(ctx, true)
	ctx.Pop()
}
