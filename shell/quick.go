package shell

import (
	"fmt"
	"image"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
)

// holdButton — кнопка с действием по долгому нажатию: тап переключает, долгое нажатие
// открывает подробные настройки.
type holdButton struct {
	ui.Button
	OnLongPress func()
}

func (b *holdButton) LongPress(h ui.Host, _ image.Point) {
	if b.OnLongPress != nil && b.Enabled() {
		b.OnLongPress()
	}
}

// quickPanel — быстрые настройки: свайп от верхнего края или тап по строке состояния.
// Подсветка, Wi-Fi, ночной режим, поворот, очистка экрана, сон и открытые приложения —
// всё, что нужно, не выходя из приложения.
type quickPanel struct {
	s     *Shell
	panel *ui.Panel
	wifi  *holdButton
}

// openQuick показывает быстрые настройки; повторный жест не открывает вторую панель.
func (s *Shell) openQuick(ctx *app.Context) {
	if s.quick != nil && ctx.HasOverlay(s.quick.panel) {
		return
	}
	q := &quickPanel{s: s}
	q.build(ctx)
	s.quick = q
	ctx.ShowOverlay(q.panel)
}

func (s *Shell) closeQuick(ctx *app.Context) {
	if s.quick != nil {
		ctx.CloseOverlay(s.quick.panel)
		s.quick = nil
	}
}

// quickTitle — строка состояния в заголовке шторки: время, дата и заряд. Шторка закрывает
// строку состояния домашнего экрана, а в приложении её нет вовсе.
func quickTitle(ctx *app.Context, now time.Time) string {
	title := now.Format("15:04") + " · " + ru.WeekdayShort(now.Weekday()) + ", " + ru.DayMonth(now, now)
	bat := ctx.Sys().Device().Battery()
	if level, err := bat.Level(); err == nil {
		title += fmt.Sprintf(" · %d %%", level)
		if charging, _ := bat.Charging(); charging {
			title += ", заряжается"
		}
	}
	return title
}

func (q *quickPanel) build(ctx *app.Context) {
	s := q.s
	t := ctx.Theme()
	now := ctx.Now()
	q.panel = &ui.Panel{Title: quickTitle(ctx, now), Anchor: ui.AnchorTop, Dismissable: true,
		OnClose: func() { s.quick = nil }}
	panel := q.panel
	if light := ctx.Sys().Device().Light(); light != nil {
		panel.Rows = append(panel.Rows, ui.Row{W: s.brightnessSlider(ctx, light)})
		if light.HasWarmth() {
			panel.Rows = append(panel.Rows, ui.Row{W: s.warmthSlider(ctx, light)})
		}
	}

	tileH := t.MM(17)
	q.wifi = &holdButton{Button: ui.Button{Vertical: true, Style: ui.Flat}}
	q.wifi.OnTap = func() {
		s.setWifi(ctx, !s.settings.Wifi)
		q.update(ctx)
	}
	q.wifi.OnLongPress = func() {
		s.closeQuick(ctx)
		s.openWifi(ctx)
	}
	q.styleWifi(ctx)
	night := &ui.Button{Text: "Ночь", Icon: icons.Moon, Vertical: true, Style: ui.Flat, Active: ctx.Sys().Night()}
	night.OnTap = func() {
		s.setNight(ctx, !ctx.Sys().Night())
		night.Active = ctx.Sys().Night()
		ctx.Invalidate(night.R, eink.UI)
	}
	toggles := &ui.HStack{Gap: t.Gap(), Items: []ui.Widget{
		q.wifi,
		night,
		&ui.Button{Text: "Поворот", Icon: icons.Rotate, Vertical: true, Style: ui.Flat, OnTap: func() {
			s.closeQuick(ctx)
			r := device.LandscapeCW
			if ctx.Rotation().Landscape() {
				r = device.Portrait
			}
			ctx.Sys().SetRotation(r)
		}},
		&ui.Button{Text: "Очистить", Icon: icons.Refresh, Vertical: true, Style: ui.Flat, OnTap: func() {
			s.closeQuick(ctx)
			ctx.Sys().Refresh()
		}},
	}}
	actions := &ui.HStack{Gap: t.Gap(), Items: []ui.Widget{
		&ui.Button{Text: "Сон", Icon: icons.Power, Vertical: true, Style: ui.Flat, OnTap: func() {
			s.closeQuick(ctx)
			s.Sleep(ctx)
		}},
		&ui.Button{Text: "Настройки", Icon: icons.Settings, Vertical: true, Style: ui.Flat, OnTap: func() {
			s.closeQuick(ctx)
			s.openSettings(ctx)
		}},
		&ui.Button{Text: "Nickel", Icon: icons.Exit, Vertical: true, Style: ui.Flat, OnTap: func() {
			s.closeQuick(ctx)
			s.confirmNickel(ctx)
		}},
		&ui.Button{Text: "Домой", Icon: icons.Home, Vertical: true, Style: ui.Flat, OnTap: func() {
			s.closeQuick(ctx)
			ctx.Home()
		}},
	}}
	panel.Rows = append(panel.Rows, ui.Row{W: toggles, H: tileH}, ui.Row{W: actions, H: tileH})

	recent := ctx.Sys().Recent()
	caption := "Открытых приложений нет"
	if len(recent) > 0 {
		caption = "Открытые приложения — долгое нажатие закрывает"
	}
	panel.Rows = append(panel.Rows, ui.Row{W: &ui.Label{Text: caption, Color: gfx.Gray16(5),
		Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}, H: t.Caption().Height + t.MM(1)})
	if len(recent) > 0 {
		row := &ui.HStack{Gap: t.Gap()}
		for _, m := range recent[:min(len(recent), 4)] {
			m := m
			b := &holdButton{Button: ui.Button{Text: m.Title, Icon: m.Icon, Vertical: true, Style: ui.Secondary}}
			b.OnTap = func() {
				if a := s.appByID(m.ID); a != nil {
					s.launch(ctx, a)
				}
			}
			b.OnLongPress = func() {
				ctx.Sys().CloseApp(m.ID)
				ctx.Toast(fmt.Sprintf("«%s» закрыто", m.Title))
				s.closeQuick(ctx)
				s.openQuick(ctx)
			}
			row.Items = append(row.Items, b)
		}
		// Пустые места держат ширину кнопок одинаковой при любом числе приложений.
		for len(row.Items) < 4 {
			row.Items = append(row.Items, &ui.Custom{})
		}
		panel.Rows = append(panel.Rows, ui.Row{W: row, H: tileH})
	}
}

