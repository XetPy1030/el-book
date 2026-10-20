package shell

import (
	"context"
	"errors"
	"fmt"
	"image"
	"time"
	"unicode/utf8"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
	"el-book/sdk/web"
)

// netStatus — состояние Wi-Fi; у книги без Wi-Fi — «выключен».
func (s *Shell) netStatus(ctx *app.Context) device.NetStatus {
	if n := ctx.Sys().Network(); n != nil {
		return n.Status()
	}
	return device.NetStatus{}
}

// wifiSummary — одна строка о Wi-Fi для настроек и экрана Wi-Fi.
func (s *Shell) wifiSummary(ctx *app.Context) string {
	if ctx.Sys().Network() == nil {
		return "на этой книге недоступен"
	}
	if !s.settings.Wifi {
		return "выключен"
	}
	st := s.netStatus(ctx)
	switch st.State {
	case device.NetOnline:
		text := "подключён к «" + st.SSID + "»"
		if st.Signal > 0 {
			text += fmt.Sprintf(", сигнал %d %%", st.Signal)
		}
		return text
	case device.NetConnecting:
		if st.SSID != "" {
			return "подключается к «" + st.SSID + "»…"
		}
		return "подключается…"
	case device.NetFailed:
		return "не подключился: " + st.Err
	}
	return "включится, когда приложению понадобится сеть"
}

// openWifi открывает экран Wi-Fi.
func (s *Shell) openWifi(ctx *app.Context) {
	w := &wifiScreen{s: s}
	s.wifi = w
	ctx.Push(w)
}

// wifiScreen — Wi-Fi: включение, состояние, сети поблизости, подключение и автовыключение.
type wifiScreen struct {
	s        *Shell
	page     ui.Page
	toggle   *ui.Toggle
	status   *ui.Label
	list     *ui.List
	search   *ui.Button
	aps      []device.AccessPoint
	scanned  bool
	scanning bool
	scanErr  error
	busy     string // сеть, к которой идёт подключение
}

func (w *wifiScreen) Layout(ctx *app.Context, b image.Rectangle) {
	s := w.s
	t := ctx.Theme()
	w.page.Clear()
	r := b
	w.search = &ui.Button{Icon: icons.Refresh, Style: ui.Flat, OnTap: func() { w.scan(ctx) }}
	bar := ui.NewTitleBar(t, "Wi-Fi", func() { ctx.Pop() }, w.search)
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	w.page.Add(bar)
	r = ui.Pad(r, t.Pad(), t.MM(3), t.Pad(), t.MM(2))

	w.toggle = &ui.Toggle{Text: "Wi-Fi", On: s.settings.Wifi, OnChange: func(on bool) {
		s.setWifi(ctx, on)
		if on && !w.scanned {
			w.scan(ctx)
		}
		w.update(ctx)
	}}
	w.toggle.SetBounds(ui.CutTop(&r, t.Touch()))
	ui.CutTop(&r, t.MM(1))
	w.status = &ui.Label{Wrap: true, VAlign: -1, Color: gfx.Gray16(4)}
	w.status.SetBounds(ui.CutTop(&r, 2*t.Body().Height))
	ui.CutTop(&r, t.MM(2))

	f := &form{t: t, page: &w.page, r: r, y: r.Min.Y}
	f.caption("Выключать без дела через, мин")
	f.row(&ui.Segmented{Options: minuteLabels(wifiOffOptions, "нет"), Selected: indexOf(wifiOffOptions, s.settings.WifiAutoOff),
		OnChange: func(i int) {
			s.settings.WifiAutoOff = wifiOffOptions[i]
			s.save(ctx)
		}})
	sep := &ui.Separator{}
	sep.SetBounds(image.Rect(r.Min.X, f.y-t.MM(3), r.Max.X, f.y-t.MM(2)))
	w.list = ui.NewList(t)
	w.list.SetBounds(image.Rect(r.Min.X-t.MM(2), f.y, r.Max.X+t.MM(1), r.Max.Y))
	w.page.Add(w.toggle, w.status, sep, w.list)
	w.fill(ctx)
}

