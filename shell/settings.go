package shell

import (
	"fmt"
	"slices"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
)

// showLight открывает панель подсветки: свет меняется прямо во время перетаскивания.
func (s *Shell) showLight(ctx *app.Context) {
	light := ctx.Sys().Device().Light()
	if light == nil {
		ctx.Toast("У этой книги нет подсветки")
		return
	}
	panel := &ui.Panel{Title: "Подсветка", Anchor: ui.AnchorBottom, Dismissable: true}
	panel.Rows = append(panel.Rows, ui.Row{W: s.brightnessSlider(ctx, light)})
	if light.HasWarmth() {
		panel.Rows = append(panel.Rows, ui.Row{W: s.warmthSlider(ctx, light)})
	}
	panel.Rows = append(panel.Rows, ui.Row{W: &ui.Button{Text: "Готово", Style: ui.Primary, OnTap: func() { panel.Close() }}})
	ctx.ShowOverlay(panel)
}

// sleepOptions — варианты автосна в минутах; 0 — не засыпать.
var sleepOptions = []int{5, 10, 20, 30, 0}

// flashOptions — после скольких крупных изменений экран очищается полностью; 0 — никогда.
var flashOptions = []int{4, 8, 16, 0}

// wifiOffOptions — через сколько минут без дела гасить Wi-Fi; 0 — не гасить.
var wifiOffOptions = []int{2, 5, 15, 0}

func indexOf(xs []int, v int) int {
	for i, x := range xs {
		if x == v {
			return i
		}
	}
	return 0
}

// minuteLabels — подписи вариантов в минутах; 0 — zero.
func minuteLabels(xs []int, zero string) []string {
	out := make([]string, len(xs))
	for i, m := range xs {
		out[i] = fmt.Sprint(m)
		if m == 0 {
			out[i] = zero
		}
	}
	return out
}

// openSettings открывает настройки: разделы списком, в подписи — текущее значение.
func (s *Shell) openSettings(ctx *app.Context) {
	ctx.Push(&kit.ListScreen{
		Title: "Настройки",
		Items: func(ctx *app.Context) []ui.ListItem { return s.settingsItems(ctx) },
	})
}

func (s *Shell) settingsItems(ctx *app.Context) []ui.ListItem {
	next := icons.ChevronRight
	sleep := "не засыпать само"
	if m := s.settings.AutoSleep; m > 0 {
		sleep = "засыпать через " + ru.Count(m, "минуту", "минуты", "минут")
	}
	sleep += "; экран сна — " + s.sleepScreenName()
	n := 0
	var usage int64
	for _, a := range s.opts.Apps {
		n++
		usage += ctx.Sys().Storage(a.Manifest().ID).Usage()
	}
	info := ctx.Sys().Device().Info()
	return []ui.ListItem{
		{Title: "Wi-Fi", Subtitle: s.wifiSummary(ctx), Icon: wifiIcon(s.netStatus(ctx), s.settings.Wifi), Trailing: next,
			OnTap: func() { s.openWifi(ctx) }},
		{Title: "Экран и подсветка", Subtitle: s.screenSummary(ctx), Icon: icons.Sun, Trailing: next,
			OnTap: func() { ctx.Push(newForm("Экран и подсветка", s.buildScreen)) }},
		{Title: "Сон", Subtitle: sleep, Icon: icons.Moon, Trailing: next,
			OnTap: func() { ctx.Push(newForm("Сон", s.buildSleep)) }},
		{Title: "Приложения", Subtitle: fmt.Sprintf("%s, данные %s", ru.Count(n, "приложение", "приложения", "приложений"), ru.Size(usage)),
			Icon: icons.Apps, Trailing: next, OnTap: func() { s.openApps(ctx) }},
		{Title: "Питание", Subtitle: "выключить, перезагрузить, вернуться в Nickel", Icon: icons.Power, Trailing: next,
			OnTap: func() { ctx.Push(newForm("Питание", s.buildPower)) }},
		{Title: "Диагностика", Subtitle: "касания, кнопки, экран, сведения о книге", Icon: icons.Info, Trailing: next,
			OnTap: func() { ctx.Push(newDiag(s)) }},
		{Title: "О программе", Subtitle: fmt.Sprintf("el-book %s · %s · прошивка %s", orDash(s.opts.Version), info.Model, orDash(info.Firmware)),
			Icon: icons.Book},
	}
}

