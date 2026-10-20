// Package weather — погода по данным Open-Meteo (без ключа): сейчас, по часам и на неделю,
// несколько городов с поиском, строка на домашнем экране. Последний прогноз хранится в кэше
// и показывается сразу, свежий — когда придёт; без сети приложение честно пишет, от какого
// времени данные.
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"math"
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
	"el-book/sdk/web"
)

// App — приложение «Погода».
type App struct {
	prefs *kit.Prefs[prefs]
}

// New создаёт приложение.
func New() *App { return &App{} }

const prefsVersion = 1

type prefs struct {
	Cities  []City `json:"cities"`
	Current int    `json:"current"`
}

func defaultPrefs() prefs {
	return prefs{Cities: []City{{Name: "Москва", Region: "Россия", Lat: 55.7558, Lon: 37.6173}}}
}

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "weather", Title: "Погода", Icon: icons.CloudSun, Landscape: true}
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.prefs = kit.LoadPrefs(ctx.Storage(), prefsVersion, defaultPrefs())
	if len(a.prefs.Value.Cities) == 0 {
		a.prefs.Value = defaultPrefs()
	}
	return &mainScreen{a: a}
}

func (a *App) city() City {
	p := &a.prefs.Value
	p.Current = min(max(p.Current, 0), len(p.Cities)-1)
	return p.Cities[p.Current]
}

func (a *App) save(ctx *app.Context) {
	if err := a.prefs.Save(); err != nil {
		ctx.Log().Error("настройки погоды", "err", err)
	}
}

// summaryFile — строка для домашнего экрана: её пишет приложение, читает оболочка.
const summaryFile = "summary.json"

type summary struct {
	Text string `json:"text"`
	Code int    `json:"code"`
	Day  bool   `json:"day"`
	At   int64  `json:"at"`
}

// Summary — погода на домашнем экране: «+8°, пасмурно · Москва». Прогноз старше полусуток
// не показывается.
func (a *App) Summary(st *storage.App, now time.Time) (app.Summary, bool) {
	data, err := st.ReadFile(summaryFile)
	if err != nil {
		return app.Summary{}, false
	}
	var s summary
	if json.Unmarshal(data, &s) != nil || now.Sub(time.Unix(s.At, 0)) > 12*time.Hour {
		return app.Summary{}, false
	}
	_, icon := condition(s.Code, s.Day)
	return app.Summary{Icon: icon, Text: s.Text}, true
}

// mainScreen — прогноз выбранного города.
type mainScreen struct {
	a       *App
	page    ui.Page
	bar     *ui.TitleBar
	body    *ui.Custom
	retry   *ui.Button
	data    *forecast
	fetched time.Time
	stale   error // обновить не удалось: показываются сохранённые данные
	err     error // данных нет совсем
	loading bool
	now     time.Time // время последней перерисовки: по нему выбираются часы прогноза
}

func (s *mainScreen) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	s.page.Clear()
	r := b
	cities := &ui.Button{Icon: icons.Menu, Style: ui.Flat, OnTap: func() { s.openCities(ctx) }}
	refresh := &ui.Button{Icon: icons.Refresh, Style: ui.Flat, OnTap: func() { s.load(ctx, true) }}
	s.bar = ui.NewTitleBar(t, s.a.city().Name, func() { ctx.Home() }, cities, refresh)
	s.bar.BackIcon = icons.Home
	s.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	s.retry = &ui.Button{Text: "Обновить", Icon: icons.Refresh, OnTap: func() { s.load(ctx, true) }}
	s.retry.SetBounds(ui.Center(r, t.MM(50), t.Touch()).Add(image.Pt(0, t.MM(20))))
	s.body = &ui.Custom{Paint: s.paint}
	s.body.SetBounds(ui.Pad(r, t.Pad(), t.MM(3), t.Pad(), t.Pad()))
	s.page.Add(s.bar, s.body, s.retry)
	if s.data == nil && !s.loading {
		s.load(ctx, false)
	}
	s.update(ctx)
}

func (s *mainScreen) Draw(c *gfx.Canvas, t *ui.Theme) { s.page.Draw(c, t) }