// styleWifi показывает в плитке Wi-Fi состояние сети.
func (q *quickPanel) styleWifi(ctx *app.Context) {
	s := q.s
	n := ctx.Sys().Network()
	st := device.NetStatus{}
	if n != nil {
		st = n.Status()
	}
	q.wifi.Active = s.settings.Wifi
	q.wifi.Icon = wifiIcon(st, s.settings.Wifi)
	q.wifi.Disabled = n == nil
	switch {
	case !s.settings.Wifi || n == nil:
		q.wifi.Text = "Wi-Fi"
	case st.State == device.NetOnline:
		q.wifi.Text = st.SSID
	case st.State == device.NetConnecting:
		q.wifi.Text = "Подключение…"
	default:
		q.wifi.Text = "Wi-Fi вкл."
	}
}

// update перерисовывает то, что зависит от сети.
func (q *quickPanel) update(ctx *app.Context) {
	q.styleWifi(ctx)
	ctx.Invalidate(q.wifi.R, eink.UI)
}

// setNight включает или выключает ночной режим и запоминает выбор.
func (s *Shell) setNight(ctx *app.Context, on bool) {
	ctx.Sys().SetNight(on)
	s.settings.Night = on
	s.save(ctx)
}

// wifiIcon — значок Wi-Fi по состоянию: перечёркнут — выключен, деления — сигнал.
func wifiIcon(st device.NetStatus, allowed bool) icons.Drawer {
	switch {
	case !allowed:
		return icons.Wifi(-1)
	case st.State == device.NetOnline:
		return icons.Wifi(signalBars(st.Signal))
	}
	return icons.Wifi(0)
}

// signalBars переводит сигнал 0…100 в деления значка 1…3; неизвестный сигнал — полный значок.
func signalBars(signal int) int {
	switch {
	case signal <= 0 || signal > 66:
		return 3
	case signal > 33:
		return 2
	}
	return 1
}

func (s *Shell) brightnessSlider(ctx *app.Context, light device.Light) *ui.Slider {
	return &ui.Slider{Min: 0, Max: 100, Value: s.settings.Brightness, LeftIcon: icons.Sun,
		OnChange: func(v int) {
			s.settings.Brightness = v
			if err := light.SetBrightness(v); err != nil {
				ctx.Log().Error("яркость", "err", err)
			}
		},
		OnCommit: func(int) { s.save(ctx) },
	}
}

func (s *Shell) warmthSlider(ctx *app.Context, light device.Light) *ui.Slider {
	return &ui.Slider{Min: 0, Max: 100, Value: s.settings.Warmth, LeftIcon: icons.Warmth,
		OnChange: func(v int) {
			s.settings.Warmth = v
			if err := light.SetWarmth(v); err != nil {
				ctx.Log().Error("теплота", "err", err)
			}
		},
		OnCommit: func(int) { s.save(ctx) },
	}
}
