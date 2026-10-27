package rachinsky

import (
	"fmt"
	"image"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// App — тренажёр «Устный счёт».
type App struct {
	st           *state
	browseFilter int
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "rachinsky", Title: "Устный счёт", Icon: icons.Book}
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.st = loadState(ctx.Storage(), len(problems))
	return &dashboard{a: a}
}

// Pause и Stop сохраняют прогресс перед сном, выходом в Nickel и закрытием приложения.
func (a *App) Pause(ctx *app.Context) { a.save(ctx) }
func (a *App) Stop(ctx *app.Context)  { a.save(ctx) }

func (a *App) save(ctx *app.Context) {
	if err := a.st.save(ctx.Storage()); err != nil {
		ctx.Fail("Не удалось сохранить прогресс", err)
	}
}

// Summary — строка на домашнем экране: сколько задач решено.
func (a *App) Summary(st *storage.App, now time.Time) (app.Summary, bool) {
	s := loadState(st, len(problems))
	if s.attempts == 0 {
		return app.Summary{}, false
	}
	if s.solved() >= s.n {
		return app.Summary{Icon: icons.Book, Text: "Все задачи решены!"}, true
	}
	return app.Summary{Icon: icons.Book, Text: fmt.Sprintf("Решено %d из %d задач", s.solved(), s.n)}, true
}

// --- Навигация к задачам ---

func (a *App) openContinue(ctx *app.Context) {
	idx := a.st.nextInOrder(a.st.cursor)
	if idx < 0 {
		ctx.Toast("Все 999 задач решены!")
		return
	}
	ctx.Push(newProblemScreen(a, modeOrder, idx))
}

func (a *App) openRandom(ctx *app.Context) {
	idx := a.st.randomUnsolved(ctx.Now())
	if idx < 0 {
		ctx.Toast("Все задачи решены — выбирать не из чего")
		return
	}
	ctx.Push(newProblemScreen(a, modeRandom, idx))
}

func (a *App) openReview(ctx *app.Context) {
	r := a.st.review()
	if len(r) == 0 {
		ctx.Toast("Нет отложенных задач и ошибок")
		return
	}
	ctx.Push(newProblemScreen(a, modeReview, r[0]))
}

func (a *App) openProblem(ctx *app.Context, idx int) {
	ctx.Push(newProblemScreen(a, modeSingle, idx))
}

func (a *App) openStats(ctx *app.Context) { ctx.Push(&statsScreen{a: a}) }

func (a *App) openMeasures(ctx *app.Context, us []Unit) {
	var blocks []ui.Block
	for _, u := range us {
		blocks = append(blocks,
			ui.Block{Kind: ui.Subheading, Text: u.Name},
			ui.Block{Kind: ui.Para, Text: u.Note})
	}
	ctx.Push(&kit.ReaderScreen{Title: "Меры в задаче", Blocks: blocks})
}

// --- Настройки ---

func (a *App) openSettings(ctx *app.Context) {
	ctx.Push(a.settingsScreen(ctx))
}

func (a *App) settingsScreen(ctx *app.Context) app.Screen {
	s := &kit.ListScreen{Title: "Настройки"}
	s.Items = func(ctx *app.Context) []ui.ListItem {
		modeText := "Самопроверка: посчитать в уме, затем сверить"
		if a.st.typed {
			modeText = "Ввод ответа: набрать ответ, приложение сверит"
		}
		return []ui.ListItem{
			{
				Title: "Способ ответа", Subtitle: modeText, Icon: icons.Keyboard,
				Detail: boolText(a.st.typed, "ввод", "в уме"),
				OnTap:  func() { a.toggleMode(ctx, s) },
			},
			{
				Title: "Сбросить прогресс", Subtitle: "Забыть все отметки и статистику",
				Icon: icons.Trash, OnTap: func() { a.confirmReset(ctx, s) },
			},
			{
				Title: "О задачнике", Icon: icons.Info,
				Subtitle: "С. А. Рачинский. 1001 задача для умственного счёта",
				OnTap:    func() { a.openAbout(ctx) },
			},
		}
	}
	return s
}

func (a *App) toggleMode(ctx *app.Context, s *kit.ListScreen) {
	a.st.typed = !a.st.typed
	a.save(ctx)
	s.Refresh(ctx)
}

func (a *App) confirmReset(ctx *app.Context, s *kit.ListScreen) {
	ctx.Confirm("Сбросить прогресс", "Все отметки, серии и статистика будут забыты. Это нельзя отменить.", "Сбросить", func() {
		typed := a.st.typed
		a.st = newState(len(problems))
		a.st.typed = typed
		a.save(ctx)
		s.Refresh(ctx)
		ctx.Toast("Прогресс сброшен")
	})
}

func (a *App) openAbout(ctx *app.Context) {
	ctx.Push(&kit.ReaderScreen{
		Title: "О задачнике",
		Blocks: []ui.Block{
			{Kind: ui.Para, Text: "Сергей Александрович Рачинский (1833–1902) — учёный и сельский учитель. В его школе крестьянские дети славились быстрым устным счётом."},
			{Kind: ui.Para, Text: "«1001 задача для умственного счёта» — его задачник: короткие задачи на счёт в уме, с ответами. Здесь собрано 999 задач этого сборника."},
			{Kind: ui.Para, Text: "В задачах часто встречаются старые русские меры. В каждой задаче, где они есть, кнопка «Меры» подскажет, чему они равны."},
			{Kind: ui.Meta, Text: "Текст задачника — из открытого доступа (Project Gutenberg, royallib.com)."},
		},
	})
}

