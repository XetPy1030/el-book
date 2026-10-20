// Package todo — задачи: сроки «сегодня», «завтра», «через неделю», просроченные сверху,
// выполненные внизу серым и зачёркнутыми, строка на домашнем экране — что на сегодня.
package todo

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

// App — приложение «Задачи».
type App struct {
	main     *kit.ListScreen
	showDone bool
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "todo", Title: "Задачи", Icon: icons.Checklist, Landscape: true}
}

// Task — задача. Due — срок «2026-10-19»; пусто — без срока.
type Task struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Done   bool   `json:"done,omitempty"`
	Due    string `json:"due,omitempty"`
	Added  int64  `json:"added"`
	DoneAt int64  `json:"done_at,omitempty"`
}

const tasksFile = "tasks.json"

func load(st *storage.App) []Task {
	data, err := st.ReadFile(tasksFile)
	if err != nil {
		return nil
	}
	var tasks []Task
	_ = json.Unmarshal(data, &tasks)
	return tasks
}

func store(ctx *app.Context, tasks []Task) {
	data, _ := json.Marshal(tasks)
	if err := ctx.Storage().WriteFile(tasksFile, data); err != nil {
		ctx.Fail("Не удалось сохранить задачи", err)
	}
}

func day(t time.Time) string { return t.Format("2006-01-02") }

// group — к какой части списка относится задача на дату now.
func group(t Task, now time.Time) int {
	switch {
	case t.Done:
		return 4
	case t.Due == "":
		return 3
	case t.Due < day(now):
		return 0 // просрочена
	case t.Due == day(now):
		return 1
	}
	return 2
}

// sorted — просроченные, на сегодня, позже по сроку, без срока, выполненные.
func sorted(tasks []Task, now time.Time) []Task {
	out := append([]Task(nil), tasks...)
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := group(out[i], now), group(out[j], now)
		if gi != gj {
			return gi < gj
		}
		if gi == 4 {
			return out[i].DoneAt > out[j].DoneAt
		}
		if out[i].Due != out[j].Due {
			return out[i].Due < out[j].Due
		}
		return out[i].Added < out[j].Added
	})
	return out
}

// dueText — «сегодня», «завтра», «пт, 24 октября», «просрочено на 2 дня».
func dueText(due string, now time.Time) string {
	t, err := time.ParseInLocation("2006-01-02", due, now.Location())
	if err != nil {
		return ""
	}
	today, _ := time.ParseInLocation("2006-01-02", day(now), now.Location())
	days := int(t.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		return "просрочено на " + ru.Count(-days, "день", "дня", "дней")
	case days == 0:
		return "сегодня"
	case days == 1:
		return "завтра"
	case days < 7:
		return ru.Weekday(t.Weekday())
	}
	return ru.DayMonth(t, now)
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.main = &kit.ListScreen{
		Title: "Задачи",
		Root:  true,
		Empty: "Задач нет. Нажмите «+», чтобы добавить. Тап по задаче отмечает её выполненной, долгое нажатие — срок и правка.",
		Actions: []*ui.Button{
			{Icon: icons.Check, Style: ui.Flat, OnTap: func() { a.toggleDone(ctx) }},
			{Icon: icons.Plus, Style: ui.Flat, OnTap: func() { a.add(ctx) }},
		},
		Items: func(ctx *app.Context) []ui.ListItem { return a.items(ctx) },
	}
	return a.main
}

func (a *App) toggleDone(ctx *app.Context) {
	a.showDone = !a.showDone
	if a.showDone {
		ctx.Toast("Выполненные показаны")
	} else {
		ctx.Toast("Выполненные скрыты")
	}
	a.main.Refresh(ctx)
}

func (a *App) items(ctx *app.Context) []ui.ListItem {
	now := ctx.Now()
	all := load(ctx.Storage())
	var items []ui.ListItem
	open, today, overdue := 0, 0, 0
	for _, t := range sorted(all, now) {
		g := group(t, now)
		switch g {
		case 0:
			overdue++
		case 1:
			today++
		}
		if !t.Done {
			open++
		}
		if t.Done && !a.showDone {
			continue
		}
		t := t
		item := ui.ListItem{Title: t.Text, Icon: icons.Checkbox(t.Done), Strike: t.Done, Muted: t.Done,
			OnTap: func() { a.setDone(ctx, t.ID, !t.Done) }, OnLongPress: func() { a.menu(ctx, t) }}
		if t.Due != "" && !t.Done {
			item.Subtitle = dueText(t.Due, now)
			item.Bold = g == 0
		}
		items = append(items, item)
	}
	var parts []string
	if overdue > 0 {
		parts = append(parts, ru.Count(overdue, "просрочена", "просрочены", "просрочено"))
	}
	if today > 0 {
		parts = append(parts, ru.Count(today, "на сегодня", "на сегодня", "на сегодня"))
	}
	if len(parts) == 0 && open > 0 {
		parts = append(parts, ru.Count(open, "задача", "задачи", "задач"))
	}
	if bar := a.main.Bar(); bar != nil {
		bar.SetTitle(ctx, "Задачи", strings.Join(parts, ", "))
	}
	return items
}

