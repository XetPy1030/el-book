package kit

import (
	"image"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ui"
)

// ListScreen — экран-список: строка заголовка, строки, листание свайпом и кнопками листания.
// Строки строит Items при каждом показе экрана: вернулись с вложенного экрана — список свежий.
type ListScreen struct {
	Title    string
	Subtitle string
	Empty    string // что сказать, когда строк нет
	Items    func(ctx *app.Context) []ui.ListItem
	Actions  []*ui.Button // кнопки справа в строке заголовка
	// Back — что делает кнопка слева; nil — закрыть экран. Root — первый экран приложения:
	// кнопка-домик уводит на домашний экран, приложение остаётся в фоне.
	Back func(ctx *app.Context)
	Root bool
	// Bottom — виджет под списком: кнопка «Добавить», строка состояния; nil — нет.
	Bottom       ui.Widget
	BottomHeight int // 0 — высота основной цели касания

	page ui.Page
	bar  *ui.TitleBar
	list *ui.List
}

// List — список экрана: для перехода на страницу с нужной строкой.
func (s *ListScreen) List() *ui.List { return s.list }

// Bar — строка заголовка: чтобы сменить заголовок.
func (s *ListScreen) Bar() *ui.TitleBar { return s.bar }

func (s *ListScreen) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	s.page.Clear()
	r := b
	back := func() {
		switch {
		case s.Back != nil:
			s.Back(ctx)
		case s.Root:
			ctx.Home()
		default:
			ctx.Pop()
		}
	}
	s.bar = ui.NewTitleBar(t, s.Title, back, s.Actions...)
	s.bar.Subtitle = s.Subtitle
	if s.Root && s.Back == nil {
		s.bar.BackIcon = icons.Home
	}
	s.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	s.page.Add(s.bar)
	if s.Bottom != nil {
		h := s.BottomHeight
		if h == 0 {
			h = t.Touch()
		}
		bottom := ui.CutBottom(&r, h+t.MM(3))
		s.Bottom.SetBounds(ui.Pad(bottom, t.Pad(), t.MM(1), t.Pad(), t.MM(2)))
		s.page.Add(s.Bottom)
	}
	page := 0
	if s.list != nil {
		page = s.list.Page()
	}
	s.list = ui.NewList(t)
	s.list.Empty = s.Empty
	s.list.SetBounds(ui.Pad(r, t.Pad()-t.MM(2), t.MM(1), t.Pad()-t.MM(1), t.MM(1)))
	if s.Items != nil {
		s.list.SetItems(s.Items(ctx))
	}
	s.list.SetPage(ctx, page)
	s.page.Add(s.list)
}

// Refresh перестраивает строки и перерисовывает список.
func (s *ListScreen) Refresh(ctx *app.Context) {
	if s.list == nil || s.Items == nil {
		return
	}
	s.list.SetItems(s.Items(ctx))
	s.list.Refresh(ctx)
}

func (s *ListScreen) Draw(c *gfx.Canvas, t *ui.Theme) { s.page.Draw(c, t) }

func (s *ListScreen) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case app.Lifecycle:
		if e == app.Shown {
			s.Refresh(ctx)
		}
	case device.KeyEvent:
		if PageKeys(ctx, e, s.list.Next, s.list.Prev) {
			return
		}
	}
	app.RoutePage(ctx, &s.page, ev)
}

// PageKeys отдаёт кнопки листания функциям next и prev. Возвращает true, если событие —
// кнопка листания (нажатие или отпускание).
func PageKeys(h ui.Host, e device.KeyEvent, next, prev func(ui.Host) bool) bool {
	switch e.Key {
	case device.KeyPageForward:
		if e.Pressed {
			next(h)
		}
		return true
	case device.KeyPageBack:
		if e.Pressed {
			prev(h)
		}
		return true
	}
	return false
}