func (s *Shell) screenSummary(ctx *app.Context) string {
	text := "дневной режим"
	if ctx.Sys().Night() {
		text = "ночной режим"
	}
	if ctx.Rotation().Landscape() {
		text += ", альбом"
	} else {
		text += ", портрет"
	}
	if light := ctx.Sys().Device().Light(); light != nil {
		text += fmt.Sprintf(", подсветка %d %%", s.settings.Brightness)
	}
	return text
}

// buildScreen — подсветка, ночной режим, ориентация, очистка экрана, жесты.
func (s *Shell) buildScreen(ctx *app.Context, f *form) {
	if light := ctx.Sys().Device().Light(); light != nil {
		f.section("ПОДСВЕТКА")
		f.row(s.brightnessSlider(ctx, light))
		if light.HasWarmth() {
			f.row(s.warmthSlider(ctx, light))
		}
	}
	f.section("ЭКРАН")
	f.row(&ui.Toggle{Text: "Ночной режим: светлое на тёмном", On: ctx.Sys().Night(), OnChange: func(on bool) { s.setNight(ctx, on) }})
	f.caption("Ориентация")
	orient := 0
	if ctx.Rotation().Landscape() {
		orient = 1
	}
	f.row(&ui.Segmented{Options: []string{"Портрет", "Альбом"}, Selected: orient, OnChange: func(i int) {
		r := device.Portrait
		if i == 1 {
			r = device.LandscapeCW
		}
		ctx.Sys().SetRotation(r)
	}})
	f.caption("Полностью очищать после крупных изменений")
	f.row(&ui.Segmented{Options: minuteLabels(flashOptions, "никогда"), Selected: indexOf(flashOptions, s.settings.FlashEvery),
		OnChange: func(i int) {
			s.settings.FlashEvery = flashOptions[i]
			ctx.Sys().Policy().FlashEvery = flashOptions[i]
			s.save(ctx)
		}})
	f.section("ЖЕСТЫ")
	f.row(&ui.Toggle{Text: "Жесты от краёв экрана", On: s.settings.Edges, OnChange: func(on bool) {
		s.settings.Edges = on
		if s.edges != nil {
			s.edges.reset()
		}
		s.save(ctx)
	}})
	f.note("Свайп от верхнего края — быстрые настройки, от нижнего — домой. Работает в любом приложении.")
}

// sleepScreenName — как называется выбранный экран сна.
func (s *Shell) sleepScreenName() string {
	if a := s.appByID(s.settings.SleepScreen); a != nil {
		if _, ok := a.(app.SleepPainter); ok {
			return "«" + a.Manifest().Title + "»"
		}
	}
	return "сообщение"
}

// buildSleep — автосон и экран сна.
func (s *Shell) buildSleep(ctx *app.Context, f *form) {
	f.section("АВТОСОН")
	f.caption("Засыпать без действий через, мин")
	f.row(&ui.Segmented{Options: minuteLabels(sleepOptions, "нет"), Selected: indexOf(sleepOptions, s.settings.AutoSleep),
		OnChange: func(i int) {
			s.settings.AutoSleep = sleepOptions[i]
			s.save(ctx)
		}})
	f.section("ЭКРАН СНА")
	options := []string{"Сообщение"}
	ids := []string{""}
	for _, a := range s.opts.Apps {
		if _, ok := a.(app.SleepPainter); ok {
			options = append(options, a.Manifest().Title)
			ids = append(ids, a.Manifest().ID)
		}
	}
	selected := max(slices.Index(ids, s.settings.SleepScreen), 0)
	f.row(&ui.Segmented{Options: options, Selected: selected, OnChange: func(i int) {
		s.settings.SleepScreen = ids[i]
		s.save(ctx)
	}})
	f.note("Экран сна виден, пока книга спит: e-ink держит картинку без питания. " +
		"Приложение рисует его по своим данным — например, последний рисунок.")
}

