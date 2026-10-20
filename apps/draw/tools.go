package draw

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"strings"

	"el-book/sdk/app"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ink"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// widthLabels — подписи толщин: «0,3», «1», «5».
func widthLabels() []string {
	out := make([]string, len(widths))
	for i, w := range widths {
		out[i] = ru.Decimal(w, 1)
	}
	return out
}

func nearest(xs []float64, v float64) int {
	best := 0
	for i, x := range xs {
		if abs(x-v) < abs(xs[best]-v) {
			best = i
		}
	}
	return best
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func toneIndex(v uint8) int {
	best := 0
	for i, t := range tones {
		if absInt(int(t)-int(v)) < absInt(int(tones[best])-int(v)) {
			best = i
		}
	}
	return best
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// brushPanel — выбор кисти, толщины и цвета.
func (s *editor) brushPanel(ctx *app.Context) {
	t := ctx.Theme()
	pr := s.prefs()
	panel := &ui.Panel{Title: "Кисть", Anchor: ui.AnchorTop, Dismissable: true}
	var width, tone *ui.Segmented
	kinds := make([]*ui.Button, 0, len(ink.Brushes))
	choose := func(b ink.Brush) {
		pr.Brush = b
		for i, k := range kinds {
			k.Active = ink.Brushes[i] == b
		}
		width.Selected = nearest(widths, pr.Widths[b])
		tone.Selected = toneIndex(pr.Tones[b])
		s.a.save(ctx)
		s.styleBar(ctx)
		ctx.Invalidate(panel.Bounds(), eink.UI)
	}
	for _, b := range ink.Brushes {
		b := b
		kinds = append(kinds, &ui.Button{Text: b.Title(), Icon: brushIcon(b), Vertical: true, Style: ui.Flat,
			Active: b == pr.Brush, OnTap: func() { choose(b) }})
	}
	row1 := &ui.HStack{Gap: t.Gap()}
	row2 := &ui.HStack{Gap: t.Gap()}
	for i, k := range kinds {
		if i < 4 {
			row1.Items = append(row1.Items, k)
		} else {
			row2.Items = append(row2.Items, k)
		}
	}
	for len(row2.Items) < len(row1.Items) {
		row2.Items = append(row2.Items, &ui.Label{})
	}
	width = &ui.Segmented{Options: widthLabels(), Selected: nearest(widths, pr.Widths[pr.Brush]), OnChange: func(i int) {
		pr.Widths[pr.Brush] = widths[i]
		s.a.save(ctx)
	}}
	tone = &ui.Segmented{Options: toneNames, Selected: toneIndex(pr.Tones[pr.Brush]), OnChange: func(i int) {
		pr.Tones[pr.Brush] = tones[i]
		s.a.save(ctx)
	}}
	caption := func(text string) ui.Row {
		return ui.Row{W: &ui.Label{Text: text, Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }},
			H: t.Caption().Height}
	}
	tileH := t.MM(15)
	panel.Rows = []ui.Row{
		{W: row1, H: tileH}, {W: row2, H: tileH},
		caption("Толщина, мм"), {W: width},
		caption("Цвет"), {W: tone},
		{W: &ui.Button{Text: "Готово", Style: ui.Primary, OnTap: func() { panel.Close() }}},
	}
	ctx.ShowOverlay(panel)
}

// eraserPanel — как стирать и чем.
func (s *editor) eraserPanel(ctx *app.Context) {
	t := ctx.Theme()
	pr := s.prefs()
	panel := &ui.Panel{Title: "Ластик", Anchor: ui.AnchorTop, Dismissable: true}
	mode := 0
	if pr.Eraser == eraseStroke {
		mode = 1
	}
	panel.Rows = []ui.Row{
		{W: &ui.Segmented{Options: []string{"По площади", "Штрихами"}, Selected: mode, OnChange: func(i int) {
			pr.Eraser = eraseArea
			if i == 1 {
				pr.Eraser = eraseStroke
			}
			s.a.save(ctx)
		}}},
		{W: &ui.Label{Text: "Толщина, мм", Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}, H: t.Caption().Height},
		{W: &ui.Segmented{Options: widthLabels(), Selected: nearest(widths, pr.EraserMM), OnChange: func(i int) {
			pr.EraserMM = widths[i]
			s.a.save(ctx)
		}}},
		{W: &ui.Button{Text: "Очистить страницу", Icon: icons.ClearPage, OnTap: func() {
			panel.Close()
			s.confirmClear(ctx)
		}}},
		{W: &ui.Button{Text: "Готово", Style: ui.Primary, OnTap: func() { panel.Close() }}},
	}
	ctx.ShowOverlay(panel)
}

func (s *editor) confirmClear(ctx *app.Context) {
	if len(s.page.strokes) == 0 {
		ctx.Toast("Страница и так чистая")
		return
	}
	ctx.Confirm("Очистить страницу?", "Отменить очистку можно кнопкой «Отменить».", "Очистить", func() {
		var removed []placed
		for i := len(s.page.strokes) - 1; i >= 0; i-- {
			removed = append(removed, placed{s: s.page.strokes[i], at: i})
		}
		o := op{removed: removed}
		s.push(o)
		s.apply(ctx, o, false)
	})
}

// pagesPanel — страницы тетради: добавить, скопировать, удалить, разметка страницы.
func (s *editor) pagesPanel(ctx *app.Context) {
	t := ctx.Theme()
	b := s.book
	panel := &ui.Panel{Title: fmt.Sprintf("Страница %d из %d", b.Current+1, len(b.Pages)), Anchor: ui.AnchorTop, Dismissable: true}
	opts := make([]string, len(templates))
	sel := 0
	for i, tm := range templates {
		opts[i] = tm.title()
		if tm == b.template(s.page) {
			sel = i
		}
	}
	actions := &ui.HStack{Gap: t.Gap(), Items: []ui.Widget{
		&ui.Button{Text: "Новая", Icon: icons.PageAdd, Vertical: true, Style: ui.Flat, OnTap: func() {
			panel.Close()
			s.addPage(ctx)
		}},
		&ui.Button{Text: "Копия", Icon: icons.Copy, Vertical: true, Style: ui.Flat, OnTap: func() {
			panel.Close()
			s.duplicatePage(ctx)
		}},
		&ui.Button{Text: "Удалить", Icon: icons.Trash, Vertical: true, Style: ui.Flat, Disabled: len(b.Pages) < 2, OnTap: func() {
			panel.Close()
			ctx.Confirm("Удалить страницу?", "Страницу не вернуть.", "Удалить", func() { s.deletePage(ctx) })
		}},
		&ui.Button{Text: "Первая", Icon: icons.Pages, Vertical: true, Style: ui.Flat, Disabled: b.Current == 0, OnTap: func() {
			panel.Close()
			s.goPage(ctx, 0)
		}},
	}}
	panel.Rows = []ui.Row{
		{W: actions, H: t.MM(15)},
		{W: &ui.Label{Text: "Разметка страницы", Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}, H: t.Caption().Height},
		{W: &ui.Segmented{Options: opts, Selected: sel, OnChange: func(i int) {
			old := b.template(s.page)
			if templates[i] == old {
				return
			}
			o := op{tmpl: &[2]Template{s.page.Template, templates[i]}}
			s.push(o)
			s.apply(ctx, o, false)
			// Новые страницы тетради — с той же разметкой.
			b.Template = templates[i]
			s.a.prefs.Value.Template = templates[i]
			s.a.save(ctx)
		}}},
		{W: &ui.Button{Text: "Готово", Style: ui.Primary, OnTap: func() { panel.Close() }}},
	}
	ctx.ShowOverlay(panel)
}

func (s *editor) duplicatePage(ctx *app.Context) {
	n := s.book.addPage(ctx.Now(), s.book.Current, s.book.template(s.page))
	p := s.book.Pages[n]
	for _, st := range s.page.strokes {
		p.strokes = append(p.strokes, st.Clone())
	}
	s.goPage(ctx, n)
	ctx.Toast("Страница скопирована")
}

func (s *editor) deletePage(ctx *app.Context) {
	b := s.book
	if len(b.Pages) < 2 {
		return
	}
	gone := s.page
	n := b.Current
	b.Pages = append(b.Pages[:n], b.Pages[n+1:]...)
	delete(s.hist, gone.ID)
	removePage(ctx.Storage(), b.ID, gone.ID)
	s.openPage(ctx, min(n, len(b.Pages)-1))
	s.dirty = true
	s.markDirty(ctx)
	s.styleBar(ctx)
	ctx.Invalidate(s.area, eink.Content)
	ctx.Toast("Страница удалена")
}

// moreMenu — название, выгрузка, очистка.
func (s *editor) moreMenu(ctx *app.Context) {
	ctx.Menu(s.book.title(),
		ui.MenuItem{Text: "Название тетради", Icon: icons.Edit, OnTap: func() { renameBook(ctx, s.book, func() { s.dirty = true; s.save(ctx, false) }) }},
		ui.MenuItem{Text: "Страница в PNG", Icon: icons.Export, OnTap: func() { s.exportPNG(ctx) }},
		ui.MenuItem{Text: "Страница в SVG", Icon: icons.Export, OnTap: func() { s.exportSVG(ctx) }},
		ui.MenuItem{Text: "Тетрадь в PDF", Icon: icons.Book, OnTap: func() {
			s.save(ctx, true)
			exportPDF(ctx, s.book)
		}},
		ui.MenuItem{Text: "Очистить страницу", Icon: icons.ClearPage, OnTap: func() { s.confirmClear(ctx) }},
	)
}

// renameBook спрашивает название тетради.
func renameBook(ctx *app.Context, b *Book, done func()) {
	kit.Prompt(ctx, kit.PromptOptions{Title: "Название тетради", Initial: b.Title, Placeholder: b.title(), AllowEmpty: true},
		func(text string) {
			b.Title = strings.TrimSpace(text)
			done()
		})
}

// exportName — имя файла выгрузки: название и время.
func exportName(b *Book, ext string, now interface{ Format(string) string }) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, b.title())
	return name + " " + now.Format("2006-01-02 15-04-05") + "." + ext
}

