package rachinsky

import (
	"image"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ui"
)

// pmode — как выбирается следующая задача.
type pmode int

const (
	modeOrder  pmode = iota // по порядку с закладки
	modeRandom              // случайная нерешённая
	modeReview              // отложенные и ошибки
	modeSingle              // открыта из списка «Все задачи»
)

// Фазы экрана задачи.
const (
	phaseInput = iota // ответ ещё не показан: считаем или вводим
	phaseMark         // ответ показан, ждём самооценку «Верно/Ошибся»
	phaseDone         // ответ записан, ждём «Дальше»
)

// problemScreen — экран одной задачи: текст, подсказка по мерам, ответ и переход дальше.
// hist хранит показанные задачи, pos — текущую: так «назад» возвращает к прежней задаче
// в любом режиме, а «дальше» выбирает новую по режиму.
type problemScreen struct {
	a    *App
	md   pmode
	hist []int
	pos  int

	page     ui.Page
	bar      *ui.TitleBar
	flagBtn  *ui.Button
	b        image.Rectangle
	phase    int
	typedBuf string
	displayR image.Rectangle
}

func newProblemScreen(a *App, md pmode, idx int) *problemScreen {
	return &problemScreen{a: a, md: md, hist: []int{idx}, pos: 0}
}

func (ps *problemScreen) cur() int { return ps.hist[ps.pos] }

func (ps *problemScreen) Layout(ctx *app.Context, b image.Rectangle) {
	ps.b = b
	ps.build(ctx)
}

func (ps *problemScreen) build(ctx *app.Context) {
	t := ctx.Theme()
	ps.page.Clear()
	st := ps.a.st
	idx := ps.cur()
	p := problems[idx]
	r := ps.b

	ps.flagBtn = &ui.Button{Icon: icons.Flag, Style: ui.Flat, Active: st.flags[idx],
		OnTap: func() { ps.toggleFlag(ctx) }}
	ps.bar = ui.NewTitleBar(t, "№"+itoa(p.Num), func() { ctx.Pop() }, ps.flagBtn)
	ps.bar.Subtitle = "решено " + itoa(st.solved()) + " из " + itoa(st.n)
	ps.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	ps.page.Add(ps.bar)

	body := ui.Pad(r, t.Pad()+t.MM(1), t.MM(3), t.Pad()+t.MM(1), t.MM(3))

	// Элементы ответа — снизу.
	ctrl := ui.CutBottom(&body, ps.controlsHeight(t))

	// Кнопка «Меры» — над элементами ответа, если в задаче есть старые меры.
	if us := unitsIn(p.Text); len(us) > 0 {
		row := ui.CutBottom(&body, t.Touch())
		ui.CutBottom(&body, t.MM(2))
		label := "Меры — " + itoa(len(us))
		btn := &ui.Button{Text: label, Icon: icons.Info, Style: ui.Secondary,
			OnTap: func() { ps.a.openMeasures(ctx, us) }}
		w := t.Body().Measure(label) + t.IconSize() + t.MM(11)
		btn.SetBounds(image.Rect(row.Min.X, row.Min.Y, min(row.Min.X+w, row.Max.X), row.Max.Y))
		ps.page.Add(btn)
	}

	// Текст задачи — в оставшейся области, шрифтом покрупнее, насколько влезает.
	face := fitFace(t, p.Text, body.Dx(), body.Dy())
	text := &ui.Label{Text: p.Text, Wrap: true, Color: gfx.Black,
		Face: func(*ui.Theme) *gfx.Face { return face }}
	text.SetBounds(body)
	ps.page.Add(text)

	ps.buildControls(ctx, ctrl)
}

func (ps *problemScreen) controlsHeight(t *ui.Theme) int {
	switch {
	case ps.phase == phaseMark || ps.phase == phaseDone:
		return ps.answerBoxH(t) + t.MM(3) + t.Touch()
	case ps.a.st.typed: // phaseInput, режим ввода
		return ps.answerBoxH(t) + t.MM(2) + ps.keypadH(t) + t.MM(3) + t.Touch()
	default: // phaseInput, самопроверка
		return t.Touch()
	}
}

func (ps *problemScreen) answerBoxH(t *ui.Theme) int { return t.MM(14) }
func (ps *problemScreen) keypadH(t *ui.Theme) int    { return 4*t.MM(13) + 3*t.MM(1.5) }