func (s *mainScreen) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case app.Lifecycle:
		if (e == app.Resumed || e == app.Shown) && s.data != nil && ctx.Now().Sub(s.fetched) > 30*time.Minute {
			s.load(ctx, false)
		}
	case device.NetworkEvent:
		// Сеть появилась, а показаны старые данные — обновляемся сами.
		if e.Status.State == device.NetOnline && s.stale != nil && !s.loading {
			s.load(ctx, false)
		}
	}
	app.RoutePage(ctx, &s.page, ev)
}

// load показывает сохранённый прогноз сразу и загружает свежий. force — не верить кэшу.
func (s *mainScreen) load(ctx *app.Context, force bool) {
	city := s.a.city()
	url := forecastURL(city)
	client := ctx.Web()
	if s.data == nil {
		if res, ok := client.Cached(url); ok {
			var f forecast
			if json.Unmarshal(res.Body, &f) == nil {
				s.data, s.fetched = &f, res.Fetched
			}
		}
	}
	maxAge := 20 * time.Minute
	if force {
		maxAge = 0
	}
	s.loading = true
	s.update(ctx)
	kit.Load(ctx, func(bg context.Context) (web.Result, error) {
		var f forecast
		res, err := client.FetchJSON(bg, url, maxAge, &f)
		return res, err
	}, func(res web.Result, err error) {
		s.loading = false
		if city != s.a.city() {
			return // пока грузили, выбрали другой город
		}
		var f forecast
		switch {
		case err != nil && s.data == nil:
			s.err = err
		case err != nil:
			s.stale = err
		case json.Unmarshal(res.Body, &f) != nil:
			s.err = fmt.Errorf("сервер прислал непонятный ответ")
		default:
			s.data, s.fetched, s.err = &f, res.Fetched, nil
			s.stale = nil
			if res.Stale {
				s.stale = res.Err
			}
			s.saveSummary(ctx, city)
		}
		if err != nil {
			ctx.Log().Info("погода не обновилась", "err", err)
		}
		s.update(ctx)
	})
}

// saveSummary записывает строку для домашнего экрана.
func (s *mainScreen) saveSummary(ctx *app.Context, city City) {
	c := s.data.Current
	desc, _ := condition(c.Code, c.IsDay == 1)
	text := fmt.Sprintf("%s, %s · %s", temp(c.Temp), lower(desc), city.Name)
	data, _ := json.Marshal(summary{Text: text, Code: c.Code, Day: c.IsDay == 1, At: s.fetched.Unix()})
	if err := ctx.Storage().WriteFile(summaryFile, data); err != nil {
		ctx.Log().Warn("строка погоды", "err", err)
	}
}

func lower(s string) string {
	r := []rune(s)
	if len(r) > 0 && r[0] >= 'А' && r[0] <= 'Я' {
		r[0] += 'а' - 'А'
	}
	return string(r)
}

// update перерисовывает заголовок и прогноз.
func (s *mainScreen) update(ctx *app.Context) {
	if s.bar == nil {
		return
	}
	now := ctx.Now()
	s.now = now
	sub := ""
	switch {
	case s.loading && s.data == nil:
		sub = "загружаем…"
	case s.loading:
		sub = "обновляем…"
	case s.stale != nil && s.data != nil:
		sub = "не обновилось · данные " + ru.Ago(s.fetched, now)
	case s.data != nil:
		sub = "обновлено " + ru.Ago(s.fetched, now)
	}
	s.bar.SetTitle(ctx, s.a.city().Name, sub)
	s.retry.Hidden = s.data != nil || s.loading
	ctx.Invalidate(s.body.R, eink.Content)
	ctx.Invalidate(s.retry.R, eink.UI)
}

