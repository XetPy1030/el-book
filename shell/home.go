package shell

import (
	"fmt"
	"image"
	"slices"
	"sort"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
)

// maxSummaries — сколько сводок приложений помещается справа от часов.
const maxSummaries = 3

// Home — домашний экран: строка состояния, крупные часы с датой, строки приложений
// (погода, задачи, таймер), сетка приложений и нижняя панель.
type Home struct {
	s         *Shell
	page      ui.Page
	bounds    image.Rectangle
	status    *tapArea
	clock     *ui.Custom
	rows      []*summaryRow
	grid      *ui.Grid
	pageLabel *ui.Label
	wifiBtn   *ui.Button
	tick      func()
	battery   int
	charging  bool
	net       device.NetStatus
	wifiOn    bool
	now       time.Time
	summaries []summary
}

// summary — строка приложения на домашнем экране.
type summary struct {
	app  app.App
	line app.Summary
}

func newHome(s *Shell) *Home { return &Home{s: s} }

func (h *Home) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	h.bounds = b
	h.page.Clear()
	h.summaries = h.s.summaries(ctx)
	pad := t.Pad()
	r := b

	h.status = &tapArea{OnTap: func() { h.s.openQuick(ctx) }}
	h.status.Paint = h.paintStatus
	h.status.SetBounds(ui.CutTop(&r, t.MM(8)))

	clockH := t.MM(34)
	if b.Dx() > b.Dy() {
		clockH = t.MM(24)
	}
	h.clock = &ui.Custom{Paint: h.paintClock}
	h.clock.SetBounds(ui.Pad(ui.CutTop(&r, clockH), pad, 0, pad, 0))

	// Сводки — справа от цифр часов. Место под них есть всегда, поэтому плитки не
	// сдвигаются, когда сводка появляется или пропадает.
	h.rows = h.rows[:0]
	band := summaryBand(t, h.clock.R)
	for i := range maxSummaries {
		row := &summaryRow{h: h, i: i}
		row.OnTap = func() {
			if i < len(h.summaries) {
				h.s.launch(ctx, h.summaries[i].app)
			}
		}
		y := band.Min.Y + band.Dy()*i/maxSummaries
		row.SetBounds(image.Rect(band.Min.X, y, band.Max.X, band.Min.Y+band.Dy()*(i+1)/maxSummaries))
		h.rows = append(h.rows, row)
		h.page.Add(row)
	}

	dockH := t.MM(17)
	dockR := ui.Pad(ui.CutBottom(&r, dockH+t.MM(2)), pad, 0, pad, t.MM(2))
	h.wifiBtn = &ui.Button{Text: "Wi-Fi", Style: ui.Flat, Vertical: true, OnTap: func() { h.s.openWifi(ctx) }}
	dock := &ui.HStack{Gap: t.Gap(), Items: []ui.Widget{
		&ui.Button{Text: "Подсветка", Icon: icons.Sun, Style: ui.Flat, Vertical: true, OnTap: func() { h.s.showLight(ctx) }},
		h.wifiBtn,
		&ui.Button{Text: "Сон", Icon: icons.Moon, Style: ui.Flat, Vertical: true, OnTap: func() { h.s.Sleep(ctx) }},
		&ui.Button{Text: "Настройки", Icon: icons.Settings, Style: ui.Flat, Vertical: true, OnTap: func() { h.s.openSettings(ctx) }},
	}}
	dock.SetBounds(dockR)
	sep := &ui.Separator{}
	sep.SetBounds(ui.Pad(ui.CutBottom(&r, t.MM(3)), pad, t.MM(1), pad, t.MM(1)))
	top := &ui.Separator{}
	top.SetBounds(ui.Pad(ui.CutTop(&r, t.MM(2)), pad, 0, pad, t.MM(1)))

	h.pageLabel = &ui.Label{Align: gfx.AlignCenter, Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }, Color: gfx.Gray16(5)}
	h.pageLabel.SetBounds(ui.CutBottom(&r, t.Caption().Height+t.MM(1)))

	cols := 3
	if b.Dx() > b.Dy() {
		cols = 5
	}
	area := ui.Pad(r, pad, t.MM(1), pad, 0)
	cellW := area.Dx() / cols
	// Рядов столько, сколько помещается плиток не ниже 30 мм; высота делится поровну.
	rows := max(area.Dy()/min(cellW, t.MM(30)), 1)
	cellH := area.Dy() / rows
	page := 0
	if h.grid != nil {
		page = h.grid.Page()
	}
	h.grid = &ui.Grid{Cols: cols, Rows: rows, OnPage: func(p, n int) { h.updatePageLabel(ctx) }}
	for _, a := range h.s.sortedApps() {
		a := a
		m := a.Manifest()
		h.grid.Items = append(h.grid.Items, &ui.Tile{Title: m.Title, Icon: m.Icon,
			OnTap:       func() { h.s.launch(ctx, a) },
			OnLongPress: func() { h.s.appMenu(ctx, a, func() { h.relayout(ctx) }) },
		})
	}
	h.grid.SetBounds(image.Rect(area.Min.X, area.Min.Y, area.Min.X+cols*cellW, area.Min.Y+rows*cellH))
	h.grid.SetPage(ctx, page)
	h.updatePageLabel(ctx)

	h.page.Add(h.status, h.clock, top, h.grid, h.pageLabel, sep, dock)
	if len(h.grid.Items) == 0 {
		empty := &ui.Label{Text: "Приложений пока нет. Они появятся здесь, когда их добавят в сборку.", Wrap: true, Align: gfx.AlignCenter, Color: gfx.Gray16(5)}
		empty.SetBounds(h.grid.R)
		h.page.Add(empty)
	}
	h.styleWifi(ctx)
	h.refreshStatus(ctx)
}