func (a *App) update(ctx *app.Context, id string, f func(*Task)) {
	tasks := load(ctx.Storage())
	for i := range tasks {
		if tasks[i].ID == id {
			f(&tasks[i])
		}
	}
	store(ctx, tasks)
	a.main.Refresh(ctx)
}

func (a *App) setDone(ctx *app.Context, id string, done bool) {
	a.update(ctx, id, func(t *Task) {
		t.Done = done
		t.DoneAt = 0
		if done {
			t.DoneAt = ctx.Now().Unix()
		}
	})
}

func (a *App) add(ctx *app.Context) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Новая задача", Hint: "Срок можно поставить долгим нажатием на задачу.", OK: "Добавить"},
		func(text string) {
			now := ctx.Now()
			tasks := load(ctx.Storage())
			tasks = append(tasks, Task{ID: strconv.FormatInt(now.UnixNano(), 36), Text: strings.TrimSpace(text), Added: now.Unix()})
			store(ctx, tasks)
			a.main.Refresh(ctx)
		})
}

// menu — срок, правка, удаление.
func (a *App) menu(ctx *app.Context, t Task) {
	now := ctx.Now()
	due := func(d time.Time) func() {
		return func() { a.update(ctx, t.ID, func(x *Task) { x.Due = day(d) }) }
	}
	ctx.Menu(t.Text,
		ui.MenuItem{Text: "Сегодня", Icon: icons.Calendar, Checked: t.Due == day(now), OnTap: due(now)},
		ui.MenuItem{Text: "Завтра", Icon: icons.Calendar, Checked: t.Due == day(now.AddDate(0, 0, 1)), OnTap: due(now.AddDate(0, 0, 1))},
		ui.MenuItem{Text: "Через неделю", Icon: icons.Calendar, Checked: t.Due == day(now.AddDate(0, 0, 7)), OnTap: due(now.AddDate(0, 0, 7))},
		ui.MenuItem{Text: "Без срока", Icon: icons.Close, Disabled: t.Due == "", OnTap: func() {
			a.update(ctx, t.ID, func(x *Task) { x.Due = "" })
		}},
		ui.MenuItem{Text: "Изменить текст", Icon: icons.Edit, OnTap: func() {
			kit.Prompt(ctx, kit.PromptOptions{Title: "Задача", Initial: t.Text}, func(text string) {
				a.update(ctx, t.ID, func(x *Task) { x.Text = strings.TrimSpace(text) })
			})
		}},
		ui.MenuItem{Text: "Удалить", Icon: icons.Trash, OnTap: func() {
			tasks := load(ctx.Storage())
			kept := tasks[:0]
			for _, x := range tasks {
				if x.ID != t.ID {
					kept = append(kept, x)
				}
			}
			store(ctx, kept)
			a.main.Refresh(ctx)
		}},
	)
}

// Summary — «3 задачи на сегодня, 1 просрочена: Купить хлеб».
func (a *App) Summary(st *storage.App, now time.Time) (app.Summary, bool) {
	tasks := sorted(load(st), now)
	today, overdue := 0, 0
	first := ""
	for _, t := range tasks {
		switch group(t, now) {
		case 0:
			overdue++
		case 1:
			today++
		default:
			continue
		}
		if first == "" {
			first = t.Text
		}
	}
	if today+overdue == 0 {
		return app.Summary{}, false
	}
	text := ""
	if today > 0 {
		text = ru.Count(today, "задача", "задачи", "задач") + " на сегодня"
	}
	if overdue > 0 {
		if text != "" {
			text += ", "
		}
		text += ru.Count(overdue, "просрочена", "просрочены", "просрочено")
	}
	return app.Summary{Text: text + ": " + first}, true
}
