package ui

import (
	"fmt"
	"image"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
)

// ListItem — строка списка.
type ListItem struct {
	Title    string
	Subtitle string       // вторая строка, мельче и серым
	Icon     icons.Drawer // слева
	Detail   string       // справа: значение, счётчик, время
	Trailing icons.Drawer // справа после Detail: «›», галочка, флажок
	Muted    bool         // заголовок серым: недоступно или выполнено
	Strike   bool         // заголовок зачёркнут: выполненная задача
	Bold     bool         // заголовок жирным: непрочитанное

	OnTap       func()
	OnLongPress func()
}

// List — список со страницами: прокрутки нет, длинный список листается свайпом и кнопками
// листания (List.Next, List.Prev). Номер страницы «2 / 5» список пишет внизу сам.
type List struct {
	Box
	Items []ListItem
	// Empty — что сказать, если строк нет; пустой список без пояснения не допускается.
	Empty string
	// RowHeight — высота строки; 0 — по содержимому (в одну или две строки текста).
	RowHeight int
	// OnPage сообщает о смене страницы.
	OnPage func(page, pages int)

	theme   *Theme
	page    int
	perPage int
	rowH    int
	rows    []*listRow
	footer  image.Rectangle
}

// NewList создаёт пустой список.
func NewList(t *Theme) *List { return &List{theme: t} }

// SetItems заменяет строки; страница сохраняется, если она ещё есть. Перерисовку просит
// вызывающий (Refresh) — так несколько изменений дают одно обновление.
func (l *List) SetItems(items []ListItem) {
	l.Items = items
	l.layout()
}

// Refresh просит перерисовать список.
func (l *List) Refresh(h Host) { h.Invalidate(l.R, eink.UI) }

func (l *List) SetBounds(r image.Rectangle) {
	l.R = r
	l.layout()
}

func (l *List) rowHeight() int {
	t := l.theme
	if l.RowHeight > 0 {
		return l.RowHeight
	}
	two := false
	for _, it := range l.Items {
		if it.Subtitle != "" {
			two = true
			break
		}
	}
	if two {
		return t.Body().Height + t.Caption().Height + t.MM(4)
	}
	return max(t.Touch(), t.Body().Height+t.MM(4))
}

func (l *List) layout() {
	t := l.theme
	l.rowH = l.rowHeight()
	area := l.R
	l.perPage = max(area.Dy()/max(l.rowH, 1), 1)
	l.footer = image.Rectangle{}
	if len(l.Items) > l.perPage {
		l.footer = CutBottom(&area, t.Caption().Height+t.MM(2))
		l.perPage = max(area.Dy()/max(l.rowH, 1), 1)
	}
	l.page = min(max(l.page, 0), l.Pages()-1)
	l.rows = l.rows[:0]
	from := l.page * l.perPage
	for i := from; i < min(from+l.perPage, len(l.Items)); i++ {
		row := &listRow{list: l, index: i}
		y := area.Min.Y + (i-from)*l.rowH
		row.R = image.Rect(area.Min.X, y, area.Max.X, y+l.rowH)
		l.rows = append(l.rows, row)
	}
}

// Pages — сколько страниц.
func (l *List) Pages() int {
	return max((len(l.Items)+max(l.perPage, 1)-1)/max(l.perPage, 1), 1)
}

// Page — текущая страница с нуля.
func (l *List) Page() int { return l.page }

// PerPage — сколько строк помещается на страницу.
func (l *List) PerPage() int { return l.perPage }

// SetPage переходит на страницу n. Возвращает false, если такой страницы нет или она уже открыта.
func (l *List) SetPage(h Host, n int) bool {
	if n < 0 || n >= l.Pages() || n == l.page {
		return false
	}
	l.page = n
	l.layout()
	if l.OnPage != nil {
		l.OnPage(l.page, l.Pages())
	}
	l.Refresh(h)
	return true
}

// Show открывает страницу, на которой строка index.
func (l *List) Show(h Host, index int) {
	if index >= 0 && l.perPage > 0 {
		l.SetPage(h, index/l.perPage)
	}
}

// Next и Prev листают страницы — для кнопок листания.
func (l *List) Next(h Host) bool { return l.SetPage(h, l.page+1) }
func (l *List) Prev(h Host) bool { return l.SetPage(h, l.page-1) }

