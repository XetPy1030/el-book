// Package timer — таймер обратного отсчёта с готовыми интервалами и секундомер с кругами.
// Состояние хранится в настройках: идущий таймер переживает уход с экрана и перезапуск,
// а оставшееся время видно на домашнем экране.
package timer

import (
	"fmt"
	"image"
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

// App — приложение «Таймер»; оно же его экран.
type App struct {
	st    *kit.Prefs[state]
	page  ui.Page
	digit *ui.Custom
	start *ui.Button
	reset *ui.Button
	extra *ui.Button
	mode  *ui.Segmented
	laps  *ui.List
	tick  func()
	alarm *ui.Panel
	now   time.Time // время последнего обновления: по нему рисуются цифры
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "timer", Title: "Таймер", Icon: icons.Timer}
}

const stateVersion = 1

// state — состояние таймера и секундомера; время — Unix-миллисекунды.
type state struct {
	Stopwatch bool    `json:"stopwatch"` // показан секундомер
	Duration  int64   `json:"duration"`  // длина таймера, мс
	EndsAt    int64   `json:"ends_at"`   // когда таймер кончится; 0 — не идёт
	Left      int64   `json:"left"`      // остаток на паузе, мс
	Finished  int64   `json:"finished"`  // когда таймер кончился и это ещё не увидели
	SwStart   int64   `json:"sw_start"`  // когда запущен секундомер; 0 — стоит
	SwAcc     int64   `json:"sw_acc"`    // накоплено до паузы, мс
	Laps      []int64 `json:"laps"`      // круги: время от начала, мс
}

func defaultState() state { return state{Duration: 5 * 60 * 1000, Left: 5 * 60 * 1000} }

// presets — готовые интервалы, мин.
var presets = []int{1, 3, 5, 10, 15, 25, 45, 60}

func ms(t time.Time) int64 { return t.UnixMilli() }

func (s *state) timerLeft(now time.Time) int64 {
	if s.EndsAt > 0 {
		return max(s.EndsAt-ms(now), 0)
	}
	return s.Left
}

func (s *state) swElapsed(now time.Time) int64 {
	if s.SwStart > 0 {
		return s.SwAcc + ms(now) - s.SwStart
	}
	return s.SwAcc
}

// digits — цифры на экране. Отсчёт таймера округляется вверх: «05:00» стоит первую секунду,
// «00:01» — до самого сигнала. Секундомер на ходу показывает целые секунды: экран обновляется
// раз в секунду.
func (s *state) digits(now time.Time) string {
	if s.Stopwatch {
		return clock(s.swElapsed(now), s.SwStart == 0)
	}
	return clock(s.timerLeft(now)+999, false)
}

// untilChange — через сколько сменятся цифры на экране.
func (s *state) untilChange(now time.Time) time.Duration {
	ms := (s.timerLeft(now)-1)%1000 + 1
	if s.Stopwatch {
		ms = 1000 - s.swElapsed(now)%1000
	}
	return time.Duration(ms) * time.Millisecond
}

// clock — «04:59», «1:02:03»; tenths — с десятыми: «04:59,3».
func clock(msec int64, tenths bool) string {
	if msec < 0 {
		msec = 0
	}
	s := msec / 1000
	out := fmt.Sprintf("%02d:%02d", s/60%60, s%60)
	if s >= 3600 {
		out = fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	if tenths {
		out += fmt.Sprintf(",%d", msec/100%10)
	}
	return out
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.st = kit.LoadPrefs(ctx.Storage(), stateVersion, defaultState())
	return a
}

func (a *App) save(ctx *app.Context) {
	if err := a.st.Save(); err != nil {
		ctx.Log().Error("таймер", "err", err)
	}
}

func (a *App) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	a.page.Clear()
	r := b
	bar := ui.NewTitleBar(t, "Таймер", func() { ctx.Home() })
	bar.BackIcon = icons.Home
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	r = ui.Pad(r, t.Pad(), t.MM(4), t.Pad(), t.Pad())
	sel := 0
	if a.st.Value.Stopwatch {
		sel = 1
	}
	a.mode = &ui.Segmented{Options: []string{"Таймер", "Секундомер"}, Selected: sel, OnChange: func(i int) {
		a.st.Value.Stopwatch = i == 1
		a.save(ctx)
		a.Layout(ctx, b)
		ctx.InvalidateAll(eink.UI)
	}}
	a.mode.SetBounds(ui.CutTop(&r, t.Touch()))
	ui.CutTop(&r, t.MM(6))
	a.digit = &ui.Custom{Paint: a.paintDigits}
	a.digit.SetBounds(ui.CutTop(&r, t.MM(34)))
	ui.CutTop(&r, t.MM(4))
	buttons := ui.CutTop(&r, t.Touch())
	cols := ui.Columns(buttons, 3, t.Gap())
	a.start = &ui.Button{Style: ui.Primary, OnTap: func() { a.toggle(ctx) }}
	a.reset = &ui.Button{Text: "Сброс", Icon: icons.Stop, OnTap: func() { a.resetAll(ctx) }}
	a.extra = &ui.Button{OnTap: func() { a.extraAction(ctx) }}
	a.reset.SetBounds(cols[0])
	a.start.SetBounds(cols[1])
	a.extra.SetBounds(cols[2])
	a.page.Add(bar, a.mode, a.digit, a.reset, a.start, a.extra)
	ui.CutTop(&r, t.MM(6))
	if a.st.Value.Stopwatch {
		a.laps = ui.NewList(t)
		a.laps.Empty = "Круги появятся здесь: кнопка «Круг» во время отсчёта."
		a.laps.SetBounds(r)
		a.page.Add(a.laps)
	} else {
		a.laps = nil
		f := t.Caption()
		lbl := &ui.Label{Text: "Готовые интервалы, мин", Color: gfx.Gray16(5), Face: func(*ui.Theme) *gfx.Face { return f }}
		lbl.SetBounds(ui.CutTop(&r, f.Height+t.MM(2)))
		a.page.Add(lbl)
		rows := ui.Rows(ui.CutTop(&r, 2*t.Touch()+t.Gap()), 2, t.Gap())
		for i := 0; i < 2; i++ {
			cells := ui.Columns(rows[i], 4, t.Gap())
			for j := 0; j < 4; j++ {
				m := presets[i*4+j]
				btn := &ui.Button{Text: fmt.Sprint(m), OnTap: func() { a.setPreset(ctx, m) }}
				btn.SetBounds(cells[j])
				a.page.Add(btn)
			}
		}
	}
	a.refresh(ctx)
}

