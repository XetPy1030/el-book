// Package news — новости из лент RSS и Atom: свои ленты, все новости вместе или по одной
// ленте, непрочитанные жирным, статья целиком прямо на книге (текст страницы без меню
// и рекламы), строка на домашнем экране с числом непрочитанного.
package news

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
	"el-book/sdk/web"
)

// App — приложение «Новости».
type App struct {
	prefs   *kit.Prefs[prefs]
	items   []Item
	updated time.Time
	errs    map[string]error // ленты, которые не обновились
	loading bool
	filter  string // URL ленты; пусто — все
	main    *kit.ListScreen
}

// New создаёт приложение.
func New() *App { return &App{} }

const prefsVersion = 1

type prefs struct {
	Sources []Source         `json:"sources"`
	Read    map[string]int64 `json:"read"` // id новости → когда прочитана
}

func defaultSources() []Source {
	return []Source{
		{Title: "Хабр", URL: "https://habr.com/ru/rss/articles/?fl=ru"},
		{Title: "N + 1", URL: "https://nplus1.ru/rss"},
		{Title: "Лента.ру", URL: "https://lenta.ru/rss/news"},
	}
}

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "news", Title: "Новости", Icon: icons.News, Landscape: true}
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.prefs = kit.LoadPrefs(ctx.Storage(), prefsVersion, prefs{Sources: defaultSources()})
	if a.prefs.Value.Read == nil {
		a.prefs.Value.Read = map[string]int64{}
	}
	a.main = &kit.ListScreen{
		Title: "Новости",
		Root:  true,
		Empty: "Новостей пока нет. Обновите ленты кнопкой вверху.",
		Actions: []*ui.Button{
			{Icon: icons.Rss, Style: ui.Flat, OnTap: func() { a.openSources(ctx) }},
			{Icon: icons.Refresh, Style: ui.Flat, OnTap: func() { a.refresh(ctx, true) }},
		},
		Items: func(ctx *app.Context) []ui.ListItem { return a.listItems(ctx) },
	}
	a.loadCached(ctx)
	a.refresh(ctx, false)
	return &mainScreen{ListScreen: a.main, a: a}
}

// mainScreen — список новостей; сам обновляется, когда появляется сеть.
type mainScreen struct {
	*kit.ListScreen
	a *App
}

func (m *mainScreen) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case device.NetworkEvent:
		if e.Status.State == device.NetOnline && len(m.a.errs) > 0 && !m.a.loading {
			m.a.refresh(ctx, false)
		}
	case app.Lifecycle:
		if e == app.Resumed && ctx.Now().Sub(m.a.updated) > time.Hour {
			m.a.refresh(ctx, false)
		}
	}
	m.ListScreen.Handle(ctx, ev)
}

func (a *App) save(ctx *app.Context) {
	if err := a.prefs.Save(); err != nil {
		ctx.Log().Error("настройки новостей", "err", err)
	}
}

func (a *App) sources() []Source {
	var out []Source
	for _, s := range a.prefs.Value.Sources {
		if a.filter == "" || s.URL == a.filter {
			out = append(out, s)
		}
	}
	return out
}

// loadCached показывает сохранённые ленты сразу, без сети.
func (a *App) loadCached(ctx *app.Context) {
	client := ctx.Web()
	var lists [][]Item
	for _, s := range a.prefs.Value.Sources {
		res, ok := client.Cached(s.URL)
		if !ok {
			continue
		}
		if _, items, err := parseFeed(res.Body, s); err == nil {
			lists = append(lists, items)
			if res.Fetched.After(a.updated) {
				a.updated = res.Fetched
			}
		}
	}
	a.items = merge(lists...)
}

