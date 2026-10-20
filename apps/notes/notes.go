// Package notes — заметки: быстрый текст с экранной клавиатуры, закреплённые сверху,
// поиск, чтение страницами, выгрузка в текстовый файл, видимый по USB.
package notes

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// App — приложение «Заметки».
type App struct {
	main  *kit.ListScreen
	query string // поиск; пусто — все заметки
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "notes", Title: "Заметки", Icon: icons.Note, Landscape: true}
}

// Note — заметка. Первая строка — заголовок.
type Note struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Created  int64  `json:"created"`
	Modified int64  `json:"modified"`
	Pinned   bool   `json:"pinned,omitempty"`
}

func noteFile(id string) string { return "notes/" + id + ".json" }

// title — первая непустая строка.
func (n Note) title() string {
	for _, line := range strings.Split(n.Text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "Пустая заметка"
}

// preview — начало текста после заголовка.
func (n Note) preview() string {
	text := strings.TrimSpace(n.Text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.Join(strings.Fields(text[i+1:]), " ")
	}
	return ""
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.main = &kit.ListScreen{
		Title: "Заметки",
		Root:  true,
		Empty: "Заметок пока нет. Нажмите «+», чтобы записать мысль, список покупок или цитату.",
		Actions: []*ui.Button{
			{Icon: icons.Search, Style: ui.Flat, OnTap: func() { a.search(ctx) }},
			{Icon: icons.Plus, Style: ui.Flat, OnTap: func() { a.create(ctx) }},
		},
		Items: func(ctx *app.Context) []ui.ListItem { return a.items(ctx) },
	}
	return a.main
}

// load читает все заметки: закреплённые первыми, затем недавно изменённые.
func load(st *storage.App) []Note {
	files, _ := st.List("notes")
	var out []Note
	for _, f := range files {
		data, err := st.ReadFile("notes/" + f.Name)
		if err != nil {
			continue
		}
		var n Note
		if json.Unmarshal(data, &n) == nil && n.ID != "" {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].Modified > out[j].Modified
	})
	return out
}

func save(ctx *app.Context, n Note) {
	data, _ := json.Marshal(n)
	if err := ctx.Storage().WriteFile(noteFile(n.ID), data); err != nil {
		ctx.Fail("Не удалось сохранить заметку", err)
	}
}

func (a *App) items(ctx *app.Context) []ui.ListItem {
	notes := load(ctx.Storage())
	q := strings.ToLower(a.query)
	now := ctx.Now()
	var items []ui.ListItem
	for _, n := range notes {
		if q != "" && !strings.Contains(strings.ToLower(n.Text), q) {
			continue
		}
		n := n
		sub := ru.Ago(time.Unix(n.Modified, 0), now)
		if p := n.preview(); p != "" {
			sub += " · " + p
		}
		item := ui.ListItem{Title: n.title(), Subtitle: sub, OnTap: func() { a.open(ctx, n) }, OnLongPress: func() { a.menu(ctx, n, false) }}
		if n.Pinned {
			item.Icon = icons.Pin
		}
		items = append(items, item)
	}
	sub := ""
	if a.query != "" {
		sub = "поиск: «" + a.query + "» · " + ru.Count(len(items), "заметка", "заметки", "заметок")
	} else if len(items) > 0 {
		sub = ru.Count(len(items), "заметка", "заметки", "заметок")
	}
	if bar := a.main.Bar(); bar != nil {
		bar.SetTitle(ctx, "Заметки", sub)
	} else {
		a.main.Subtitle = sub
	}
	return items
}

func (a *App) search(ctx *app.Context) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Поиск", Initial: a.query, Hint: "Пустой поиск покажет все заметки.", AllowEmpty: true, OK: "Найти"},
		func(q string) {
			a.query = strings.TrimSpace(q)
			a.main.Refresh(ctx)
			a.main.List().SetPage(ctx, 0)
		})
}

func (a *App) create(ctx *app.Context) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Новая заметка", Multiline: true, Placeholder: "Первая строка станет заголовком"},
		func(text string) {
			now := ctx.Now()
			n := Note{ID: strconv.FormatInt(now.UnixNano(), 36), Text: strings.TrimRight(text, "\n "), Created: now.Unix(), Modified: now.Unix()}
			save(ctx, n)
			a.main.Refresh(ctx)
		})
}

// open показывает заметку страницами; «Изменить» открывает редактор.
func (a *App) open(ctx *app.Context, n Note) {
	var screen *kit.ReaderScreen
	edit := &ui.Button{Text: "Изменить", Icon: icons.Edit, Style: ui.Primary, OnTap: func() {
		a.edit(ctx, n, func(changed Note) {
			n = changed
			screen.Bar().SetTitle(ctx, n.title(), "изменена "+ru.Ago(time.Unix(n.Modified, 0), ctx.Now()))
			screen.SetBlocks(ctx, kit.Paragraphs(n.Text))
		})
	}}
	more := &ui.Button{Icon: icons.More, Style: ui.Flat, OnTap: func() { a.menu(ctx, n, true) }}
	screen = &kit.ReaderScreen{Title: n.title(), Subtitle: "изменена " + ru.Ago(time.Unix(n.Modified, 0), ctx.Now()),
		Blocks: kit.Paragraphs(n.Text), Bottom: edit, Actions: []*ui.Button{more}, Size: 14}
	ctx.Push(screen)
}

func (a *App) edit(ctx *app.Context, n Note, done func(Note)) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Заметка", Initial: n.Text, Multiline: true, AllowEmpty: true},
		func(text string) {
			n.Text = strings.TrimRight(text, "\n ")
			n.Modified = ctx.Now().Unix()
			save(ctx, n)
			a.main.Refresh(ctx)
			done(n)
		})
}

// menu — закрепить, выгрузить, удалить. inReader — меню открыто из просмотра заметки:
// удалённую заметку он закрывает.
func (a *App) menu(ctx *app.Context, n Note, inReader bool) {
	pin := "Закрепить сверху"
	if n.Pinned {
		pin = "Открепить"
	}
	ctx.Menu(n.title(),
		ui.MenuItem{Text: pin, Icon: icons.Pin, OnTap: func() {
			n.Pinned = !n.Pinned
			save(ctx, n)
			a.main.Refresh(ctx)
		}},
		ui.MenuItem{Text: "Выгрузить в .txt", Icon: icons.Export, OnTap: func() {
			name := strings.Map(func(r rune) rune {
				if strings.ContainsRune(`/\:*?"<>|`, r) {
					return '_'
				}
				return r
			}, n.title())
			if len([]rune(name)) > 40 {
				name = string([]rune(name)[:40])
			}
			path, err := ctx.Storage().Export(name+".txt", []byte(n.Text+"\n"))
			if err != nil {
				ctx.Fail("Не удалось выгрузить заметку", err)
				return
			}
			ctx.Toast("Сохранено: " + path)
		}},
		ui.MenuItem{Text: "Удалить", Icon: icons.Trash, OnTap: func() {
			ctx.Confirm("Удалить заметку?", "«"+n.title()+"» не вернуть.", "Удалить", func() {
				if err := ctx.Storage().Remove(noteFile(n.ID)); err != nil {
					ctx.Fail("Не удалось удалить заметку", err)
					return
				}
				if inReader {
					ctx.Pop()
				}
				a.main.Refresh(ctx)
			})
		}},
	)
}