func (a *App) Draw(c *gfx.Canvas, t *ui.Theme) { a.page.Draw(c, t) }

func (a *App) Handle(ctx *app.Context, ev app.Event) {
	if e, ok := ev.(app.Lifecycle); ok {
		switch e {
		case app.Shown, app.Resumed:
			a.refresh(ctx)
		case app.Hidden, app.Paused:
			if a.tick != nil {
				a.tick()
				a.tick = nil
			}
		}
	}
	app.RoutePage(ctx, &a.page, ev)
}

// running — идёт ли то, что показано.
func (a *App) running() bool {
	s := &a.st.Value
	if s.Stopwatch {
		return s.SwStart > 0
	}
	return s.EndsAt > 0
}

// refresh обновляет кнопки, круги и цифры после изменения состояния и, пока что-то идёт,
// заводит отсчёт.
func (a *App) refresh(ctx *app.Context) {
	if a.start == nil {
		return
	}
	s := &a.st.Value
	now := ctx.Now()
	a.now = now
	if !s.Stopwatch && s.EndsAt > 0 && s.timerLeft(now) == 0 {
		a.finish(ctx)
	}
	if a.running() {
		a.start.Text, a.start.Icon = "Пауза", icons.Pause
	} else {
		a.start.Text, a.start.Icon = "Старт", icons.Play
	}
	if s.Stopwatch {
		a.extra.Text, a.extra.Icon, a.extra.Disabled = "Круг", icons.Flag, s.SwStart == 0
		var items []ui.ListItem
		for i := len(s.Laps) - 1; i >= 0; i-- {
			prev := int64(0)
			if i > 0 {
				prev = s.Laps[i-1]
			}
			items = append(items, ui.ListItem{Title: fmt.Sprintf("Круг %d", i+1), Detail: clock(s.Laps[i]-prev, true), Subtitle: "всего " + clock(s.Laps[i], true)})
		}
		a.laps.SetItems(items)
		a.laps.Refresh(ctx)
	} else {
		a.extra.Text, a.extra.Icon, a.extra.Disabled = "+1 мин", icons.Plus, false
	}
	ctx.Invalidate(a.start.R, eink.UI)
	ctx.Invalidate(a.extra.R, eink.UI)
	ctx.Invalidate(a.digit.R, eink.UI)
	a.schedule(ctx)
}

// schedule заводит следующее обновление цифр — к моменту, когда сменится секунда на экране.
// Раз в секунду, а не чаще: панели e-ink нужно 0,4 с на обновление с оттенками, и частые
// обновления только копятся в контроллере.
func (a *App) schedule(ctx *app.Context) {
	if a.tick != nil {
		a.tick()
		a.tick = nil
	}
	if !a.running() || !ctx.Visible() {
		return
	}
	a.tick = ctx.After(a.st.Value.untilChange(a.now), func() {
		a.tick = nil
		a.second(ctx)
	})
}

// second — очередная секунда: меняется только строка цифр, а кончившийся таймер меняет и кнопки.
func (a *App) second(ctx *app.Context) {
	s := &a.st.Value
	a.now = ctx.Now()
	if !s.Stopwatch && s.EndsAt > 0 && s.timerLeft(a.now) == 0 {
		a.refresh(ctx)
		return
	}
	r := a.digit.R
	r.Max.Y = r.Min.Y + digitFace(ctx.Theme()).Height
	ctx.Invalidate(r, eink.UI)
	a.schedule(ctx)
}