func (ps *problemScreen) buildControls(ctx *app.Context, r image.Rectangle) {
	t := ctx.Theme()
	p := problems[ps.cur()]
	switch {
	case ps.phase == phaseMark:
		ps.addAnswerLabel(ctx, &r, "Ответ: "+p.Answer)
		ui.CutTop(&r, t.MM(3))
		cols := ui.Columns(r, 2, t.MM(2))
		wrong := &ui.Button{Text: "Ошибся", Style: ui.Secondary, OnTap: func() { ps.mark(ctx, false) }}
		right := &ui.Button{Text: "Верно", Style: ui.Primary, OnTap: func() { ps.mark(ctx, true) }}
		wrong.SetBounds(cols[0])
		right.SetBounds(cols[1])
		ps.page.Add(wrong, right)
	case ps.phase == phaseDone:
		ps.addAnswerLabel(ctx, &r, "Ответ: "+p.Answer)
		ui.CutTop(&r, t.MM(3))
		next := &ui.Button{Text: "Дальше", Icon: icons.ChevronRight, Style: ui.Primary, OnTap: func() { ps.goNext(ctx) }}
		next.SetBounds(r)
		ps.page.Add(next)
	case ps.a.st.typed:
		ps.displayR = ui.CutTop(&r, ps.answerBoxH(t))
		display := &ui.Custom{Paint: func(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) { ps.paintTyped(c, t, r) }}
		display.SetBounds(ps.displayR)
		ps.page.Add(display)
		ui.CutTop(&r, t.MM(2))
		ps.addKeypad(ctx, ui.CutTop(&r, ps.keypadH(t)))
		ui.CutTop(&r, t.MM(3))
		cols := ui.Columns(r, 2, t.MM(2))
		show := &ui.Button{Text: "Показать", Style: ui.Secondary, OnTap: func() { ps.giveUp(ctx) }}
		check := &ui.Button{Text: "Проверить", Style: ui.Primary, OnTap: func() { ps.check(ctx) }}
		show.SetBounds(cols[0])
		check.SetBounds(cols[1])
		ps.page.Add(show, check)
	default:
		show := &ui.Button{Text: "Показать ответ", Style: ui.Primary, OnTap: func() { ps.reveal(ctx) }}
		show.SetBounds(r)
		ps.page.Add(show)
	}
}

func (ps *problemScreen) addAnswerLabel(ctx *app.Context, r *image.Rectangle, text string) {
	box := ui.CutTop(r, ps.answerBoxH(ctx.Theme()))
	face := ctx.Theme().Face(gfx.Bold, 18)
	lbl := &ui.Label{Text: text, Wrap: true, Align: gfx.AlignCenter, Color: gfx.Black,
		Face: func(*ui.Theme) *gfx.Face { return face }}
	lbl.SetBounds(box)
	ps.page.Add(lbl)
}

func (ps *problemScreen) addKeypad(ctx *app.Context, r image.Rectangle) {
	t := ctx.Theme()
	keyFace := func(t *ui.Theme) *gfx.Face { return t.Face(gfx.Medium, 22) }
	rows := ui.Rows(r, 4, t.MM(1.5))
	keys := [][]string{{"1", "2", "3"}, {"4", "5", "6"}, {"7", "8", "9"}, {"⌫", "0", "C"}}
	for ri, row := range rows {
		cols := ui.Columns(row, 3, t.MM(1.5))
		for ci, k := range keys[ri] {
			k := k
			btn := &ui.Button{Style: ui.Secondary, Face: keyFace, OnTap: func() { ps.typeKey(ctx, k) }}
			switch k {
			case "⌫":
				btn.Icon, btn.Face = icons.Backspace, nil
			case "C":
				btn.Text = k
			default:
				btn.Text = k
			}
			btn.SetBounds(cols[ci])
			ps.page.Add(btn)
		}
	}
}

func (ps *problemScreen) paintTyped(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	c.StrokeRoundRect(r, t.Radius(), t.Line(), gfx.Gray16(6))
	s, col := ps.typedBuf, gfx.Black
	if s == "" {
		s, col = "ответ", gfx.Gray16(9)
	}
	t.Face(gfx.Bold, 26).DrawIn(c, s, r.Inset(t.MM(2)), gfx.AlignCenter, col)
}

// --- Действия ---

func (ps *problemScreen) typeKey(ctx *app.Context, k string) {
	switch k {
	case "⌫":
		if n := len(ps.typedBuf); n > 0 {
			ps.typedBuf = ps.typedBuf[:n-1]
		}
	case "C":
		ps.typedBuf = ""
	default:
		if len(ps.typedBuf) < 12 {
			ps.typedBuf += k
		}
	}
	ctx.Invalidate(ps.displayR, eink.UI)
}

func (ps *problemScreen) reveal(ctx *app.Context) {
	ps.phase = phaseMark
	ps.show(ctx)
}