func (l *List) Swipe(h Host, dir Direction) bool {
	switch dir {
	case DirLeft, DirUp:
		return l.Next(h)
	case DirRight, DirDown:
		return l.Prev(h)
	}
	return false
}

func (l *List) Children() []Widget {
	out := make([]Widget, len(l.rows))
	for i, r := range l.rows {
		out[i] = r
	}
	return out
}

func (l *List) Draw(c *gfx.Canvas, t *Theme) {
	if len(l.Items) == 0 {
		if l.Empty != "" {
			lbl := Label{Text: l.Empty, Wrap: true, Align: gfx.AlignCenter, Color: gfx.Gray16(5)}
			lbl.SetBounds(Pad(l.R, t.MM(4), 0, t.MM(4), 0))
			lbl.Draw(c, t)
		}
		return
	}
	if !l.footer.Empty() {
		text := fmt.Sprintf("%d / %d", l.page+1, l.Pages())
		t.Caption().DrawIn(c, text, l.footer, gfx.AlignCenter, gfx.Gray16(5))
	}
}

// listRow — видимая строка: виджет, который Page инвертирует при нажатии.
type listRow struct {
	Box
	list  *List
	index int
}

func (r *listRow) item() *ListItem { return &r.list.Items[r.index] }

func (r *listRow) Enabled() bool { return r.item().OnTap != nil }

func (r *listRow) Tap(Host, image.Point) {
	if f := r.item().OnTap; f != nil {
		f()
	}
}

func (r *listRow) LongPress(Host, image.Point) {
	if f := r.item().OnLongPress; f != nil {
		f()
	}
}

func (r *listRow) Draw(c *gfx.Canvas, t *Theme) {
	it := r.item()
	b := r.Bounds()
	inner := Pad(b, t.MM(2), 0, t.MM(1), 0)
	if it.Icon != nil {
		ic := CutLeft(&inner, t.IconSize())
		it.Icon(c, Center(ic, t.IconSize(), t.IconSize()), gfx.Black)
		CutLeft(&inner, t.MM(3))
	}
	if it.Trailing != nil {
		tr := CutRight(&inner, t.IconSize())
		color := gfx.Black
		if it.Muted {
			color = gfx.Gray16(7)
		}
		it.Trailing(c, Center(tr, t.IconSize(), t.IconSize()), color)
		CutRight(&inner, t.MM(2))
	}
	if it.Detail != "" {
		face := t.Body()
		w := min(face.Measure(it.Detail), inner.Dx()/2)
		d := CutRight(&inner, w)
		face.DrawIn(c, it.Detail, d, gfx.AlignRight, gfx.Gray16(4))
		CutRight(&inner, t.MM(3))
	}
	title := t.Body()
	if it.Bold {
		title = t.Strong()
	}
	color := gfx.Black
	if it.Muted {
		color = gfx.Gray16(7)
	}
	tr := inner
	if it.Subtitle != "" {
		sub := t.Caption()
		h := title.Height + sub.Height + t.MM(0.5)
		y := inner.Min.Y + (inner.Dy()-h)/2
		tr = image.Rect(inner.Min.X, y, inner.Max.X, y+title.Height)
		sr := image.Rect(inner.Min.X, tr.Max.Y+t.MM(0.5), inner.Max.X, tr.Max.Y+t.MM(0.5)+sub.Height)
		sub.DrawIn(c, it.Subtitle, sr, gfx.AlignLeft, gfx.Gray16(5))
	}
	title.DrawIn(c, it.Title, tr, gfx.AlignLeft, color)
	if it.Strike {
		w := min(title.Measure(title.Ellipsize(it.Title, tr.Dx())), tr.Dx())
		y := tr.Min.Y + tr.Dy()/2
		c.Fill(image.Rect(tr.Min.X, y-t.Line()/2, tr.Min.X+w, y-t.Line()/2+max(t.Line()/2, 2)), color)
	}
	last := r.index == len(r.list.Items)-1 || r.index%r.list.perPage == r.list.perPage-1
	if !last {
		c.Fill(image.Rect(b.Min.X+t.MM(2), b.Max.Y-1, b.Max.X-t.MM(1), b.Max.Y), gfx.Gray16(11))
	}
}