func (s *mainScreen) paint(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	if s.data == nil {
		msg := "Загружаем прогноз…"
		if s.err != nil && !s.loading {
			msg = web.Explain(s.err)
		}
		lbl := ui.Label{Text: msg, Wrap: true, Align: gfx.AlignCenter, Color: gfx.Gray16(4)}
		lbl.SetBounds(image.Rect(r.Min.X, r.Min.Y+r.Dy()/3, r.Max.X, r.Min.Y+r.Dy()/3+t.MM(20)))
		lbl.Draw(c, t)
		return
	}
	f := s.data
	now := s.now
	y := r.Min.Y
	// Сейчас: значок, температура, описание и подробности.
	cur := f.Current
	desc, icon := condition(cur.Code, cur.IsDay == 1)
	iconSize := t.MM(24)
	icon(c, image.Rect(r.Min.X, y, r.Min.X+iconSize, y+iconSize), gfx.Black)
	big := t.Face(gfx.Regular, 54)
	x := r.Min.X + iconSize + t.MM(5)
	big.DrawIn(c, temp(cur.Temp), image.Rect(x, y-t.MM(2), r.Max.X, y+big.Height), gfx.AlignLeft, gfx.Black)
	y += iconSize + t.MM(3)
	t.Header().DrawIn(c, desc, image.Rect(r.Min.X, y, r.Max.X, y+t.Header().Height), gfx.AlignLeft, gfx.Black)
	y += t.Header().Height + t.MM(1)
	details := fmt.Sprintf("Ощущается как %s · ветер %s м/с, %s · влажность %d %% · %d мм рт. ст.",
		temp(cur.Feels), ru.Decimal(cur.Wind, 0), windDir(cur.WindDir), int(math.Round(cur.Humidity)),
		int(math.Round(cur.Pressure*0.750062)))
	body := t.Body()
	for _, line := range body.Wrap(details, r.Dx()) {
		body.DrawIn(c, line, image.Rect(r.Min.X, y, r.Max.X, y+body.Height), gfx.AlignLeft, gfx.Gray16(3))
		y += body.Height
	}
	y += t.MM(4)
	c.Fill(image.Rect(r.Min.X, y, r.Max.X, y+t.Line()), gfx.Gray16(10))
	y += t.MM(3)

	// По часам: шесть столбцов через два часа.
	hours := f.nextHours(now, 6, 2)
	if len(hours) > 0 {
		colW := r.Dx() / len(hours)
		small := t.Caption()
		for i, h := range hours {
			cx := r.Min.X + i*colW
			col := image.Rect(cx, y, cx+colW, y+small.Height)
			small.DrawIn(c, h.at.Format("15:04"), col, gfx.AlignCenter, gfx.Gray16(4))
			_, hi := condition(h.code, h.at.Hour() >= 7 && h.at.Hour() < 20)
			is := t.MM(8)
			hi(c, image.Rect(cx+(colW-is)/2, y+small.Height+t.MM(1), cx+(colW+is)/2, y+small.Height+t.MM(1)+is), gfx.Black)
			ty := y + small.Height + is + t.MM(2)
			t.Strong().DrawIn(c, temp(h.temp), image.Rect(cx, ty, cx+colW, ty+t.Strong().Height), gfx.AlignCenter, gfx.Black)
			if h.chance >= 10 {
				py := ty + t.Strong().Height
				small.DrawIn(c, fmt.Sprintf("%d %%", int(h.chance)), image.Rect(cx, py, cx+colW, py+small.Height), gfx.AlignCenter, gfx.Gray16(5))
			}
		}
		y += small.Height*2 + t.MM(9) + t.Strong().Height + t.MM(2)
		c.Fill(image.Rect(r.Min.X, y, r.Max.X, y+t.Line()), gfx.Gray16(10))
		y += t.MM(2)
	}

	// По дням.
	rowH := t.MM(10)
	for _, d := range f.days() {
		if y+rowH > r.Max.Y {
			break
		}
		name := dayName(d.at, now)
		body.DrawIn(c, name, image.Rect(r.Min.X, y, r.Min.X+r.Dx()*2/5, y+rowH), gfx.AlignLeft, gfx.Black)
		_, di := condition(d.code, true)
		is := t.MM(7)
		ix := r.Min.X + r.Dx()*2/5
		di(c, image.Rect(ix, y+(rowH-is)/2, ix+is, y+(rowH+is)/2), gfx.Black)
		rain := ""
		if d.precip >= 0.5 {
			rain = ru.Decimal(d.precip, 1) + " мм"
		} else if d.chance >= 30 {
			rain = fmt.Sprintf("%d %%", int(d.chance))
		}
		t.Caption().DrawIn(c, rain, image.Rect(ix+is+t.MM(2), y, ix+is+t.MM(20), y+rowH), gfx.AlignLeft, gfx.Gray16(5))
		t.Strong().DrawIn(c, temp(d.lo)+" … "+temp(d.hi), image.Rect(r.Min.X, y, r.Max.X, y+rowH), gfx.AlignRight, gfx.Black)
		y += rowH
	}
	if days := f.days(); len(days) > 0 && days[0].sunrise != "" && y+t.Caption().Height < r.Max.Y {
		y += t.MM(2)
		t.Caption().DrawIn(c, "Восход "+days[0].sunrise+" · закат "+days[0].sunset+" · данные Open-Meteo",
			image.Rect(r.Min.X, y, r.Max.X, y+t.Caption().Height), gfx.AlignLeft, gfx.Gray16(5))
	}
}

