package shell

import (
	"image"

	"el-book/sdk/app"
	"el-book/sdk/gfx"
	"el-book/sdk/ui"
)

// formScreen — экран настроек одного раздела: строка заголовка и столбец разделов
// с подписями и виджетами. build раскладывает содержимое заново при каждом Layout.
type formScreen struct {
	title string
	build func(ctx *app.Context, f *form)
	page  ui.Page
}

func newForm(title string, build func(ctx *app.Context, f *form)) *formScreen {
	return &formScreen{title: title, build: build}
}

func (fs *formScreen) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	fs.page.Clear()
	r := b
	bar := ui.NewTitleBar(t, fs.title, func() { ctx.Pop() })
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	fs.page.Add(bar)
	r = ui.Pad(r, t.Pad(), t.MM(4), t.Pad(), t.Pad())
	fs.build(ctx, &form{t: t, page: &fs.page, r: r, y: r.Min.Y})
}

func (fs *formScreen) Draw(c *gfx.Canvas, t *ui.Theme) { fs.page.Draw(c, t) }

func (fs *formScreen) Handle(ctx *app.Context, ev app.Event) { app.RoutePage(ctx, &fs.page, ev) }

// form раскладывает строки сверху вниз.
type form struct {
	t    *ui.Theme
	page *ui.Page
	r    image.Rectangle
	y    int
}

// section — заголовок раздела мелкими прописными.
func (f *form) section(text string) {
	t := f.t
	l := &ui.Label{Text: text, Face: func(t *ui.Theme) *gfx.Face { return t.Face(gfx.Bold, 9) }, Color: gfx.Gray16(5)}
	f.add(l, t.Caption().Height)
	f.y += t.MM(1.5)
}

// caption — подпись над виджетом.
func (f *form) caption(text string) {
	f.add(&ui.Label{Text: text}, f.t.Body().Height)
	f.y += f.t.MM(1.5)
}

// note — пояснение серым, в несколько строк.
func (f *form) note(text string) *ui.Label {
	l := &ui.Label{Text: text, Wrap: true, VAlign: -1, Color: gfx.Gray16(4)}
	f.add(l, l.Height(f.t, f.r.Dx()))
	f.y += f.t.MM(3)
	return l
}

// row — виджет высотой в цель касания.
func (f *form) row(w ui.Widget) {
	f.rowH(w, f.t.Touch())
}

// rowH — виджет заданной высоты.
func (f *form) rowH(w ui.Widget, h int) {
	f.add(w, h)
	f.y += f.t.MM(5)
}

func (f *form) add(w ui.Widget, h int) {
	w.SetBounds(image.Rect(f.r.Min.X, f.y, f.r.Max.X, f.y+h))
	f.page.Add(w)
	f.y += h
}

// rest — всё, что осталось до низа экрана.
func (f *form) rest() image.Rectangle {
	return image.Rect(f.r.Min.X, f.y, f.r.Max.X, f.r.Max.Y)
}