// relayout раскладывает экран заново: порядок приложений или число строк изменились.
func (h *Home) relayout(ctx *app.Context) {
	if h.bounds.Empty() {
		return
	}
	h.Layout(ctx, h.bounds)
	ctx.Invalidate(h.bounds, eink.UI)
}

func (h *Home) updatePageLabel(ctx *app.Context) {
	h.pageLabel.Hidden = h.grid.Pages() < 2
	h.pageLabel.Text = h.grid.PageText()
	ctx.Invalidate(h.pageLabel.R, eink.UI)
}

func (h *Home) Draw(c *gfx.Canvas, t *ui.Theme) { h.page.Draw(c, t) }

func (h *Home) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case app.Lifecycle:
		switch e {
		case app.Shown, app.Resumed:
			h.refreshSummaries(ctx)
			h.refreshStatus(ctx)
			h.scheduleTick(ctx)
		case app.Hidden, app.Paused:
			if h.tick != nil {
				h.tick()
				h.tick = nil
			}
		}
	case device.KeyEvent:
		if e.Pressed && e.Key == device.KeyPageForward {
			h.grid.SetPage(ctx, h.grid.Page()+1)
		} else if e.Pressed && e.Key == device.KeyPageBack {
			h.grid.SetPage(ctx, h.grid.Page()-1)
		}
	default:
		app.RoutePage(ctx, &h.page, ev)
	}
}

// scheduleTick обновляет часы и строки приложений в начале каждой минуты: перерисовывается
// только то, что изменилось.
func (h *Home) scheduleTick(ctx *app.Context) {
	if h.tick != nil {
		h.tick()
	}
	now := ctx.Now()
	next := now.Truncate(time.Minute).Add(time.Minute).Sub(now) + 50*time.Millisecond
	h.tick = ctx.After(next, func() {
		h.refreshStatus(ctx)
		h.refreshSummaries(ctx)
		h.scheduleTick(ctx)
	})
}

// refreshStatus перечитывает время, батарею и Wi-Fi и перерисовывает то, что изменилось.
func (h *Home) refreshStatus(ctx *app.Context) {
	now := ctx.Now()
	bat := ctx.Sys().Device().Battery()
	level, err := bat.Level()
	if err != nil {
		level = -1
	}
	charging, _ := bat.Charging()
	net := h.s.netStatus(ctx)
	if level != h.battery || charging != h.charging || net != h.net || h.wifiOn != h.s.settings.Wifi {
		h.battery, h.charging, h.net, h.wifiOn = level, charging, net, h.s.settings.Wifi
		if h.status != nil {
			ctx.Invalidate(h.status.R, eink.UI)
		}
		h.styleWifi(ctx)
	}
	if now.Truncate(time.Minute) != h.now.Truncate(time.Minute) {
		h.now = now
		if h.clock != nil {
			ctx.Invalidate(h.clock.R, eink.UI)
		}
	}
}

