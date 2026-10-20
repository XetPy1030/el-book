package draw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
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

// gallery — тетради с миниатюрами открытых страниц.
type gallery struct {
	a         *App
	page      ui.Page
	grid      *ui.Grid
	pageLabel *ui.Label
	thumbs    map[string]*image.Gray
	bounds    image.Rectangle
	migrated  bool
}

func newGallery(a *App) *gallery { return &gallery{a: a, thumbs: map[string]*image.Gray{}} }

// books читает тетради, недавно изменённые первыми.
func (g *gallery) books(ctx *app.Context) []*Book {
	st := ctx.Storage()
	dirs, err := st.List("books")
	if err != nil {
		ctx.Log().Error("список тетрадей", "err", err)
		return nil
	}
	var out []*Book
	for _, d := range dirs {
		if !d.Dir {
			continue
		}
		b, err := loadBook(st, d.Name)
		if err != nil {
			ctx.Log().Warn("тетрадь не читается", "id", d.Name, "err", err)
			continue
		}
		out = append(out, b)
	}
	return out
}

func (g *gallery) Layout(ctx *app.Context, b image.Rectangle) {
	if !g.migrated {
		g.migrated = true
		if n, err := migrateV1(ctx.Storage()); err != nil {
			ctx.Fail("Часть рисунков не перенеслась в тетради", err)
		} else if n > 0 {
			ctx.Log().Info("рисунки перенесены в тетради", "n", n)
		}
	}
	g.bounds = b
	t := ctx.Theme()
	g.page.Clear()
	books := g.books(ctx)
	r := b
	add := &ui.Button{Text: "Новая", Icon: icons.Plus, Style: ui.Primary, OnTap: func() { g.create(ctx) }}
	bar := ui.NewTitleBar(t, "Рисовалка", func() { ctx.Home() }, add)
	bar.BackIcon = icons.Home
	if len(books) > 0 {
		bar.Subtitle = ru.Count(len(books), "тетрадь", "тетради", "тетрадей")
	}
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	g.page.Add(bar)

	r = ui.Pad(r, t.Pad(), t.MM(3), t.Pad(), t.Pad())
	labelH := t.Caption().Height
	g.pageLabel = &ui.Label{Align: gfx.AlignCenter, Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}
	g.pageLabel.SetBounds(ui.CutBottom(&r, labelH+t.MM(1)))
	if len(books) == 0 {
		g.grid = nil
		empty := &ui.Label{Text: "Тетрадей пока нет. Нажмите «Новая», чтобы начать: тетрадь — это страницы с разметкой, кистями и фигурами.",
			Wrap: true, Align: gfx.AlignCenter, Color: gfx.Gray16(5)}
		empty.SetBounds(r)
		g.page.Add(empty)
		return
	}
	cols := 3
	if b.Dx() > b.Dy() {
		cols = 4
	}
	cellW := r.Dx() / cols
	cellH := cellW*b.Dy()/b.Dx() + 2*labelH + t.MM(3)
	rows := max(r.Dy()/cellH, 1)
	page := 0
	if g.grid != nil {
		page = g.grid.Page()
	}
	g.grid = &ui.Grid{Cols: cols, Rows: rows, OnPage: func(int, int) { g.updatePageLabel(ctx) }}
	now := ctx.Now()
	for _, bk := range books {
		bk := bk
		g.grid.Items = append(g.grid.Items, &thumbTile{
			title:  bk.title(),
			sub:    fmt.Sprintf("%s · %s", ru.Count(len(bk.Pages), "стр.", "стр.", "стр."), ru.Ago(time.Unix(bk.Modified, 0), now)),
			thumb:  func() *image.Gray { return g.thumb(ctx, bk.ID) },
			onTap:  func() { g.open(ctx, bk) },
			onLong: func() { g.menu(ctx, bk) },
		})
	}
	g.grid.SetBounds(image.Rect(r.Min.X, r.Min.Y, r.Min.X+cols*cellW, r.Min.Y+rows*cellH))
	g.grid.SetPage(ctx, page)
	g.page.Add(g.grid, g.pageLabel)
	g.updatePageLabel(ctx)
}

func (g *gallery) updatePageLabel(ctx *app.Context) {
	if g.grid == nil {
		return
	}
	g.pageLabel.Text = "долгое нажатие — меню тетради"
	if g.grid.Pages() > 1 {
		g.pageLabel.Text = g.grid.PageText() + " · " + g.pageLabel.Text
	}
	ctx.Invalidate(g.pageLabel.R, eink.UI)
}

func (g *gallery) thumb(ctx *app.Context, id string) *image.Gray {
	if img, ok := g.thumbs[id]; ok {
		return img
	}
	var img *image.Gray
	if data, err := ctx.Storage().ReadFile(bookThumb(id)); err == nil {
		if decoded, err := png.Decode(bytes.NewReader(data)); err == nil {
			img = gfx.ToGray(decoded)
		}
	}
	g.thumbs[id] = img
	return img
}

// create спрашивает разметку и открывает новую тетрадь.
func (g *gallery) create(ctx *app.Context) {
	opts := make([]string, len(templates))
	sel := 0
	for i, t := range templates {
		opts[i] = t.title()
		if t == g.a.prefs.Value.Template {
			sel = i
		}
	}
	ctx.ShowOverlay(ui.NewChoice(ctx.Theme(), "Разметка листа", opts, sel, func(i int) {
		g.a.prefs.Value.Template = templates[i]
		g.a.save(ctx)
		b := newBook(ctx.Now(), 0, 0, ctx.Theme().DPI, templates[i])
		ctx.Push(newEditor(g.a, b))
	}))
}