// refresh загружает ленты параллельно. force — не верить кэшу.
func (a *App) refresh(ctx *app.Context, force bool) {
	if a.loading {
		return
	}
	a.loading = true
	a.updateTitle(ctx)
	srcs := append([]Source(nil), a.prefs.Value.Sources...)
	client := ctx.Web()
	maxAge := 15 * time.Minute
	if force {
		maxAge = 0
	}
	type result struct {
		items []Item
		errs  map[string]error
		at    time.Time
	}
	kit.Load(ctx, func(bg context.Context) (result, error) {
		// Ленты по очереди: Wi-Fi книги всё равно один, а параллельные запросы мешали бы
		// друг другу и правилу «фоновая работа — через ctx.Go».
		lists := make([][]Item, len(srcs))
		res := result{errs: map[string]error{}}
		for i, s := range srcs {
			r, err := client.Fetch(bg, s.URL, maxAge)
			var items []Item
			if err == nil {
				_, items, err = parseFeed(r.Body, s)
			}
			if err == nil && r.Stale {
				err = r.Err
			}
			if err != nil {
				res.errs[s.URL] = err
			}
			if r.Fetched.After(res.at) {
				res.at = r.Fetched
			}
			lists[i] = items
		}
		res.items = merge(lists...)
		if len(res.items) == 0 && len(res.errs) > 0 {
			for _, err := range res.errs {
				return res, err
			}
		}
		return res, nil
	}, func(res result, err error) {
		a.loading = false
		a.errs = res.errs
		if err != nil {
			ctx.Toast(web.Explain(err))
		} else {
			a.items, a.updated = res.items, res.at
			a.saveSummary(ctx)
			if n := len(res.errs); n > 0 {
				ctx.Log().Info("ленты не обновились", "n", n)
			}
		}
		a.main.Refresh(ctx)
		a.updateTitle(ctx)
	})
}

func (a *App) updateTitle(ctx *app.Context) {
	bar := a.main.Bar()
	if bar == nil {
		return
	}
	title := "Новости"
	for _, s := range a.prefs.Value.Sources {
		if s.URL == a.filter {
			title = s.Title
		}
	}
	sub := ""
	switch {
	case a.loading:
		sub = "обновляем…"
	case len(a.errs) > 0 && !a.updated.IsZero():
		sub = "не все ленты обновились · " + ru.Ago(a.updated, ctx.Now())
	case !a.updated.IsZero():
		sub = "обновлено " + ru.Ago(a.updated, ctx.Now())
	}
	if n := a.unread(); n > 0 {
		sub = strings.TrimPrefix(sub+" · "+ru.Count(n, "непрочитанная", "непрочитанные", "непрочитанных"), " · ")
	}
	bar.SetTitle(ctx, title, sub)
}

func (a *App) unread() int {
	n := 0
	for _, it := range a.items {
		if _, ok := a.prefs.Value.Read[it.ID]; !ok && a.visible(it) {
			n++
		}
	}
	return n
}

func (a *App) visible(it Item) bool {
	if a.filter == "" {
		return true
	}
	for _, s := range a.prefs.Value.Sources {
		if s.URL == a.filter {
			return it.Source == s.Title
		}
	}
	return true
}

func (a *App) listItems(ctx *app.Context) []ui.ListItem {
	now := ctx.Now()
	var out []ui.ListItem
	for _, it := range a.items {
		if !a.visible(it) {
			continue
		}
		it := it
		_, read := a.prefs.Value.Read[it.ID]
		sub := it.Source
		if !it.Date.IsZero() {
			sub += " · " + ru.Ago(it.Date, now)
		}
		out = append(out, ui.ListItem{Title: it.Title, Subtitle: sub, Bold: !read, Muted: read,
			OnTap: func() { a.openItem(ctx, it) }})
	}
	return out
}