// --- Дашборд ---

type dashboard struct {
	a    *App
	page ui.Page
	b    image.Rectangle
}

func (d *dashboard) Layout(ctx *app.Context, b image.Rectangle) {
	d.b = b
	d.build(ctx)
}

func (d *dashboard) build(ctx *app.Context) {
	t := ctx.Theme()
	d.page.Clear()
	r := d.b
	bar := ui.NewTitleBar(t, "Устный счёт", func() { ctx.Home() },
		&ui.Button{Icon: icons.Settings, Style: ui.Flat, OnTap: func() { d.a.openSettings(ctx) }})
	bar.Subtitle = "Задачи С. А. Рачинского"
	bar.BackIcon = icons.Home
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	d.page.Add(bar)

	body := ui.Pad(r, t.Pad()+t.MM(1), t.MM(3), t.Pad()+t.MM(1), t.MM(3))

	header := &ui.Custom{Paint: func(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) { d.paintHeader(c, t, r) }}
	header.SetBounds(ui.CutTop(&body, t.MM(32)))
	d.page.Add(header)
	ui.CutTop(&body, t.MM(4))

	st := d.a.st
	allSolved := st.solved() >= st.n
	contText := "Продолжить"
	if st.solved() == 0 {
		contText = "Начать"
	}
	d.addButton(ctx, &body, &ui.Button{Text: contText, Style: ui.Primary, Disabled: allSolved,
		OnTap: func() { d.a.openContinue(ctx) }})
	d.addButton(ctx, &body, &ui.Button{Text: "Случайная задача", Icon: icons.Dice, Style: ui.Secondary, Disabled: allSolved,
		OnTap: func() { d.a.openRandom(ctx) }})

	review := len(st.review())
	d.addButton(ctx, &body, &ui.Button{Text: "Повторить трудные", Icon: icons.Refresh, Style: ui.Secondary, Disabled: review == 0,
		OnTap: func() { d.a.openReview(ctx) }})
	d.addButton(ctx, &body, &ui.Button{Text: "Статистика", Icon: icons.Sparkle, Style: ui.Secondary,
		OnTap: func() { d.a.openStats(ctx) }})
	d.addButton(ctx, &body, &ui.Button{Text: "Все задачи", Icon: icons.Pages, Style: ui.Secondary,
		OnTap: func() { d.a.openBrowse(ctx) }})
}

func (d *dashboard) addButton(ctx *app.Context, body *image.Rectangle, b *ui.Button) {
	t := ctx.Theme()
	b.SetBounds(ui.CutTop(body, t.MM(12)))
	ui.CutTop(body, t.MM(2.5))
	d.page.Add(b)
}

func (d *dashboard) paintHeader(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	st := d.a.st
	solved := st.solved()
	// Крупно: сколько решено из всех.
	big := t.Face(gfx.Bold, 34)
	small := t.Face(gfx.Regular, 13)
	num := fmt.Sprintf("%d", solved)
	numW := big.Measure(num)
	tail := fmt.Sprintf(" из %d решено", st.n)
	line := r.Min.Y + big.Ascent
	big.Draw(c, num, r.Min.X, line, gfx.Black)
	small.Draw(c, tail, r.Min.X+numW+t.MM(2), r.Min.Y+big.Ascent-big.Ascent/4, gfx.Gray16(3))

	// Полоса прогресса.
	barR := image.Rect(r.Min.X, r.Min.Y+big.Height+t.MM(2), r.Max.X, r.Min.Y+big.Height+t.MM(2)+t.MM(4))
	drawBar(c, t, barR, float64(solved)/float64(st.n))

	// Подпись под полосой: точность, серия, отложено.
	var parts []string
	if acc := st.accuracy(); acc >= 0 {
		parts = append(parts, fmt.Sprintf("верно %d%%", acc))
	}
	if st.curStreak > 0 {
		parts = append(parts, "подряд "+itoa(st.curStreak))
	}
	if f := st.flagged(); f > 0 {
		parts = append(parts, ru.Count(f, "отложена", "отложено", "отложено"))
	}
	caption := "Решайте задачи в уме"
	if len(parts) > 0 {
		caption = strings.Join(parts, " · ")
	}
	capFace := t.Caption()
	capFace.Draw(c, caption, r.Min.X, barR.Max.Y+t.MM(2)+capFace.Ascent, gfx.Gray16(4))
}

func (d *dashboard) Draw(c *gfx.Canvas, t *ui.Theme) { d.page.Draw(c, t) }

func (d *dashboard) Handle(ctx *app.Context, ev app.Event) {
	if lc, ok := ev.(app.Lifecycle); ok {
		switch lc {
		case app.Shown:
			d.build(ctx)
			ctx.InvalidateAll(eink.Clean)
		case app.Hidden, app.Paused:
			d.a.save(ctx)
		}
		return
	}
	app.RoutePage(ctx, &d.page, ev)
}

// drawBar рисует полосу прогресса: рамка и заполнение на долю frac (0..1).
func drawBar(c *gfx.Canvas, t *ui.Theme, r image.Rectangle, frac float64) {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	c.StrokeRoundRect(r, r.Dy()/2, t.Line(), gfx.Black)
	w := int(float64(r.Dx()) * frac)
	if w < r.Dy() && w > 0 {
		w = r.Dy() // виден даже маленький прогресс
	}
	if w > 0 {
		c.FillRoundRect(image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), r.Dy()/2, gfx.Black)
	}
}

func boolText(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