func (ps *problemScreen) giveUp(ctx *app.Context) {
	ps.phase = phaseMark
	ps.show(ctx)
}

func (ps *problemScreen) check(ctx *app.Context) {
	if ps.typedBuf == "" {
		ctx.Toast("Наберите ответ")
		return
	}
	ok, decisive := checkTyped(problems[ps.cur()].Answer, ps.typedBuf)
	if !decisive {
		// Ответ составной — сверить нельзя, покажем и предложим оценить самому.
		ps.phase = phaseMark
		ps.show(ctx)
		return
	}
	ps.record(ctx, ok)
	if ok {
		ctx.Toast("Верно!")
		ps.goNext(ctx)
		return
	}
	ps.phase = phaseDone
	ps.show(ctx)
}

func (ps *problemScreen) mark(ctx *app.Context, correct bool) {
	ps.record(ctx, correct)
	ps.goNext(ctx)
}

func (ps *problemScreen) record(ctx *app.Context, correct bool) {
	ps.a.st.record(ps.cur(), correct, ctx.Now())
	ps.a.save(ctx)
}

func (ps *problemScreen) toggleFlag(ctx *app.Context) {
	idx := ps.cur()
	ps.a.st.toggleFlag(idx)
	ps.a.save(ctx)
	ps.flagBtn.Active = ps.a.st.flags[idx]
	ctx.Invalidate(ps.flagBtn.Bounds(), eink.UI)
	if ps.a.st.flags[idx] {
		ctx.Toast("Отложено")
	} else {
		ctx.Toast("Снято с отложенных")
	}
}

func (ps *problemScreen) goNext(ctx *app.Context) {
	if ps.pos < len(ps.hist)-1 {
		ps.pos++
		ps.resetPhase(ctx)
		return
	}
	nx := ps.pick(ctx)
	if nx < 0 {
		ctx.Toast(ps.doneText())
		return
	}
	ps.hist = append(ps.hist, nx)
	ps.pos++
	ps.resetPhase(ctx)
}

func (ps *problemScreen) goPrev(ctx *app.Context) {
	if ps.pos > 0 {
		ps.pos--
		ps.resetPhase(ctx)
		return
	}
	ctx.Pop()
}

func (ps *problemScreen) resetPhase(ctx *app.Context) {
	ps.phase = phaseInput
	ps.typedBuf = ""
	ps.show(ctx)
}

func (ps *problemScreen) pick(ctx *app.Context) int {
	st := ps.a.st
	cur := ps.cur()
	switch ps.md {
	case modeOrder:
		return st.nextInOrder(cur + 1)
	case modeRandom:
		return st.randomUnsolved(ctx.Now())
	case modeReview:
		rev := st.review()
		for _, i := range rev {
			if i > cur {
				return i
			}
		}
		for _, i := range rev {
			if i != cur {
				return i
			}
		}
		return -1
	case modeSingle:
		if cur+1 < st.n {
			return cur + 1
		}
	}
	return -1
}

func (ps *problemScreen) doneText() string {
	if ps.md == modeReview {
		return "Трудных задач больше нет"
	}
	return "Все задачи решены!"
}

func (ps *problemScreen) show(ctx *app.Context) {
	if ps.b.Empty() {
		return
	}
	ps.build(ctx)
	ctx.InvalidateAll(eink.Clean)
}

func (ps *problemScreen) Draw(c *gfx.Canvas, t *ui.Theme) { ps.page.Draw(c, t) }

func (ps *problemScreen) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case app.Lifecycle:
		switch e {
		case app.Shown:
			ps.show(ctx)
		case app.Hidden, app.Paused:
			ps.a.save(ctx)
		}
		return
	case device.KeyEvent:
		if e.Pressed {
			switch e.Key {
			case device.KeyPageForward:
				ps.goNext(ctx)
				return
			case device.KeyPageBack:
				ps.goPrev(ctx)
				return
			}
		}
	case ui.Gesture:
		if e.Kind == ui.Swipe {
			switch e.Dir {
			case ui.DirLeft:
				ps.goNext(ctx)
				return
			case ui.DirRight:
				ps.goPrev(ctx)
				return
			}
		}
	}
	app.RoutePage(ctx, &ps.page, ev)
}

// fitFace выбирает самый крупный шрифт, при котором текст влезает в область w×h.
func fitFace(t *ui.Theme, text string, w, h int) *gfx.Face {
	for _, pt := range []float64{17, 15, 14, 13, 12} {
		f := t.Face(gfx.Regular, pt)
		if len(f.Wrap(text, w))*f.Height <= h {
			return f
		}
	}
	return t.Face(gfx.Regular, 11)
}
