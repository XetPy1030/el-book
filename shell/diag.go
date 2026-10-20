package shell

import (
	"fmt"
	"image"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ui"
)

// diagScreen — диагностика книги: сведения об устройстве, проверка касаний и поворотов,
// замер скорости обновления панели и отчёт в папку выгрузки. Нужна при первом запуске
// на книге и после обновления прошивки.
type diagScreen struct {
	s       *Shell
	page    ui.Page
	info    *ui.Label
	pager   *ui.Label // «1 / 2», если отчёт не влез на экран
	lines   []string  // строки отчёта после переноса по ширине
	perPage int
	pageNo  int
	results []string
	touches int
}

func newDiag(s *Shell) *diagScreen { return &diagScreen{s: s} }

func (d *diagScreen) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	d.page.Clear()
	pad := t.Pad()
	x0, x1 := b.Min.X+pad, b.Max.X-pad
	y := b.Min.Y + t.MM(2)

	back := &ui.Button{Icon: icons.Back, Style: ui.Flat, OnTap: func() { ctx.Pop() }}
	back.SetBounds(image.Rect(x0, y, x0+t.Touch(), y+t.Touch()))
	title := &ui.Label{Text: "Диагностика", Face: func(t *ui.Theme) *gfx.Face { return t.Title() }}
	title.SetBounds(image.Rect(back.R.Max.X+t.Gap(), y, x1, y+t.Touch()))
	d.page.Add(back, title)
	y += t.Touch() + t.MM(4)

	actions := &ui.HStack{Gap: t.Gap(), Items: []ui.Widget{
		&ui.Button{Text: "Касания", Icon: icons.Pencil, OnTap: func() { ctx.Push(newTouchTest(d, ctx.Rotation())) }},
		&ui.Button{Text: "Замер экрана", Icon: icons.Bolt, OnTap: func() { d.startMeasure(ctx) }},
		&ui.Button{Text: "Отчёт", Icon: icons.Export, OnTap: func() { d.saveReport(ctx) }},
	}}
	actions.SetBounds(image.Rect(x0, y, x1, y+t.Touch()))
	d.page.Add(actions)
	y += t.Touch() + t.MM(4)

	pagerH := t.Caption().Height
	d.pager = &ui.Label{Align: gfx.AlignCenter, Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}
	d.pager.SetBounds(image.Rect(x0, b.Max.Y-pad-pagerH, x1, b.Max.Y-pad))
	d.info = &ui.Label{Wrap: true, VAlign: -1}
	d.info.SetBounds(image.Rect(x0, y, x1, d.pager.R.Min.Y-t.MM(1)))
	d.perPage = max(d.info.R.Dy()/t.Body().Height, 1)
	d.page.Add(d.info, d.pager)
	d.paginate(ctx)
}

// paginate раскладывает отчёт по страницам: прокрутки нет, длинный текст листается.
func (d *diagScreen) paginate(ctx *app.Context) {
	face := ctx.Theme().Body()
	d.lines = d.lines[:0]
	for _, line := range d.report(ctx) {
		d.lines = append(d.lines, face.Wrap(line, d.info.R.Dx())...)
	}
	pages := (len(d.lines) + d.perPage - 1) / d.perPage
	d.pageNo = min(max(d.pageNo, 0), max(pages-1, 0))
	from := d.pageNo * d.perPage
	d.info.Text = strings.Join(d.lines[from:min(from+d.perPage, len(d.lines))], "\n")
	d.pager.Text = ""
	if pages > 1 {
		d.pager.Text = fmt.Sprintf("%d / %d", d.pageNo+1, pages)
	}
}

// turn листает отчёт; за первую и последнюю страницу не листает, чтобы экран не мигал впустую.
func (d *diagScreen) turn(ctx *app.Context, delta int) {
	pages := (len(d.lines) + d.perPage - 1) / d.perPage
	if next := d.pageNo + delta; next >= 0 && next < pages {
		d.pageNo = next
		d.refresh(ctx)
	}
}