func (g *gallery) open(ctx *app.Context, b *Book) {
	delete(g.thumbs, b.ID)
	ctx.Push(newEditor(g.a, b))
}

// menu — действия с тетрадью.
func (g *gallery) menu(ctx *app.Context, b *Book) {
	st := ctx.Storage()
	refresh := func() {
		g.Layout(ctx, g.bounds)
		ctx.InvalidateAll(eink.UI)
	}
	ctx.Menu(b.title(),
		ui.MenuItem{Text: "Открыть", Icon: icons.Pencil, OnTap: func() { g.open(ctx, b) }},
		ui.MenuItem{Text: "Название", Icon: icons.Edit, OnTap: func() {
			renameBook(ctx, b, func() {
				data, err := jsonBook(b)
				if err == nil {
					err = st.WriteFile(bookFile(b.ID), data)
				}
				if err != nil {
					ctx.Fail("Не удалось переименовать тетрадь", err, "id", b.ID)
				}
				refresh()
			})
		}},
		ui.MenuItem{Text: "Копия", Icon: icons.Copy, OnTap: func() {
			if err := copyBook(st, b, ctx.Now()); err != nil {
				ctx.Fail("Не удалось скопировать тетрадь", err, "id", b.ID)
				return
			}
			ctx.Toast("Копия готова")
			refresh()
		}},
		ui.MenuItem{Text: "Выгрузить в PDF", Icon: icons.Export, OnTap: func() { exportPDF(ctx, b) }},
		ui.MenuItem{Text: "Удалить", Icon: icons.Trash, OnTap: func() {
			ctx.Confirm("Удалить тетрадь?", fmt.Sprintf("«%s» — %s. Удалённую тетрадь не вернуть.", b.title(),
				ru.Count(len(b.Pages), "страница", "страницы", "страниц")), "Удалить", func() {
				if err := st.Remove(b.dir()); err != nil {
					ctx.Fail("Не удалось удалить тетрадь", err, "id", b.ID)
					return
				}
				delete(g.thumbs, b.ID)
				refresh()
			})
		}},
	)
}

func (g *gallery) Draw(c *gfx.Canvas, t *ui.Theme) { g.page.Draw(c, t) }

func (g *gallery) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case app.Lifecycle:
		if e == app.Shown && !g.bounds.Empty() {
			// Вернулись из тетради: список и миниатюры могли измениться.
			g.Layout(ctx, g.bounds)
		}
	case device.KeyEvent:
		if g.grid != nil && kit.PageKeys(ctx, e, func(h ui.Host) bool { return g.grid.SetPage(h, g.grid.Page()+1) },
			func(h ui.Host) bool { return g.grid.SetPage(h, g.grid.Page()-1) }) {
			return
		}
	}
	app.RoutePage(ctx, &g.page, ev)
}

// thumbTile — миниатюра тетради с названием и подписью.
type thumbTile struct {
	ui.Box
	title  string
	sub    string
	thumb  func() *image.Gray
	onTap  func()
	onLong func()
}

func (tt *thumbTile) Enabled() bool                  { return true }
func (tt *thumbTile) Tap(ui.Host, image.Point)       { tt.onTap() }
func (tt *thumbTile) LongPress(ui.Host, image.Point) { tt.onLong() }

func (tt *thumbTile) Draw(c *gfx.Canvas, t *ui.Theme) {
	r := tt.Bounds().Inset(t.MM(1.5))
	labelH := t.Caption().Height
	box := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-2*labelH-t.MM(1))
	if img := tt.thumb(); img != nil {
		fit := gfx.Fit(img.Bounds().Size(), box.Inset(t.Line()))
		c.Scale(fit, img)
		c.StrokeRect(fit.Inset(-t.Line()), t.Line(), gfx.Black)
	} else {
		c.StrokeRect(box, t.Line(), gfx.Gray16(8))
	}
	y := box.Max.Y + t.MM(1)
	t.Face(gfx.Medium, 10).DrawIn(c, tt.title, image.Rect(r.Min.X, y, r.Max.X, y+labelH), gfx.AlignCenter, gfx.Black)
	t.Caption().DrawIn(c, tt.sub, image.Rect(r.Min.X, y+labelH, r.Max.X, y+2*labelH), gfx.AlignCenter, gfx.Gray16(5))
}

func dateTitle(t time.Time) string {
	return fmt.Sprintf("%d %s, %s", t.Day(), shortMonth(t.Month()), t.Format("15:04"))
}

var shortMonths = [...]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}

func shortMonth(m time.Month) string { return shortMonths[m-1] }

// jsonBook — описание тетради для записи.
func jsonBook(b *Book) ([]byte, error) { return json.Marshal(b) }

// copyBook копирует тетрадь со всеми страницами под новым id.
func copyBook(st *storage.App, b *Book, now time.Time) error {
	cp := *b
	cp.ID = newID(now)
	cp.Title = b.title() + " (копия)"
	cp.Created, cp.Modified = now.Unix(), now.Unix()
	cp.Pages = nil
	for _, p := range b.Pages {
		if err := b.load(st, p); err != nil {
			return err
		}
		np := *p
		np.strokes = p.strokes
		np.dirty = true
		cp.Pages = append(cp.Pages, &np)
	}
	var thumb *image.Gray
	if data, err := st.ReadFile(bookThumb(b.ID)); err == nil {
		if img, err := png.Decode(bytes.NewReader(data)); err == nil {
			thumb = gfx.ToGray(img)
		}
	}
	snap, err := cp.snapshot(nil, now)
	if err != nil {
		return err
	}
	snap.thumb = thumb
	return snap.write(st)
}
