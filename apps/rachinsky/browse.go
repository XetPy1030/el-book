package rachinsky

import (
	"strconv"
	"strings"

	"el-book/sdk/app"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
)

// Фильтры списка «Все задачи».
const (
	filterAll = iota
	filterUnsolved
	filterWrong
	filterFlagged
)

var filterNames = []string{"Все", "Нерешённые", "С ошибкой", "Отложенные"}

func (a *App) openBrowse(ctx *app.Context) {
	s := &kit.ListScreen{Title: "Все задачи", Empty: "В этом разделе задач нет."}
	s.Actions = []*ui.Button{
		{Icon: icons.Checklist, Style: ui.Flat, OnTap: func() { a.browseMenu(ctx, s) }},
		{Icon: icons.Search, Style: ui.Flat, OnTap: func() { a.jumpTo(ctx) }},
	}
	s.Items = func(ctx *app.Context) []ui.ListItem { return a.browseItems(ctx, s) }
	ctx.Push(s)
}

func (a *App) browseItems(ctx *app.Context, s *kit.ListScreen) []ui.ListItem {
	st := a.st
	var items []ui.ListItem
	for i := range problems {
		if !a.inFilter(i) {
			continue
		}
		i := i
		p := problems[i]
		it := ui.ListItem{
			Title: "№" + itoa(p.Num) + ". " + p.Text,
			OnTap: func() { a.openProblem(ctx, i) },
			OnLongPress: func() {
				st.toggleFlag(i)
				a.save(ctx)
				s.Refresh(ctx)
			},
		}
		if st.flags[i] {
			it.Icon = icons.Flag
		}
		switch st.status[i] {
		case stSolved:
			it.Detail, it.Muted = "решено", true
		case stWrong:
			it.Detail = "ошибка"
		}
		items = append(items, it)
	}
	if bar := s.Bar(); bar != nil {
		bar.SetTitle(ctx, "Все задачи", filterNames[a.browseFilter]+" · "+ru.Count(len(items), "задача", "задачи", "задач"))
	}
	return items
}

func (a *App) inFilter(i int) bool {
	switch a.browseFilter {
	case filterUnsolved:
		return a.st.status[i] != stSolved
	case filterWrong:
		return a.st.status[i] == stWrong
	case filterFlagged:
		return a.st.flags[i]
	}
	return true
}

func (a *App) browseMenu(ctx *app.Context, s *kit.ListScreen) {
	item := func(f int) ui.MenuItem {
		return ui.MenuItem{Text: filterNames[f], Checked: a.browseFilter == f, OnTap: func() {
			a.browseFilter = f
			s.List().SetPage(ctx, 0)
			s.Refresh(ctx)
		}}
	}
	ctx.Menu("Показать", item(filterAll), item(filterUnsolved), item(filterWrong), item(filterFlagged))
}

func (a *App) jumpTo(ctx *app.Context) {
	kit.Prompt(ctx, kit.PromptOptions{
		Title: "Перейти к задаче", Hint: "Номер задачи от 1 до 1001.", English: true, OK: "Открыть",
		Check: func(text string) string {
			if a.find(text) < 0 {
				return "Нет задачи с таким номером"
			}
			return ""
		},
	}, func(text string) {
		if idx := a.find(text); idx >= 0 {
			a.openProblem(ctx, idx)
		}
	})
}

// find возвращает позицию первой задачи с номером из текста; -1, если нет.
func (a *App) find(text string) int {
	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return -1
	}
	for i := range problems {
		if problems[i].Num == num {
			return i
		}
	}
	return -1
}
