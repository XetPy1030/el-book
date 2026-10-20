// Package calendar — календарь: месяц сеткой с понедельника, праздники России, события
// по дням со временем, листание месяцев свайпом и кнопками, строка на домашнем экране.
package calendar

import (
	"encoding/json"
	"fmt"
	"image"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// App — приложение «Календарь».
type App struct {
	month  time.Time // первое число показанного месяца
	page   ui.Page
	bar    *ui.TitleBar
	grid   *monthGrid
	events []Event
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "calendar", Title: "Календарь", Icon: icons.Calendar, Landscape: true}
}

// Event — событие дня. Time — «14:30»; пусто — на весь день.
type Event struct {
	ID   string `json:"id"`
	Date string `json:"date"` // 2026-10-19
	Time string `json:"time,omitempty"`
	Text string `json:"text"`
}

const eventsFile = "events.json"

func loadEvents(st *storage.App) []Event {
	data, err := st.ReadFile(eventsFile)
	if err != nil {
		return nil
	}
	var out []Event
	_ = json.Unmarshal(data, &out)
	return out
}

func (a *App) saveEvents(ctx *app.Context) {
	data, _ := json.Marshal(a.events)
	if err := ctx.Storage().WriteFile(eventsFile, data); err != nil {
		ctx.Fail("Не удалось сохранить события", err)
	}
}

func day(t time.Time) string { return t.Format("2006-01-02") }

// holidays — праздничные дни России, одинаковые каждый год.
var holidays = map[string]string{
	"01-01": "Новый год", "01-02": "Новогодние каникулы", "01-03": "Новогодние каникулы",
	"01-04": "Новогодние каникулы", "01-05": "Новогодние каникулы", "01-06": "Новогодние каникулы",
	"01-07": "Рождество Христово", "01-08": "Новогодние каникулы",
	"02-23": "День защитника Отечества", "03-08": "Международный женский день",
	"05-01": "Праздник Весны и Труда", "05-09": "День Победы",
	"06-12": "День России", "11-04": "День народного единства",
}

func holiday(t time.Time) string { return holidays[t.Format("01-02")] }

// eventsOn — события дня по времени: на весь день первыми.
func eventsOn(events []Event, date string) []Event {
	var out []Event
	for _, e := range events {
		if e.Date == date {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.events = loadEvents(ctx.Storage())
	now := ctx.Now()
	a.month = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return a
}

func (a *App) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	a.page.Clear()
	r := b
	a.bar = ui.NewTitleBar(t, "", func() { ctx.Home() },
		&ui.Button{Icon: icons.ChevronLeft, Style: ui.Flat, OnTap: func() { a.shift(ctx, -1) }},
		&ui.Button{Text: "Сегодня", Style: ui.Flat, OnTap: func() { a.today(ctx) }},
		&ui.Button{Icon: icons.ChevronRight, Style: ui.Flat, OnTap: func() { a.shift(ctx, 1) }},
	)
	a.bar.BackIcon = icons.Home
	a.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	a.grid = &monthGrid{a: a, theme: t, today: ctx.Now(), onDay: func(d time.Time) { a.openDay(ctx, d) }, onSwipe: func(n int) { a.shift(ctx, n) }}
	a.grid.SetBounds(ui.Pad(r, t.Pad()-t.MM(1), t.MM(2), t.Pad()-t.MM(1), t.Pad()))
	a.page.Add(a.bar, a.grid)
	a.updateTitle(ctx)
}

func (a *App) updateTitle(ctx *app.Context) {
	title := capitalize(ru.Month(a.month.Month())) + " " + strconv.Itoa(a.month.Year())
	a.bar.SetTitle(ctx, title, "")
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
	}
	return string(r)
}

func (a *App) shift(ctx *app.Context, n int) {
	a.month = a.month.AddDate(0, n, 0)
	a.updateTitle(ctx)
	ctx.Invalidate(a.grid.R, eink.Content)
}

func (a *App) today(ctx *app.Context) {
	now := ctx.Now()
	a.month = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	a.updateTitle(ctx)
	ctx.Invalidate(a.grid.R, eink.Content)
}