func (w *wifiScreen) Draw(c *gfx.Canvas, t *ui.Theme) { w.page.Draw(c, t) }

func (w *wifiScreen) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case app.Lifecycle:
		switch e {
		case app.Shown:
			w.s.wifi = w
			if w.s.settings.Wifi && !w.scanned && ctx.Sys().Network() != nil {
				w.scan(ctx)
			}
		case app.Hidden:
			if w.s.wifi == w {
				w.s.wifi = nil
			}
		}
	case device.KeyEvent:
		if kit.PageKeys(ctx, e, w.list.Next, w.list.Prev) {
			return
		}
	}
	app.RoutePage(ctx, &w.page, ev)
}

// update перерисовывает переключатель, состояние и список: сеть изменилась.
func (w *wifiScreen) update(ctx *app.Context) {
	if w.toggle == nil {
		return
	}
	w.toggle.On = w.s.settings.Wifi
	ctx.Invalidate(w.toggle.R, eink.UI)
	w.fill(ctx)
}

// fill пишет состояние и строки сетей.
func (w *wifiScreen) fill(ctx *app.Context) {
	s := w.s
	w.status.Text = s.wifiSummary(ctx)
	if st := s.netStatus(ctx); st.State == device.NetOnline && st.Addr != "" {
		w.status.Text += "\nАдрес в сети: " + st.Addr
	}
	ctx.Invalidate(w.status.R, eink.UI)
	w.search.Disabled = !s.settings.Wifi || ctx.Sys().Network() == nil || w.scanning
	ctx.Invalidate(w.search.R, eink.UI)

	switch {
	case ctx.Sys().Network() == nil:
		w.list.Empty = "Скрипт запуска не передал оболочке модуль Wi-Fi. Подробности — в журнале."
	case !s.settings.Wifi:
		w.list.Empty = "Включите Wi-Fi, чтобы искать сети поблизости."
	case w.scanning && len(w.aps) == 0:
		w.list.Empty = "Ищем сети…"
	case w.scanErr != nil:
		w.list.Empty = "Не удалось найти сети: " + web.Explain(w.scanErr)
	default:
		w.list.Empty = "Сетей поблизости не нашлось. Обновите поиск кнопкой вверху."
	}
	var items []ui.ListItem
	if s.settings.Wifi {
		cur := s.netStatus(ctx)
		for _, ap := range w.aps {
			ap := ap
			item := ui.ListItem{Title: ap.SSID, Icon: icons.Wifi(signalBars(ap.Signal)), Detail: fmt.Sprintf("%d %%", ap.Signal)}
			switch {
			case w.busy == ap.SSID:
				item.Subtitle = "подключается…"
			case cur.State == device.NetOnline && cur.SSID == ap.SSID:
				item.Subtitle = "подключено"
				item.Bold = true
				item.Trailing = icons.Check
			case ap.Saved:
				item.Subtitle = "сохранена"
			case ap.Secure:
				item.Subtitle = "нужен пароль"
			default:
				item.Subtitle = "открытая"
			}
			item.OnTap = func() { w.choose(ctx, ap) }
			item.OnLongPress = func() { w.menu(ctx, ap) }
			items = append(items, item)
		}
	}
	w.list.SetItems(items)
	w.list.Refresh(ctx)
}

// scan ищет сети в фоне.
func (w *wifiScreen) scan(ctx *app.Context) {
	n := ctx.Sys().Network()
	if n == nil || w.scanning || !w.s.settings.Wifi {
		return
	}
	w.scanning = true
	w.scanErr = nil
	w.s.netOnAt = ctx.Now()
	w.fill(ctx)
	kit.Load(ctx, func(bg context.Context) ([]device.AccessPoint, error) {
		bg, cancel := context.WithTimeout(bg, 20*time.Second)
		defer cancel()
		return n.Scan(bg)
	}, func(aps []device.AccessPoint, err error) {
		w.scanning = false
		w.scanned = true
		w.scanErr = err
		if err == nil || len(aps) > 0 {
			w.aps = aps
		}
		w.fill(ctx)
	})
}