func (d *diagScreen) Draw(c *gfx.Canvas, t *ui.Theme) { d.page.Draw(c, t) }

func (d *diagScreen) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case device.KeyEvent:
		if e.Pressed && e.Key == device.KeyPageForward {
			d.turn(ctx, 1)
		} else if e.Pressed && e.Key == device.KeyPageBack {
			d.turn(ctx, -1)
		}
		return
	case ui.Gesture:
		if e.Kind == ui.Swipe {
			d.turn(ctx, map[ui.Direction]int{ui.DirLeft: 1, ui.DirRight: -1}[e.Dir])
			return
		}
	case app.Lifecycle:
		if e == app.Shown && d.info != nil { // вернулись из проверки касаний
			d.refresh(ctx)
		}
	}
	app.RoutePage(ctx, &d.page, ev)
}

func (d *diagScreen) refresh(ctx *app.Context) {
	d.paginate(ctx)
	ctx.Invalidate(d.info.R.Union(d.pager.R), eink.UI)
}

// report — сведения об устройстве и результаты проверок, по строке на факт.
func (d *diagScreen) report(ctx *app.Context) []string {
	dev := ctx.Sys().Device()
	disp := dev.Display()
	b := disp.Bounds()
	lines := []string{
		"Время: " + ctx.Now().Format("2006-01-02 15:04"),
		"el-book: " + orDash(d.s.opts.Version),
		fmt.Sprintf("Экран: %d×%d, %d dpi, %s", b.Dx(), b.Dy(), disp.DPI(), disp.Rotation()),
	}
	if lvl, err := dev.Battery().Level(); err == nil {
		charging, _ := dev.Battery().Charging()
		state := "не заряжается"
		if charging {
			state = "заряжается"
		}
		lines = append(lines, fmt.Sprintf("Батарея: %d%%, %s", lvl, state))
	} else {
		lines = append(lines, "Батарея: "+err.Error())
	}
	if l := dev.Light(); l != nil {
		lines = append(lines, fmt.Sprintf("Подсветка: яркость %d%%, теплота %d%%", l.Brightness(), l.Warmth()))
	}
	if ds, ok := dev.(device.Describer); ok {
		for _, f := range ds.Describe() {
			lines = append(lines, f.Name+": "+orDash(f.Value))
		}
	} else {
		info := dev.Info()
		lines = append(lines, "Модель: "+info.Model, "Прошивка: "+orDash(info.Firmware))
	}
	updates, waits := ctx.Sys().Stats()
	lines = append(lines, fmt.Sprintf("Обновлений панели за сеанс: %d, ожиданий: %d", updates, waits))
	if d.touches > 0 {
		lines = append(lines, "Проверка касаний: "+ui.Count(d.touches, "касание", "касания", "касаний"))
	}
	if len(d.results) > 0 {
		lines = append(lines, "Замер обновления, мс (два прогона; в скобках оценка платформы):")
		lines = append(lines, d.results...)
	}
	return lines
}

// measureCases — режимы, которые меряет «Замер экрана».
var measureCases = []struct {
	name string
	u    device.Update
}{
	{"DU", device.Update{Waveform: device.WaveformDU}},
	{"A2", device.Update{Waveform: device.WaveformA2}},
	{"GL16", device.Update{Waveform: device.WaveformGL16}},
	{"REAGL", device.Update{Waveform: device.WaveformREAGL}},
	{"GC16", device.Update{Waveform: device.WaveformGC16}},
	{"GC16 полное", device.Update{Waveform: device.WaveformGC16, Full: true}},
}

func (d *diagScreen) startMeasure(ctx *app.Context) {
	d.results = []string{"  идёт замер, экран будет мигать…"}
	d.refresh(ctx)
	// Замер блокирует цикл, поэтому начинается после того, как надпись нарисована.
	ctx.After(300*time.Millisecond, func() { d.measure(ctx) })
}