// styleWifi показывает в кнопке панели, включён ли Wi-Fi.
func (h *Home) styleWifi(ctx *app.Context) {
	if h.wifiBtn == nil {
		return
	}
	icon := wifiIcon(h.s.netStatus(ctx), h.s.settings.Wifi)
	h.wifiBtn.Icon = icon
	h.wifiBtn.Disabled = ctx.Sys().Network() == nil
	ctx.Invalidate(h.wifiBtn.R, eink.UI)
}

// refreshSummaries перечитывает сводки приложений и перерисовывает изменившиеся.
func (h *Home) refreshSummaries(ctx *app.Context) {
	fresh := h.s.summaries(ctx)
	old := h.summaries
	h.summaries = fresh
	for i, row := range h.rows {
		was, is := i < len(old), i < len(fresh)
		if was != is || is && (fresh[i].line.Text != old[i].line.Text || fresh[i].app != old[i].app) {
			ctx.Invalidate(row.R, eink.UI)
		}
	}
}

func (h *Home) paintStatus(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	pad := t.Pad()
	face := t.Caption()
	x := r.Min.X + pad
	iconH := t.MM(3.6)
	if h.wifiOn && h.net.State != device.NetOff {
		icon := wifiIcon(h.net, true)
		icon(c, image.Rect(x, r.Min.Y+(r.Dy()-iconH)/2, x+iconH, r.Min.Y+(r.Dy()+iconH)/2), gfx.Black)
		x += iconH + t.MM(1.5)
		text := ""
		switch h.net.State {
		case device.NetOnline:
			text = h.net.SSID
		case device.NetConnecting:
			text = "подключение…"
		case device.NetFailed:
			text = "нет сети"
		}
		face.DrawIn(c, text, image.Rect(x, r.Min.Y, r.Min.X+r.Dx()/2, r.Max.Y), gfx.AlignLeft, gfx.Gray16(3))
	} else {
		face.DrawIn(c, "el-book", image.Rect(x, r.Min.Y, r.Min.X+r.Dx()/2, r.Max.Y), gfx.AlignLeft, gfx.Gray16(4))
	}
	if h.battery >= 0 {
		text := fmt.Sprintf("%d%%", h.battery)
		tw := face.Measure(text)
		iconH := t.MM(3.2)
		iconW := iconH * 2
		x := r.Max.X - pad - tw
		face.DrawIn(c, text, image.Rect(x, r.Min.Y, r.Max.X-pad, r.Max.Y), gfx.AlignRight, gfx.Black)
		x -= t.MM(1.5) + iconW
		icons.Battery(h.battery, h.charging)(c, image.Rect(x, r.Min.Y+(r.Dy()-iconH)/2, x+iconW, r.Min.Y+(r.Dy()+iconH)/2), gfx.Black)
	}
	// Ручка под строкой: отсюда тянут быстрые настройки.
	hw := t.MM(10)
	c.FillRoundRect(image.Rect(r.Min.X+(r.Dx()-hw)/2, r.Max.Y-t.MM(1.2), r.Min.X+(r.Dx()+hw)/2, r.Max.Y-t.MM(0.6)), t.MM(0.3), gfx.Gray16(9))
}

// clockFace — шрифт цифр часов в области r.
func clockFace(t *ui.Theme, r image.Rectangle) *gfx.Face {
	return t.Fonts.Face(gfx.Regular, min(r.Dy()*55/100, t.PT(64)))
}

// summaryBand — место сводок: справа от цифр часов, от верха области часов до даты.
func summaryBand(t *ui.Theme, clock image.Rectangle) image.Rectangle {
	big := clockFace(t, clock)
	x := clock.Min.X + big.Measure("00:00") + t.MM(6)
	return image.Rect(x, clock.Min.Y, clock.Max.X, clock.Min.Y+t.MM(3)+big.Height)
}