// buildPower — выключение, перезагрузка, Nickel.
func (s *Shell) buildPower(ctx *app.Context, f *form) {
	f.section("ПИТАНИЕ")
	f.row(&ui.Button{Text: "Выключить", Icon: icons.Power, OnTap: func() {
		ctx.Confirm("Выключить книгу?", "", "Выключить", func() {
			s.save(ctx)
			ctx.Sys().Exit(app.ExitPowerOff, func(c *gfx.Canvas, t *ui.Theme) {
				paintMessage(c, t, icons.Power, "Книга выключена", "Чтобы включить, нажмите кнопку питания")
			})
		})
	}})
	f.row(&ui.Button{Text: "Перезагрузить", Icon: icons.Rotate, OnTap: func() {
		ctx.Confirm("Перезагрузить книгу?", "", "Перезагрузить", func() {
			s.save(ctx)
			ctx.Sys().Exit(app.ExitReboot, func(c *gfx.Canvas, t *ui.Theme) {
				paintMessage(c, t, icons.Rotate, "Перезагрузка", "")
			})
		})
	}})
	f.section("NICKEL")
	f.row(&ui.Button{Text: "Вернуться в Nickel", Icon: icons.Exit, OnTap: func() { s.confirmNickel(ctx) }})
	f.note("Nickel — штатная программа книги: магазин, библиотека, подключение к компьютеру как диск.")
	updates, waits := ctx.Sys().Stats()
	f.note(fmt.Sprintf("Обновлений панели за сеанс: %d, ожиданий: %d.", updates, waits))
}

// openApps — приложения: сколько занимают, закрепить, закрыть, очистить кэш.
func (s *Shell) openApps(ctx *app.Context) {
	var screen *kit.ListScreen
	screen = &kit.ListScreen{
		Title: "Приложения",
		Empty: "Приложений нет",
		Items: func(ctx *app.Context) []ui.ListItem {
			var items []ui.ListItem
			for _, a := range s.sortedApps() {
				a := a
				m := a.Manifest()
				var marks []string
				if s.pinned(m.ID) {
					marks = append(marks, "закреплено")
				}
				if s.running(ctx, m.ID) {
					marks = append(marks, "открыто")
				}
				sub := ru.Size(ctx.Sys().Storage(m.ID).Usage())
				for _, mk := range marks {
					sub += " · " + mk
				}
				items = append(items, ui.ListItem{Title: m.Title, Subtitle: sub, Icon: m.Icon, Trailing: icons.More,
					OnTap: func() { s.appMenu(ctx, a, func() { screen.Refresh(ctx) }) }})
			}
			return items
		},
	}
	ctx.Push(screen)
}

// appMenu — действия с приложением: открыть, закрепить, закрыть, очистить кэш.
func (s *Shell) appMenu(ctx *app.Context, a app.App, changed func()) {
	m := a.Manifest()
	pin := "Закрепить первым"
	if s.pinned(m.ID) {
		pin = "Открепить"
	}
	st := ctx.Sys().Storage(m.ID)
	ctx.Menu(m.Title,
		ui.MenuItem{Text: "Открыть", Icon: m.Icon, OnTap: func() { s.launch(ctx, a) }},
		ui.MenuItem{Text: pin, Icon: icons.Pin, OnTap: func() {
			s.togglePin(ctx, m.ID)
			changed()
		}},
		ui.MenuItem{Text: "Закрыть", Icon: icons.Close, Disabled: !s.running(ctx, m.ID), OnTap: func() {
			ctx.Sys().CloseApp(m.ID)
			ctx.Toast(fmt.Sprintf("«%s» закрыто", m.Title))
			changed()
		}},
		ui.MenuItem{Text: "Очистить кэш", Icon: icons.Trash, OnTap: func() {
			if err := st.ClearCache(); err != nil {
				ctx.Fail("Не удалось очистить кэш", err, "app", m.ID)
				return
			}
			ctx.Toast("Кэш очищен")
			changed()
		}},
	)
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