// measure отправляет обновления прямо на дисплей, в обход цикла, и меряет время от
// отправки до конца обновления. Потом экран перерисовывается целиком.
func (d *diagScreen) measure(ctx *app.Context) {
	disp := ctx.Sys().Device().Display()
	b := disp.Bounds()
	side := min(b.Dx(), b.Dy()) / 2
	r := image.Rect(0, 0, side, side).Add(image.Pt(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2))
	src := image.NewGray(b)
	c := gfx.NewCanvas(src)
	c.Fill(b, 255)
	time.Sleep(time.Second) // обновления, отправленные циклом, успевают закончиться
	estimates := eink.DefaultDurations()
	var res []string
	for _, mc := range measureCases {
		var runs []string
		for run := 0; run < 2; run++ {
			paintBands(c, r, run == 1)
			u := mc.u
			u.Rect = r
			start := time.Now()
			m, err := disp.Present(src, u)
			if err == nil {
				err = disp.Wait(m)
			}
			if err != nil {
				runs = append(runs, "ошибка: "+err.Error())
				continue
			}
			runs = append(runs, fmt.Sprint(time.Since(start).Milliseconds()))
		}
		est := estimates[mc.u.Waveform].Milliseconds()
		res = append(res, fmt.Sprintf("  %s: %s (%d)", mc.name, strings.Join(runs, ", "), est))
	}
	d.results = res
	ctx.Log().Info("замер обновления", "результаты", strings.Join(res, ";"))
	d.paginate(ctx)
	ctx.InvalidateAll(eink.Clean)
}

// paintBands рисует 16 полос от чёрного к белому; reverse — наоборот, чтобы каждый
// прогон менял все пиксели.
func paintBands(c *gfx.Canvas, r image.Rectangle, reverse bool) {
	for i := 0; i < 16; i++ {
		level := i
		if reverse {
			level = 15 - i
		}
		x0 := r.Min.X + r.Dx()*i/16
		x1 := r.Min.X + r.Dx()*(i+1)/16
		c.Fill(image.Rect(x0, r.Min.Y, x1, r.Max.Y), gfx.Gray16(level))
	}
}

func (d *diagScreen) saveReport(ctx *app.Context) {
	text := strings.Join(d.report(ctx), "\n") + "\n"
	name := "diag-" + ctx.Now().Format("20060102-150405") + ".txt"
	path, err := ctx.Storage().Export(name, []byte(text))
	if err != nil {
		ctx.Fail("Не удалось сохранить отчёт", err)
		return
	}
	ctx.Toast("Отчёт сохранён: data/" + path)
}

// touchTest — проверка касаний: под пальцем рисуется крест с координатами. Повороты
// переключаются сверху, стрелка показывает логический верх экрана; «Готово» возвращает
// поворот, с которым проверку открыли. Внизу видно, какие кнопки листания нажимали и как
// датчик видит положение книги.
type touchTest struct {
	d      *diagScreen
	start  device.Rotation
	page   ui.Page
	bounds image.Rectangle
	header image.Rectangle
	status image.Rectangle // строки о кнопках и датчике
	marks  []touchMark
	keys   map[device.Key]int // нажатия кнопок листания
	sensor string             // положение книги по датчику
}

type touchMark struct {
	p     image.Point
	slot  int
	phase device.TouchPhase
}

// maxTouchMarks — сколько отметок копится до очистки. Очищаются они все разом при новом
// касании: стирать на ходу по одной нельзя, каждое стирание — медленное обновление, и они
// копились бы в очереди панели, а след отставал бы от пальца.
const maxTouchMarks = 300

func newTouchTest(d *diagScreen, start device.Rotation) *touchTest {
	return &touchTest{d: d, start: start, keys: map[device.Key]int{}}
}

