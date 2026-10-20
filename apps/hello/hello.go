// Package hello — минимальное приложение-образец: счётчик нажатий, который переживает перезапуск.
package hello

import (
	"fmt"
	"image"

	"el-book/sdk/app"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ui"
)

// App — приложение «Привет»; оно же его единственный экран.
type App struct {
	count int
	page  ui.Page
}

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "hello", Title: "Привет", Icon: icons.Plus}
}

func (a *App) Start(ctx *app.Context) app.Screen {
	_, _ = ctx.Storage().LoadSettings(&a.count) // при первом запуске настроек нет — счёт с нуля
	return a
}

func (a *App) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	a.page.Clear()
	label := &ui.Label{Text: fmt.Sprint(a.count), Align: gfx.AlignCenter, Face: func(t *ui.Theme) *gfx.Face { return t.Header() }}
	label.SetBounds(image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+b.Dy()/2))
	tap := &ui.Button{Text: "Нажми", Style: ui.Primary, OnTap: func() {
		a.count++
		if err := ctx.Storage().SaveSettings(1, a.count); err != nil {
			ctx.Fail("Не удалось сохранить счёт", err)
		}
		label.Text = fmt.Sprint(a.count)
		ctx.Invalidate(label.R, eink.UI)
	}}
	tap.SetBounds(image.Rect(b.Min.X+t.MM(20), label.R.Max.Y, b.Max.X-t.MM(20), label.R.Max.Y+t.Touch()))
	corner := image.Rect(0, 0, t.Touch(), t.Touch()).Add(b.Min.Add(image.Pt(t.Pad(), t.Pad())))
	a.page.Add(label, tap, &ui.Button{Box: ui.Box{R: corner}, Icon: icons.Home, Style: ui.Flat, OnTap: ctx.Pop})
}

func (a *App) Draw(c *gfx.Canvas, t *ui.Theme)       { a.page.Draw(c, t) }
func (a *App) Handle(ctx *app.Context, ev app.Event) { app.RoutePage(ctx, &a.page, ev) }