func (a *App) Draw(c *gfx.Canvas, t *ui.Theme) { a.page.Draw(c, t) }

func (a *App) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case device.KeyEvent:
		if kit.PageKeys(ctx, e, func(ui.Host) bool { a.shift(ctx, 1); return true }, func(ui.Host) bool { a.shift(ctx, -1); return true }) {
			return
		}
	case app.Lifecycle:
		if e == app.Shown || e == app.Resumed {
			a.grid.today = ctx.Now()
			ctx.Invalidate(a.grid.R, eink.UI)
		}
	}
	app.RoutePage(ctx, &a.page, ev)
}

// monthGrid — дни месяца сеткой 7 × 6 с понедельника.
type monthGrid struct {
	ui.Box
	a       *App
	theme   *ui.Theme
	today   time.Time
	onDay   func(time.Time)
	onSwipe func(int)
}

func (g *monthGrid) first() time.Time {
	m := g.a.month
	offset := (int(m.Weekday()) + 6) % 7 // понедельник — 0
	return m.AddDate(0, 0, -offset)
}

func (g *monthGrid) cells(t *ui.Theme) (head int, cellW, cellH int) {
	head = t.Caption().Height + t.MM(2)
	return head, g.R.Dx() / 7, (g.R.Dy() - head) / 6
}

func (g *monthGrid) Draw(c *gfx.Canvas, t *ui.Theme) {
	head, cw, ch := g.cells(t)
	for i, name := range []string{"пн", "вт", "ср", "чт", "пт", "сб", "вс"} {
		color := gfx.Gray16(5)
		if i >= 5 {
			color = gfx.Black
		}
		t.Caption().DrawIn(c, name, image.Rect(g.R.Min.X+i*cw, g.R.Min.Y, g.R.Min.X+(i+1)*cw, g.R.Min.Y+head), gfx.AlignCenter, color)
	}
	today := g.today
	d := g.first()
	num := t.Face(gfx.Regular, 16)
	bold := t.Face(gfx.Bold, 16)
	small := t.Caption()
	for row := 0; row < 6; row++ {
		for col := 0; col < 7; col++ {
			cell := image.Rect(g.R.Min.X+col*cw, g.R.Min.Y+head+row*ch, g.R.Min.X+(col+1)*cw, g.R.Min.Y+head+(row+1)*ch)
			inMonth := d.Month() == g.a.month.Month()
			face := num
			color := uint8(gfx.Black)
			if col >= 5 || holiday(d) != "" {
				face = bold
			}
			if !inMonth {
				color = gfx.Gray16(9)
			}
			box := cell.Inset(t.MM(0.8))
			if day(d) == day(today) {
				c.FillRoundRect(box, t.MM(1.5), gfx.Black)
				color = gfx.White
			}
			face.DrawIn(c, strconv.Itoa(d.Day()), image.Rect(box.Min.X, box.Min.Y+t.MM(1), box.Max.X, box.Min.Y+t.MM(1)+face.Height), gfx.AlignCenter, color)
			if inMonth {
				events := eventsOn(g.a.events, day(d))
				label := ""
				switch {
				case len(events) > 0 && events[0].Time != "":
					label = events[0].Time
				case len(events) > 0:
					label = "•"
				}
				if len(events) > 1 {
					label += fmt.Sprintf(" +%d", len(events)-1)
				}
				if label != "" {
					y := box.Max.Y - small.Height - t.MM(1)
					small.DrawIn(c, label, image.Rect(box.Min.X, y, box.Max.X, y+small.Height), gfx.AlignCenter, color)
				}
			}
			d = d.AddDate(0, 0, 1)
		}
	}
	// Тонкие линии между неделями.
	for row := 1; row < 6; row++ {
		y := g.R.Min.Y + head + row*ch
		c.Fill(image.Rect(g.R.Min.X, y, g.R.Max.X, y+1), gfx.Gray16(12))
	}
}

func (g *monthGrid) Enabled() bool    { return true }
func (g *monthGrid) ShowsPress() bool { return false }