func (tt *touchTest) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	tt.bounds = b
	tt.marks = nil
	tt.page.Clear()
	pad := t.Pad()
	y := b.Min.Y + t.MM(2)
	done := &ui.Button{Text: "Готово", Style: ui.Primary, OnTap: func() {
		tt.d.s.hw = nil
		ctx.Sys().SetRotation(tt.start)
		ctx.Pop()
	}}
	doneW := t.MM(28)
	done.SetBounds(image.Rect(b.Max.X-pad-doneW, y, b.Max.X-pad, y+t.Touch()))
	rot := &ui.Segmented{Options: []string{"0°", "90°", "180°", "270°"}, Selected: int(ctx.Rotation()), OnChange: func(i int) {
		ctx.Sys().SetRotation(device.Rotation(i))
	}}
	rot.SetBounds(image.Rect(b.Min.X+pad, y, done.R.Min.X-t.Gap(), y+t.Touch()))
	tt.page.Add(rot, done)
	tt.header = image.Rect(b.Min.X, b.Min.Y, b.Max.X, y+t.Touch()+t.MM(2))
	hintTop := b.Max.Y - t.Pad() - t.Body().Height
	tt.status = image.Rect(b.Min.X, hintTop-t.MM(2)-2*t.Body().Height, b.Max.X, hintTop-t.MM(2))
	tt.d.s.hw = tt.hardware
}

// hardware показывает нажатую кнопку листания или новое положение книги по датчику.
func (tt *touchTest) hardware(ctx *app.Context, ev device.Event) {
	switch e := ev.(type) {
	case device.KeyEvent:
		if !e.Pressed || (e.Key != device.KeyPageBack && e.Key != device.KeyPageForward) {
			return
		}
		tt.keys[e.Key]++
	case device.OrientationEvent:
		tt.sensor = bookPosition(e.Rotation)
	default:
		return
	}
	ctx.Invalidate(tt.status, eink.UI)
}

// statusLines — строки о кнопках и датчике, не больше двух.
func (tt *touchTest) statusLines() []string {
	if len(tt.keys) == 0 && tt.sensor == "" {
		return []string{"Нажмите кнопки листания и поверните книгу"}
	}
	var lines []string
	if len(tt.keys) > 0 {
		lines = append(lines, fmt.Sprintf("Кнопки: назад %d, вперёд %d", tt.keys[device.KeyPageBack], tt.keys[device.KeyPageForward]))
	}
	if tt.sensor != "" {
		lines = append(lines, "Датчик: "+tt.sensor)
	}
	return lines
}

// bookPosition — положение книги в руках по повороту, который датчик предлагает для
// содержимого: книгу повернули по часовой — содержимое надо повернуть против часовой.
func bookPosition(r device.Rotation) string {
	switch r {
	case device.LandscapeCCW:
		return "книга повёрнута по часовой"
	case device.PortraitFlipped:
		return "книга перевёрнута"
	case device.LandscapeCW:
		return "книга повёрнута против часовой"
	}
	return "книга стоит прямо"
}

func (tt *touchTest) Draw(c *gfx.Canvas, t *ui.Theme) {
	tt.page.Draw(c, t)
	b := tt.bounds
	c.Fill(image.Rect(b.Min.X, tt.header.Max.Y-t.Line(), b.Max.X, tt.header.Max.Y), 0)

	// Всё чёрное: кресты обновляются двухцветным DU, и серое под ними пропало бы до очистки.
	cx := (b.Min.X + b.Max.X) / 2
	cy := (tt.header.Max.Y + b.Max.Y) / 2
	h := t.MM(12)
	w := float64(t.Line() * 2)
	c.Line(image.Pt(cx, cy+h), image.Pt(cx, cy-h), w, 0)
	c.Line(image.Pt(cx, cy-h), image.Pt(cx-h/2, cy-h/2), w, 0)
	c.Line(image.Pt(cx, cy-h), image.Pt(cx+h/2, cy-h/2), w, 0)
	t.Caption().DrawIn(c, "верх экрана", image.Rect(b.Min.X, cy+h+t.MM(1), b.Max.X, cy+h+t.MM(1)+t.Caption().Height), gfx.AlignCenter, 0)
	hint := image.Rect(b.Min.X, b.Max.Y-t.Pad()-t.Body().Height, b.Max.X, b.Max.Y-t.Pad())
	t.Body().DrawIn(c, "Касайтесь экрана: крест должен появляться под пальцем", hint, gfx.AlignCenter, 0)
	lines := tt.statusLines()
	for i, line := range lines { // снизу вверх, вплотную к подсказке
		y := tt.status.Max.Y - (len(lines)-i)*t.Body().Height
		t.Body().DrawIn(c, line, image.Rect(b.Min.X, y, b.Max.X, y+t.Body().Height), gfx.AlignCenter, 0)
	}

	for _, m := range tt.marks {
		tt.drawMark(c, t, m)
	}
}