func (s *editor) exportPNG(ctx *app.Context) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, s.layer); err != nil {
		ctx.Fail("Не удалось выгрузить страницу", err)
		return
	}
	path, err := ctx.Storage().Export(exportName(s.book, "png", ctx.Now()), buf.Bytes())
	if err != nil {
		ctx.Fail("Не удалось выгрузить страницу", err)
		return
	}
	ctx.Toast("Сохранено: " + path)
}

func (s *editor) sheet(p *PageRef) ink.Sheet {
	b := s.book
	return ink.Sheet{Width: b.Width, Height: b.Height, Strokes: p.strokes,
		Ruling: ruling(b.template(p), b.Width, b.Height, b.DPI)}
}

func (s *editor) exportSVG(ctx *app.Context) {
	var buf bytes.Buffer
	if err := ink.WriteSVG(&buf, s.sheet(s.page)); err != nil {
		ctx.Fail("Не удалось выгрузить страницу", err)
		return
	}
	path, err := ctx.Storage().Export(exportName(s.book, "svg", ctx.Now()), buf.Bytes())
	if err != nil {
		ctx.Fail("Не удалось выгрузить страницу", err)
		return
	}
	ctx.Toast("Сохранено: " + path)
}

// exportPDF выгружает тетрадь целиком: страница тетради — страница PDF. Читает страницы
// и пишет файл в фоне.
func exportPDF(ctx *app.Context, b *Book) {
	st := ctx.Storage()
	// Описание копируется: тетрадь в редакторе продолжают менять, пока идёт выгрузка.
	snapshot := *b
	pages := make([]*PageRef, len(b.Pages))
	for i, p := range b.Pages {
		cp := *p
		cp.strokes = append([]*ink.Stroke(nil), p.strokes...)
		pages[i] = &cp
	}
	snapshot.Pages = pages
	name := exportName(b, "pdf", ctx.Now())
	ctx.Toast("Выгружаем тетрадь…")
	kit.Load(ctx, func(context.Context) (string, error) {
		return writePDF(st, &snapshot, name)
	}, func(path string, err error) {
		if err != nil {
			ctx.Fail("Не удалось выгрузить тетрадь", err, "book", b.ID)
			return
		}
		ctx.Toast("Сохранено: " + path)
	})
}

func writePDF(st *storage.App, b *Book, name string) (string, error) {
	sheets := make([]ink.Sheet, 0, len(b.Pages))
	for _, p := range b.Pages {
		if err := b.load(st, p); err != nil {
			return "", err
		}
		sheets = append(sheets, ink.Sheet{Width: b.Width, Height: b.Height, Strokes: p.strokes,
			Ruling: ruling(b.template(p), b.Width, b.Height, b.DPI)})
	}
	var buf bytes.Buffer
	if err := ink.WritePDF(&buf, sheets, b.DPI); err != nil {
		return "", err
	}
	return st.Export(name, buf.Bytes())
}

// pageImage рисует страницу тетради в новом изображении: для экрана сна и миниатюр.
func pageImage(st *storage.App, b *Book, p *PageRef) (*image.Gray, error) {
	if err := b.load(st, p); err != nil {
		return nil, err
	}
	img := image.NewGray(image.Rect(0, 0, b.Width, b.Height))
	render(img, img.Bounds(), p.strokes, ruling(b.template(p), b.Width, b.Height, b.DPI))
	return img, nil
}