func (g *monthGrid) Tap(_ ui.Host, p image.Point) {
	head, cw, ch := g.cells(g.theme)
	if p.Y < g.R.Min.Y+head {
		return
	}
	col := (p.X - g.R.Min.X) / cw
	row := (p.Y - g.R.Min.Y - head) / ch
	if col < 0 || col > 6 || row < 0 || row > 5 {
		return
	}
	g.onDay(g.first().AddDate(0, 0, row*7+col))
}

func (g *monthGrid) Swipe(_ ui.Host, dir ui.Direction) bool {
	switch dir {
	case ui.DirLeft, ui.DirUp:
		g.onSwipe(1)
	case ui.DirRight, ui.DirDown:
		g.onSwipe(-1)
	default:
		return false
	}
	return true
}

var timeRe = regexp.MustCompile(`^([01]?\d|2[0-3])[:.]([0-5]\d)\s+(.+)$`)

// parseEvent отделяет время в начале текста: «14:30 Встреча» — время 14:30.
func parseEvent(text string) (at, rest string) {
	text = strings.TrimSpace(text)
	if m := timeRe.FindStringSubmatch(text); m != nil {
		h, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("%02d:%s", h, m[2]), m[3]
	}
	return "", text
}

// openDay — события дня: добавить, удалить.
func (a *App) openDay(ctx *app.Context, d time.Time) {
	date := day(d)
	var screen *kit.ListScreen
	add := &ui.Button{Text: "Добавить событие", Icon: icons.Plus, Style: ui.Primary, OnTap: func() {
		kit.Prompt(ctx, kit.PromptOptions{Title: "Событие", Hint: "Время в начале — по желанию: «14:30 Встреча».", OK: "Добавить"},
			func(text string) {
				at, rest := parseEvent(text)
				a.events = append(a.events, Event{ID: strconv.FormatInt(ctx.Now().UnixNano(), 36), Date: date, Time: at, Text: rest})
				a.saveEvents(ctx)
				screen.Refresh(ctx)
			})
	}}
	screen = &kit.ListScreen{
		Title:  capitalize(ru.Date(d)),
		Bottom: add,
		Empty:  "Событий нет.",
		Items: func(ctx *app.Context) []ui.ListItem {
			var items []ui.ListItem
			if h := holiday(d); h != "" {
				items = append(items, ui.ListItem{Title: h, Subtitle: "праздник", Icon: icons.Star(true), Muted: true})
			}
			for _, e := range eventsOn(a.events, date) {
				e := e
				detail := e.Time
				if detail == "" {
					detail = "весь день"
				}
				items = append(items, ui.ListItem{Title: e.Text, Detail: detail, Icon: icons.Clock, OnLongPress: func() {
					ctx.Confirm("Удалить событие?", e.Text, "Удалить", func() {
						kept := a.events[:0]
						for _, x := range a.events {
							if x.ID != e.ID {
								kept = append(kept, x)
							}
						}
						a.events = kept
						a.saveEvents(ctx)
						screen.Refresh(ctx)
					})
				}})
			}
			return items
		},
	}
	ctx.Push(screen)
}

// Summary — «Сегодня: 14:30 Встреча и ещё 1» или праздник.
func (a *App) Summary(st *storage.App, now time.Time) (app.Summary, bool) {
	events := eventsOn(loadEvents(st), day(now))
	var upcoming []Event
	for _, e := range events {
		if e.Time == "" || e.Time >= now.Format("15:04") {
			upcoming = append(upcoming, e)
		}
	}
	switch {
	case len(upcoming) > 0:
		e := upcoming[0]
		text := "Сегодня: " + strings.TrimSpace(e.Time+" "+e.Text)
		if len(upcoming) > 1 {
			text += " и ещё " + strconv.Itoa(len(upcoming)-1)
		}
		return app.Summary{Text: text}, true
	case holiday(now) != "":
		return app.Summary{Icon: icons.Star(true), Text: holiday(now)}, true
	}
	return app.Summary{}, false
}