// markGeometry — крест и подпись отметки: подпись справа, у правого края — слева.
// Движение рисуется точкой без подписи.
func (tt *touchTest) markGeometry(t *ui.Theme, m touchMark) (rect image.Rectangle, labelX int) {
	if m.phase == device.TouchMove {
		r := t.MM(1)
		return image.Rect(m.p.X-r-2, m.p.Y-r-2, m.p.X+r+2, m.p.Y+r+2).Intersect(tt.bounds), 0
	}
	r := t.MM(6)
	labelW := t.Caption().Measure(markLabel(m))
	gap := t.MM(1)
	labelX = m.p.X + r + gap
	rect = image.Rect(m.p.X-r-2, m.p.Y-r-2, labelX+labelW+gap, m.p.Y+r+2)
	if rect.Max.X > tt.bounds.Max.X {
		labelX = m.p.X - r - gap - labelW
		rect = image.Rect(labelX-gap, m.p.Y-r-2, m.p.X+r+2, m.p.Y+r+2)
	}
	return rect.Intersect(tt.bounds), labelX
}

func markLabel(m touchMark) string {
	return fmt.Sprintf("%d, %d · %d", m.p.X, m.p.Y, m.slot)
}

func (tt *touchTest) drawMark(c *gfx.Canvas, t *ui.Theme, m touchMark) {
	x, y := m.p.X, m.p.Y
	if m.phase == device.TouchMove {
		c.Disc(float64(x), float64(y), float64(t.MM(1)), 0, true)
		return
	}
	r := t.MM(6)
	w := float64(t.Line())
	c.Line(image.Pt(x-r, y), image.Pt(x+r, y), w, 0)
	c.Line(image.Pt(x, y-r), image.Pt(x, y+r), w, 0)
	if m.phase == device.TouchDown {
		c.Ring(float64(x), float64(y), float64(r)*0.6, w, 0)
	} else {
		c.Disc(float64(x), float64(y), float64(r)*0.3, 0, true)
	}
	_, labelX := tt.markGeometry(t, m)
	t.Caption().Draw(c, markLabel(m), labelX, y-t.MM(1), 0)
}

func (tt *touchTest) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case device.TouchEvent:
		if !e.Point().In(tt.header) {
			tt.add(ctx, e)
			return
		}
	case device.KeyEvent:
		return // кнопки листания показываются внизу через hardware
	}
	app.RoutePage(ctx, &tt.page, ev)
}

func (tt *touchTest) add(ctx *app.Context, e device.TouchEvent) {
	t := ctx.Theme()
	m := touchMark{p: e.Point(), slot: e.Slot, phase: e.Phase}
	if n := len(tt.marks); n > 0 && e.Phase == device.TouchMove {
		last := tt.marks[n-1]
		d := last.p.Sub(m.p)
		if last.slot == m.slot && d.X*d.X+d.Y*d.Y < t.MM(3)*t.MM(3) {
			return // движение рисуется не чаще, чем через 3 мм
		}
	}
	if e.Phase == device.TouchDown {
		tt.d.touches++
		if len(tt.marks) >= maxTouchMarks {
			tt.marks = tt.marks[:0]
			ctx.Invalidate(image.Rect(tt.bounds.Min.X, tt.header.Max.Y, tt.bounds.Max.X, tt.bounds.Max.Y), eink.UI)
		}
	}
	tt.marks = append(tt.marks, m)
	r, _ := tt.markGeometry(t, m)
	class := eink.Feedback
	if e.Phase == device.TouchMove {
		class = eink.Stroke // след пальца: обновления склеиваются, как при рисовании
	}
	ctx.Invalidate(r, class)
}