// openItem показывает новость: заголовок, дату и описание; полный текст — по кнопке.
func (a *App) openItem(ctx *app.Context, it Item) {
	a.markRead(ctx, it)
	blocks := []ui.Block{{Kind: ui.Heading, Text: it.Title}}
	meta := it.Source
	if !it.Date.IsZero() {
		meta += " · " + ru.Short(it.Date, ctx.Now())
	}
	blocks = append(blocks, ui.Block{Kind: ui.Meta, Text: meta})
	blocks = append(blocks, kit.Paragraphs(it.Summary)...)
	var screen *kit.ReaderScreen
	full := &ui.Button{Text: "Читать полностью", Icon: icons.Book, Style: ui.Primary}
	full.Disabled = it.Link == ""
	full.OnTap = func() {
		full.Disabled = true
		full.Text = "Загружаем…"
		ctx.Invalidate(full.R, eink.UI)
		kit.Load(ctx, func(bg context.Context) (web.Result, error) {
			return ctx.Web().Fetch(bg, it.Link, 24*time.Hour)
		}, func(res web.Result, err error) {
			full.Text = "Читать полностью"
			if err != nil {
				full.Disabled = false
				ctx.Invalidate(full.R, eink.UI)
				ctx.Fail(web.Explain(err), err)
				return
			}
			page := web.ExtractText(res.Body)
			if len(page.Paras) == 0 {
				full.Disabled = false
				ctx.Invalidate(full.R, eink.UI)
				ctx.Toast("Текст статьи не нашёлся на странице")
				return
			}
			full.Text = "Полный текст"
			ctx.Invalidate(full.R, eink.UI)
			screen.SetBlocks(ctx, articleBlocks(it, page, meta))
		})
	}
	screen = &kit.ReaderScreen{Title: it.Source, Blocks: blocks, Bottom: full}
	ctx.Push(screen)
}

// articleBlocks — полный текст статьи в абзацы читалки.
func articleBlocks(it Item, page web.Page, meta string) []ui.Block {
	title := it.Title
	if page.Title != "" {
		title = page.Title
	}
	out := []ui.Block{{Kind: ui.Heading, Text: title}, {Kind: ui.Meta, Text: meta}}
	for _, p := range page.Paras {
		b := ui.Block{Kind: ui.Para, Text: p.Text}
		switch {
		case p.Heading == 1:
			b.Kind = ui.Heading
		case p.Heading > 1:
			b.Kind = ui.Subheading
		case p.Item:
			b.Kind = ui.Item
		case p.Quote:
			b.Kind = ui.Quote
		}
		out = append(out, b)
	}
	return out
}

func (a *App) markRead(ctx *app.Context, it Item) {
	read := a.prefs.Value.Read
	if _, ok := read[it.ID]; ok {
		return
	}
	read[it.ID] = ctx.Now().Unix()
	// Отметки старше месяца не нужны: таких новостей в лентах уже нет.
	if len(read) > 3000 {
		cut := ctx.Now().AddDate(0, -1, 0).Unix()
		for id, at := range read {
			if at < cut {
				delete(read, id)
			}
		}
	}
	a.save(ctx)
	a.saveSummary(ctx)
	a.updateTitle(ctx)
}

// openSources — ленты: показать одну, все, добавить, удалить.
func (a *App) openSources(ctx *app.Context) {
	var screen *kit.ListScreen
	add := &ui.Button{Text: "Добавить ленту", Icon: icons.Plus, Style: ui.Primary, OnTap: func() { a.addSource(ctx, screen) }}
	screen = &kit.ListScreen{
		Title:  "Ленты",
		Bottom: add,
		Items: func(ctx *app.Context) []ui.ListItem {
			items := []ui.ListItem{{Title: "Все новости", Icon: icons.News, OnTap: func() { a.setFilter(ctx, "") }}}
			if a.filter == "" {
				items[0].Trailing = icons.Check
			}
			for i, s := range a.prefs.Value.Sources {
				i, s := i, s
				item := ui.ListItem{Title: s.Title, Subtitle: s.URL, Icon: icons.Rss,
					OnTap: func() { a.setFilter(ctx, s.URL) },
					OnLongPress: func() {
						ctx.Confirm("Удалить ленту «"+s.Title+"»?", "", "Удалить", func() {
							src := &a.prefs.Value.Sources
							*src = append((*src)[:i], (*src)[i+1:]...)
							if a.filter == s.URL {
								a.filter = ""
							}
							a.save(ctx)
							screen.Refresh(ctx)
						})
					}}
				if err := a.errs[s.URL]; err != nil {
					item.Subtitle = "не обновилась: " + web.Explain(err)
				}
				if a.filter == s.URL {
					item.Trailing = icons.Check
				}
				items = append(items, item)
			}
			return items
		},
	}
	ctx.Push(screen)
}

