package kit

import (
	"image"
	"strings"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ui"
)

// ReaderScreen — экран чтения: строка заголовка, текст со страницами, номер страницы внизу.
// Листается свайпом, тапом по краю текста и кнопками листания. Для статьи, новости, заметки.
type ReaderScreen struct {
	Title    string
	Subtitle string
	Blocks   []ui.Block
	Size     float64      // кегль; 0 — 13 pt
	Actions  []*ui.Button // кнопки справа в строке заголовка
	// Bottom — виджет под текстом: «Читать полностью», «Изменить»; nil — нет.
	Bottom ui.Widget
	// Root — первый экран приложения: кнопка-домик уводит на домашний экран.
	Root bool

	page   ui.Page
	bar    *ui.TitleBar
	reader *ui.Reader
	pager  *ui.Label
}

// SetBlocks заменяет текст: статья догрузилась.
func (s *ReaderScreen) SetBlocks(h ui.Host, bs []ui.Block) {
	s.Blocks = bs
	if s.reader == nil {
		return
	}
	s.reader.SetBlocks(bs)
	s.updatePager(h)
	h.Invalidate(s.reader.R, eink.Content)
}

// Bar — строка заголовка: чтобы сменить заголовок или подзаголовок.
func (s *ReaderScreen) Bar() *ui.TitleBar { return s.bar }

func (s *ReaderScreen) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	s.page.Clear()
	r := b
	back := func() { ctx.Pop() }
	if s.Root {
		back = func() { ctx.Home() }
	}
	s.bar = ui.NewTitleBar(t, s.Title, back, s.Actions...)
	s.bar.Subtitle = s.Subtitle
	if s.Root {
		s.bar.BackIcon = icons.Home
	}
	s.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	s.page.Add(s.bar)
	if s.Bottom != nil {
		s.Bottom.SetBounds(ui.Pad(ui.CutBottom(&r, t.Touch()+t.MM(3)), t.Pad(), t.MM(1), t.Pad(), t.MM(2)))
		s.page.Add(s.Bottom)
	}
	s.pager = &ui.Label{Align: gfx.AlignCenter, Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}
	s.pager.SetBounds(ui.CutBottom(&r, t.Caption().Height+t.MM(2)))
	page := 0
	if s.reader != nil {
		page = s.reader.Page()
	}
	s.reader = ui.NewReader(t)
	s.reader.Size = s.Size
	s.reader.OnPage = func(int, int) { s.updatePager(ctx) }
	s.reader.Blocks = s.Blocks
	s.reader.SetBounds(ui.Pad(r, t.Pad(), t.MM(4), t.Pad(), t.MM(2)))
	s.reader.SetPage(ctx, page)
	s.page.Add(s.reader, s.pager)
	s.updatePager(ctx)
}

func (s *ReaderScreen) updatePager(h ui.Host) {
	if s.pager == nil {
		return
	}
	s.pager.Hidden = s.reader.Pages() < 2
	s.pager.Text = s.reader.PageText()
	h.Invalidate(s.pager.R, eink.UI)
}

func (s *ReaderScreen) Draw(c *gfx.Canvas, t *ui.Theme) { s.page.Draw(c, t) }

func (s *ReaderScreen) Handle(ctx *app.Context, ev app.Event) {
	if k, ok := ev.(device.KeyEvent); ok && PageKeys(ctx, k, s.reader.Next, s.reader.Prev) {
		return
	}
	app.RoutePage(ctx, &s.page, ev)
}

// Paragraphs делит простой текст на абзацы по пустым строкам и переводам строк.
func Paragraphs(text string) []ui.Block {
	var out []ui.Block
	for _, p := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, ui.Block{Kind: ui.Para, Text: p})
		}
	}
	return out
}