func digitFace(t *ui.Theme) *gfx.Face { return t.Face(gfx.Medium, 66) }

func (a *App) paintDigits(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	s := &a.st.Value
	text := s.digits(a.now)
	sub := "длина " + ru.Duration(time.Duration(s.Duration)*time.Millisecond)
	if s.Stopwatch {
		sub = ""
	} else if s.EndsAt > 0 {
		sub = "закончится в " + time.UnixMilli(s.EndsAt).Format("15:04")
	} else if s.Finished > 0 {
		sub = "время вышло в " + time.UnixMilli(s.Finished).Format("15:04")
	}
	big := digitFace(t)
	big.DrawIn(c, text, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+big.Height), gfx.AlignCenter, gfx.Black)
	if sub != "" {
		t.Body().DrawIn(c, sub, image.Rect(r.Min.X, r.Max.Y-t.Body().Height, r.Max.X, r.Max.Y), gfx.AlignCenter, gfx.Gray16(4))
	}
}

func (a *App) toggle(ctx *app.Context) {
	s := &a.st.Value
	now := ms(ctx.Now())
	switch {
	case s.Stopwatch && s.SwStart > 0:
		s.SwAcc += now - s.SwStart
		s.SwStart = 0
	case s.Stopwatch:
		s.SwStart = now
	case s.EndsAt > 0:
		s.Left = max(s.EndsAt-now, 0)
		s.EndsAt = 0
	default:
		if s.Left <= 0 {
			s.Left = s.Duration
		}
		s.EndsAt = now + s.Left
		s.Finished = 0
	}
	a.save(ctx)
	a.refresh(ctx)
}

func (a *App) resetAll(ctx *app.Context) {
	s := &a.st.Value
	if s.Stopwatch {
		s.SwStart, s.SwAcc, s.Laps = 0, 0, nil
	} else {
		s.EndsAt, s.Left, s.Finished = 0, s.Duration, 0
	}
	a.save(ctx)
	a.refresh(ctx)
}

func (a *App) extraAction(ctx *app.Context) {
	s := &a.st.Value
	if s.Stopwatch {
		if s.SwStart > 0 {
			s.Laps = append(s.Laps, s.swElapsed(ctx.Now()))
		}
	} else if s.EndsAt > 0 {
		s.EndsAt += 60 * 1000
	} else {
		s.Left += 60 * 1000
	}
	a.save(ctx)
	a.refresh(ctx)
}

func (a *App) setPreset(ctx *app.Context, minutes int) {
	s := &a.st.Value
	s.Duration = int64(minutes) * 60 * 1000
	s.Left = s.Duration
	s.EndsAt, s.Finished = 0, 0
	a.save(ctx)
	a.toggle(ctx)
}

// finish — время вышло: окно и запомненное время для домашнего экрана.
func (a *App) finish(ctx *app.Context) {
	s := &a.st.Value
	s.Finished = s.EndsAt
	s.EndsAt, s.Left = 0, s.Duration
	a.save(ctx)
	if a.alarm != nil && ctx.HasOverlay(a.alarm) {
		return
	}
	a.alarm = ui.NewDialog(ctx.Theme(), "Время вышло",
		fmt.Sprintf("Таймер на %s закончился в %s.", ru.Duration(time.Duration(s.Duration)*time.Millisecond), time.UnixMilli(s.Finished).Format("15:04")),
		ui.DialogButton{Text: "Ещё раз", OnTap: func() { a.toggle(ctx) }},
		ui.DialogButton{Text: "Готово", Primary: true, OnTap: func() {
			s.Finished = 0
			a.save(ctx)
			a.refresh(ctx)
		}},
	)
	ctx.ShowOverlay(a.alarm)
}

// Summary — идущий таймер или секундомер на домашнем экране.
func (a *App) Summary(st *storage.App, now time.Time) (app.Summary, bool) {
	p := kit.LoadPrefs(st, stateVersion, defaultState())
	s := &p.Value
	switch {
	case s.EndsAt > 0 && s.timerLeft(now) > 0:
		left := time.Duration(s.timerLeft(now)) * time.Millisecond
		return app.Summary{Text: "Таймер: осталось " + ru.Duration(left.Round(time.Minute)) + ", до " + time.UnixMilli(s.EndsAt).Format("15:04")}, true
	case s.EndsAt > 0 || s.Finished > 0:
		at := s.Finished
		if at == 0 {
			at = s.EndsAt
		}
		return app.Summary{Text: "Таймер: время вышло в " + time.UnixMilli(at).Format("15:04")}, true
	case s.SwStart > 0:
		return app.Summary{Text: "Секундомер: " + ru.Duration(time.Duration(s.swElapsed(now))*time.Millisecond)}, true
	}
	return app.Summary{}, false
}