// choose подключается к сети: сохранённой и открытой — сразу, иначе спрашивает пароль.
func (w *wifiScreen) choose(ctx *app.Context, ap device.AccessPoint) {
	if cur := w.s.netStatus(ctx); cur.State == device.NetOnline && cur.SSID == ap.SSID {
		ctx.Toast("Уже подключено")
		return
	}
	if ap.Saved || !ap.Secure {
		w.connect(ctx, ap.SSID, "")
		return
	}
	w.askPassword(ctx, ap.SSID, "")
}

// askPassword спрашивает пароль сети; problem — почему спрашиваем снова.
func (w *wifiScreen) askPassword(ctx *app.Context, ssid, problem string) {
	kit.Prompt(ctx, kit.PromptOptions{
		Title: "Пароль сети", Hint: "Сеть «" + ssid + "». Пароль не сохраняется: книга запомнит только ключ сети.",
		Secret: true, English: true, NoCaps: true, OK: "Подключить", Error: problem, Check: checkPassword,
	}, func(pass string) { w.connect(ctx, ssid, pass) })
}

// checkPassword — пароль WPA: от 8 до 63 символов.
func checkPassword(pass string) string {
	if n := utf8.RuneCountInString(pass); n < 8 || n > 63 {
		return "В пароле Wi-Fi от 8 до 63 символов, а здесь " + ru.Count(n, "символ", "символа", "символов") + "."
	}
	return ""
}

// connect подключается в фоне; неверный пароль — спросить снова.
func (w *wifiScreen) connect(ctx *app.Context, ssid, pass string) {
	n := ctx.Sys().Network()
	if n == nil {
		return
	}
	w.busy = ssid
	w.s.netOnAt = ctx.Now()
	w.fill(ctx)
	kit.Load(ctx, func(bg context.Context) (struct{}, error) {
		bg, cancel := context.WithTimeout(bg, time.Minute)
		defer cancel()
		return struct{}{}, n.Connect(bg, ssid, pass)
	}, func(_ struct{}, err error) {
		w.busy = ""
		switch {
		case errors.Is(err, device.ErrWrongPass):
			w.askPassword(ctx, ssid, "Пароль не подошёл. Проверьте раскладку и заглавные буквы.")
		case err != nil:
			ctx.Fail(web.Explain(err), err, "сеть", ssid)
		default:
			ctx.Toast("Подключено к «" + ssid + "»")
			if !w.s.settings.Wifi {
				w.s.setWifi(ctx, true)
			}
			for i := range w.aps {
				if w.aps[i].SSID == ssid {
					w.aps[i].Saved = true
				}
			}
		}
		w.fill(ctx)
	})
}

// menu — действия с сетью по долгому нажатию.
func (w *wifiScreen) menu(ctx *app.Context, ap device.AccessPoint) {
	n := ctx.Sys().Network()
	if n == nil {
		return
	}
	ctx.Menu(ap.SSID,
		ui.MenuItem{Text: "Подключиться", Icon: icons.Wifi(3), OnTap: func() { w.choose(ctx, ap) }},
		ui.MenuItem{Text: "Ввести пароль заново", Icon: icons.Keyboard, Disabled: !ap.Secure, OnTap: func() { w.askPassword(ctx, ap.SSID, "") }},
		ui.MenuItem{Text: "Забыть сеть", Icon: icons.Trash, Disabled: !ap.Saved, OnTap: func() {
			// Forget ждёт, пока закончится поиск или подключение, поэтому в фоне.
			kit.Load(ctx, func(context.Context) (struct{}, error) { return struct{}{}, n.Forget(ap.SSID) },
				func(_ struct{}, err error) {
					if err != nil {
						ctx.Fail(err.Error(), err)
						return
					}
					for i := range w.aps {
						if w.aps[i].SSID == ap.SSID {
							w.aps[i].Saved = false
						}
					}
					ctx.Toast("Сеть забыта")
					w.fill(ctx)
				})
		}},
	)
}