func (h *Home) paintClock(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	now := h.now
	big := clockFace(t, r)
	y := r.Min.Y + t.MM(3)
	big.DrawIn(c, now.Format("15:04"), image.Rect(r.Min.X, y, r.Max.X, y+big.Height), gfx.AlignLeft, gfx.Black)
	date := t.Face(gfx.Medium, 14)
	y += big.Height + t.MM(1)
	date.DrawIn(c, russianDate(now), image.Rect(r.Min.X, y, r.Max.X, y+date.Height), gfx.AlignLeft, gfx.Gray16(3))
}

// summaries — строки приложений, у которых сейчас есть что сказать: недавние первыми.
func (s *Shell) summaries(ctx *app.Context) []summary {
	var out []summary
	now := ctx.Now()
	for _, a := range s.sortedApps() {
		sm, ok := a.(app.Summarizer)
		if !ok {
			continue
		}
		line, ok := sm.Summary(ctx.Sys().Storage(a.Manifest().ID), now)
		if !ok || line.Text == "" {
			continue
		}
		out = append(out, summary{app: a, line: line})
		if len(out) == maxSummaries {
			break
		}
	}
	return out
}

// sortedApps — приложения: закреплённые первыми в порядке закрепления, остальные — в
// порядке сборки (самые нужные первыми). От запусков порядок не зависит: плитка остаётся
// там, где палец её помнит.
func (s *Shell) sortedApps() []app.App {
	apps := append([]app.App(nil), s.opts.Apps...)
	pin := func(a app.App) int {
		if i := slices.Index(s.settings.Pinned, a.Manifest().ID); i >= 0 {
			return i
		}
		return len(s.settings.Pinned)
	}
	sort.SliceStable(apps, func(i, j int) bool { return pin(apps[i]) < pin(apps[j]) })
	return apps
}

// tapArea — область, которая рисует себя функцией и откликается на тап.
type tapArea struct {
	ui.Custom
	OnTap func()
}

func (a *tapArea) Enabled() bool                { return a.OnTap != nil }
func (a *tapArea) Tap(_ ui.Host, _ image.Point) { a.OnTap() }

// summaryRow — строка приложения под часами: иконка и текст; тап открывает приложение.
type summaryRow struct {
	ui.Box
	h     *Home
	i     int
	OnTap func()
}

func (r *summaryRow) Enabled() bool                { return r.i < len(r.h.summaries) }
func (r *summaryRow) Tap(_ ui.Host, _ image.Point) { r.OnTap() }

func (r *summaryRow) Draw(c *gfx.Canvas, t *ui.Theme) {
	if r.i >= len(r.h.summaries) {
		return
	}
	sm := r.h.summaries[r.i]
	icon := sm.line.Icon
	if icon == nil {
		icon = sm.app.Manifest().Icon
	}
	b := r.Bounds()
	size := t.MM(5)
	icon(c, image.Rect(b.Min.X, b.Min.Y+(b.Dy()-size)/2, b.Min.X+size, b.Min.Y+(b.Dy()+size)/2), gfx.Black)
	// Текст — в две строки, если помещаются; не влезшее уходит в многоточие. Интерлиньяж
	// плотнее обычного: две строки одной сводки читаются как одна запись.
	face := t.Caption()
	lineH := face.Ascent + face.Descent
	x := b.Min.X + size + t.MM(2)
	lines := face.Wrap(sm.line.Text, b.Max.X-x)
	n := min(len(lines), max(b.Dy()/lineH, 1), 2)
	if len(lines) > n {
		lines[n-1] = strings.Join(lines[n-1:], " ")
		lines = lines[:n]
	}
	y := b.Min.Y + (b.Dy()-n*lineH)/2
	for _, l := range lines {
		face.DrawIn(c, l, image.Rect(x, y, b.Max.X, y+lineH), gfx.AlignLeft, gfx.Black)
		y += lineH
	}
}

// russianDate — «понедельник, 19 октября».
func russianDate(t time.Time) string { return ru.Date(t) }

func rect(x0, y0, x1, y1 int) image.Rectangle { return image.Rect(x0, y0, x1, y1) }

func rectAround(cx, top, size int) image.Rectangle {
	return image.Rect(cx-size/2, top, cx+size/2, top+size)
}