// openCities — города: выбрать, добавить, удалить.
func (s *mainScreen) openCities(ctx *app.Context) {
	var list *kit.ListScreen
	add := &ui.Button{Text: "Добавить город", Icon: icons.Plus, Style: ui.Primary, OnTap: func() { s.addCity(ctx, list) }}
	list = &kit.ListScreen{
		Title:  "Города",
		Bottom: add,
		Items: func(ctx *app.Context) []ui.ListItem {
			p := &s.a.prefs.Value
			var items []ui.ListItem
			for i, c := range p.Cities {
				i, c := i, c
				item := ui.ListItem{Title: c.Name, Subtitle: c.Region, Icon: icons.Globe}
				if i == p.Current {
					item.Trailing = icons.Check
					item.Bold = true
				}
				item.OnTap = func() {
					s.selectCity(ctx, i)
					ctx.Pop()
				}
				item.OnLongPress = func() {
					if len(p.Cities) < 2 {
						ctx.Toast("Последний город удалить нельзя")
						return
					}
					ctx.Confirm("Удалить «"+c.Name+"»?", "", "Удалить", func() {
						p.Cities = append(p.Cities[:i], p.Cities[i+1:]...)
						if p.Current >= len(p.Cities) || p.Current == i {
							p.Current = 0
						}
						s.a.save(ctx)
						s.data = nil
						list.Refresh(ctx)
					})
				}
				items = append(items, item)
			}
			return items
		},
	}
	ctx.Push(list)
}

func (s *mainScreen) selectCity(ctx *app.Context, i int) {
	s.a.prefs.Value.Current = i
	s.a.save(ctx)
	s.data, s.err, s.stale = nil, nil, nil
	s.load(ctx, false)
}

// addCity ищет город по названию и добавляет выбранный.
func (s *mainScreen) addCity(ctx *app.Context, list *kit.ListScreen) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Город", Hint: "Название города: «Казань», «Санкт-Петербург».", OK: "Найти"},
		func(name string) {
			ctx.Toast("Ищем «" + name + "»…")
			kit.Load(ctx, func(bg context.Context) (geoResult, error) {
				var g geoResult
				err := ctx.Web().GetJSON(bg, geoURL(name), &g)
				return g, err
			}, func(g geoResult, err error) {
				if err != nil {
					ctx.Fail(web.Explain(err), err)
					return
				}
				if len(g.Results) == 0 {
					ctx.Toast("Город «" + name + "» не нашёлся")
					return
				}
				var opts []string
				var found []City
				for _, r := range g.Results {
					region := r.Country
					if r.Admin1 != "" && r.Admin1 != r.Name {
						region = r.Admin1 + ", " + r.Country
					}
					opts = append(opts, r.Name+" — "+region)
					found = append(found, City{Name: r.Name, Region: region, Lat: r.Lat, Lon: r.Lon})
				}
				ctx.ShowOverlay(ui.NewChoice(ctx.Theme(), "Какой город?", opts, -1, func(i int) {
					p := &s.a.prefs.Value
					p.Cities = append(p.Cities, found[i])
					s.selectCity(ctx, len(p.Cities)-1)
					list.Refresh(ctx)
				}))
			})
		})
}