func (a *App) setFilter(ctx *app.Context, url string) {
	a.filter = url
	ctx.Pop()
	a.main.Refresh(ctx)
	a.main.List().SetPage(ctx, 0)
	a.updateTitle(ctx)
}

// addSource проверяет ленту по адресу и добавляет её под её же названием.
func (a *App) addSource(ctx *app.Context, screen *kit.ListScreen) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Адрес ленты", Hint: "Адрес RSS или Atom, например https://nplus1.ru/rss",
		Placeholder: "https://", English: true, NoCaps: true, OK: "Добавить"}, func(url string) {
		url = strings.TrimSpace(url)
		if !strings.Contains(url, "://") {
			url = "https://" + url
		}
		for _, s := range a.prefs.Value.Sources {
			if s.URL == url {
				ctx.Toast("Эта лента уже есть")
				return
			}
		}
		ctx.Toast("Проверяем ленту…")
		kit.Load(ctx, func(bg context.Context) (string, error) {
			res, err := ctx.Web().Fetch(bg, url, 0)
			if err != nil {
				return "", err
			}
			title, items, err := parseFeed(res.Body, Source{URL: url})
			if err != nil {
				return "", err
			}
			if len(items) == 0 {
				return "", errors.New("в ленте нет новостей")
			}
			return title, nil
		}, func(title string, err error) {
			if err != nil {
				msg := web.Explain(err)
				if strings.Contains(err.Error(), "не лента") || strings.Contains(err.Error(), "не разбирается") {
					msg = "По этому адресу нет ленты RSS или Atom"
				}
				ctx.Fail(msg, err)
				return
			}
			if title == "" {
				title = url
			}
			a.prefs.Value.Sources = append(a.prefs.Value.Sources, Source{Title: title, URL: url})
			a.save(ctx)
			screen.Refresh(ctx)
			a.refresh(ctx, false)
			ctx.Toast(fmt.Sprintf("Лента «%s» добавлена", title))
		})
	})
}

// summaryFile — строка домашнего экрана: число непрочитанных и свежая новость.
const summaryFile = "summary.json"

type summary struct {
	Unread int    `json:"unread"`
	Top    string `json:"top"`
	At     int64  `json:"at"`
}

func (a *App) saveSummary(ctx *app.Context) {
	s := summary{At: ctx.Now().Unix()}
	for _, it := range a.items {
		if _, ok := a.prefs.Value.Read[it.ID]; !ok {
			s.Unread++
			if s.Top == "" {
				s.Top = it.Title
			}
		}
	}
	data, _ := json.Marshal(s)
	if err := ctx.Storage().WriteFile(summaryFile, data); err != nil {
		ctx.Log().Warn("строка новостей", "err", err)
	}
}

// Summary — «12 непрочитанных: заголовок свежей новости».
func (a *App) Summary(st *storage.App, now time.Time) (app.Summary, bool) {
	data, err := st.ReadFile(summaryFile)
	if err != nil {
		return app.Summary{}, false
	}
	var s summary
	if json.Unmarshal(data, &s) != nil || s.Unread == 0 || now.Sub(time.Unix(s.At, 0)) > 24*time.Hour {
		return app.Summary{}, false
	}
	return app.Summary{Text: ru.Count(s.Unread, "новая", "новых", "новых") + ": " + s.Top}, true
}
