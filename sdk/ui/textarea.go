package ui

import (
	"fmt"
	"image"
	"strings"
	"unicode"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
)

// TextArea — многострочный редактор со страницами: длинный текст не прокручивается, а
// листается (свайп, TextArea.Next и Prev для кнопок листания). Печатает в него Keyboard.
// После правки перерисовываются только изменившиеся строки: печать в конце текста обновляет
// одну строку, а не всю страницу.
type TextArea struct {
	Box
	Placeholder string
	OnChange    func(text string)
	OnPage      func(page, pages int)
	ReadOnly    bool // только просмотр: курсора нет

	theme  *Theme
	pt     float64
	text   []rune
	cursor int
	page   int
	lines  []textLine
}

// textLine — строка после переноса: руны [start, end) без перевода строки.
type textLine struct{ start, end int }

// NewTextArea создаёт редактор с кеглем pt (0 — 13 pt).
func NewTextArea(t *Theme, pt float64) *TextArea {
	if pt <= 0 {
		pt = 13
	}
	return &TextArea{theme: t, pt: pt}
}

func (a *TextArea) face() *gfx.Face { return a.theme.Face(gfx.Regular, a.pt) }

// Text — текст целиком.
func (a *TextArea) Text() string { return string(a.text) }

// SetText заменяет текст; курсор — в конец, страница — с курсором.
func (a *TextArea) SetText(s string) {
	a.text = []rune(s)
	a.cursor = len(a.text)
	a.rewrap()
	a.page = a.pageOf(a.cursor)
}

// Cursor — позиция курсора в рунах.
func (a *TextArea) Cursor() int { return a.cursor }

func (a *TextArea) SetBounds(r image.Rectangle) {
	a.R = r
	a.rewrap()
	a.page = min(a.page, a.Pages()-1)
}

func (a *TextArea) lineHeight() int { return a.face().Height }

func (a *TextArea) perPage() int { return max(a.R.Dy()/max(a.lineHeight(), 1), 1) }

func (a *TextArea) rewrap() {
	f := a.face()
	a.lines = wrapRunes(a.text, a.R.Dx()-a.theme.MM(1), f.Advance)
}

// Pages — сколько страниц.
func (a *TextArea) Pages() int { return max((len(a.lines)+a.perPage()-1)/a.perPage(), 1) }

// Page — текущая страница с нуля.
func (a *TextArea) Page() int { return a.page }

// PageText — «2 / 5»; пусто, если страница одна.
func (a *TextArea) PageText() string {
	if a.Pages() < 2 {
		return ""
	}
	return fmt.Sprintf("%d / %d", a.page+1, a.Pages())
}

// lineOf — строка, на которой стоит позиция i: последняя строка, начинающаяся не позже i.
func (a *TextArea) lineOf(i int) int {
	n := 0
	for k, l := range a.lines {
		if l.start <= i {
			n = k
		}
	}
	return n
}

func (a *TextArea) pageOf(i int) int { return a.lineOf(i) / a.perPage() }

// SetPage открывает страницу n.
func (a *TextArea) SetPage(h Host, n int) bool {
	if n < 0 || n >= a.Pages() || n == a.page {
		return false
	}
	a.page = n
	if a.OnPage != nil {
		a.OnPage(a.page, a.Pages())
	}
	h.Invalidate(a.R, eink.UI)
	return true
}

// Next и Prev листают страницы.
func (a *TextArea) Next(h Host) bool { return a.SetPage(h, a.page+1) }
func (a *TextArea) Prev(h Host) bool { return a.SetPage(h, a.page-1) }

func (a *TextArea) Swipe(h Host, dir Direction) bool {
	switch dir {
	case DirLeft, DirUp:
		return a.Next(h)
	case DirRight, DirDown:
		return a.Prev(h)
	}
	return false
}

// visible — тексты строк текущей страницы: по ним после правки ищутся изменившиеся строки.
func (a *TextArea) visible() []string {
	per := a.perPage()
	from := a.page * per
	out := make([]string, 0, per)
	for i := from; i < min(from+per, len(a.lines)); i++ {
		out = append(out, string(a.text[a.lines[i].start:a.lines[i].end]))
	}
	return out
}

// edit применяет правку и перерисовывает то, что изменилось.
func (a *TextArea) edit(h Host, f func()) {
	oldPage, oldLine := a.page, a.lineOf(a.cursor)
	old := a.visible()
	f()
	a.rewrap()
	a.page = a.pageOf(a.cursor)
	if a.OnChange != nil {
		a.OnChange(string(a.text))
	}
	if a.page != oldPage {
		if a.OnPage != nil {
			a.OnPage(a.page, a.Pages())
		}
		h.Invalidate(a.R, eink.UI)
		return
	}
	per := a.perPage()
	now := a.visible()
	dirty := image.Rectangle{}
	for i := 0; i < max(len(old), len(now)); i++ {
		var o, n string
		if i < len(old) {
			o = old[i]
		}
		if i < len(now) {
			n = now[i]
		}
		if o != n {
			dirty = dirty.Union(a.lineRect(a.page*per + i))
		}
	}
	dirty = dirty.Union(a.lineRect(oldLine)).Union(a.lineRect(a.lineOf(a.cursor)))
	h.Invalidate(dirty.Intersect(a.R), eink.UI)
}

// lineRect — где на экране строка k текущей страницы (пусто, если она на другой странице).
func (a *TextArea) lineRect(k int) image.Rectangle {
	per := a.perPage()
	if k/per != a.page {
		return image.Rectangle{}
	}
	y := a.R.Min.Y + (k%per)*a.lineHeight()
	return image.Rect(a.R.Min.X, y, a.R.Max.X, y+a.lineHeight())
}

// Insert вставляет текст в позицию курсора.
func (a *TextArea) Insert(h Host, s string) {
	if a.ReadOnly {
		return
	}
	a.edit(h, func() {
		rs := []rune(s)
		a.text = append(a.text[:a.cursor], append(rs, a.text[a.cursor:]...)...)
		a.cursor += len(rs)
	})
}

// Backspace стирает символ или слово перед курсором.
func (a *TextArea) Backspace(h Host, word bool) {
	if a.ReadOnly || a.cursor == 0 {
		return
	}
	a.edit(h, func() {
		from := a.cursor - 1
		if word {
			from = wordStart(a.text, a.cursor)
		}
		a.text = append(a.text[:from], a.text[a.cursor:]...)
		a.cursor = from
	})
}

// Enter переводит строку.
func (a *TextArea) Enter(h Host) { a.Insert(h, "\n") }

// WantCaps — курсор в начале текста, абзаца или предложения.
func (a *TextArea) WantCaps() bool {
	before := strings.TrimRightFunc(string(a.text[:a.cursor]), func(r rune) bool { return r == ' ' })
	if before == "" || strings.HasSuffix(string(a.text[:a.cursor]), "\n") {
		return true
	}
	if len(before) == len(string(a.text[:a.cursor])) {
		return false // курсор сразу после буквы
	}
	last := []rune(before)
	switch last[len(last)-1] {
	case '.', '!', '?', '…':
		return true
	}
	return false
}

func (a *TextArea) Enabled() bool    { return !a.ReadOnly }
func (a *TextArea) ShowsPress() bool { return false }

// Tap ставит курсор в ближайшую к касанию позицию текущей страницы.
func (a *TextArea) Tap(h Host, p image.Point) {
	per := a.perPage()
	k := a.page*per + (p.Y-a.R.Min.Y)/a.lineHeight()
	if k >= len(a.lines) {
		k = len(a.lines) - 1
	}
	if k < a.page*per {
		k = a.page * per
	}
	l := a.lines[k]
	offs := a.face().Offsets(a.text[l.start:l.end])
	x := p.X - a.R.Min.X
	best := 0
	for i, o := range offs {
		if abs(o-x) < abs(offs[best]-x) {
			best = i
		}
	}
	old := a.lineOf(a.cursor)
	a.cursor = l.start + best
	h.Invalidate(a.lineRect(old).Union(a.lineRect(k)), eink.UI)
}

func (a *TextArea) Draw(c *gfx.Canvas, t *Theme) {
	face := a.face()
	per := a.perPage()
	from := a.page * per
	if len(a.text) == 0 && a.Placeholder != "" {
		face.Draw(c, a.Placeholder, a.R.Min.X, a.R.Min.Y+face.Ascent, gfx.Gray16(8))
	}
	for i := from; i < min(from+per, len(a.lines)); i++ {
		l := a.lines[i]
		y := a.R.Min.Y + (i-from)*a.lineHeight()
		if l.end > l.start {
			face.Draw(c, string(a.text[l.start:l.end]), a.R.Min.X, y+face.Ascent, gfx.Black)
		}
		if !a.ReadOnly && a.lineOf(a.cursor) == i {
			offs := face.Offsets(a.text[l.start:a.cursor])
			x := a.R.Min.X + offs[len(offs)-1]
			c.Fill(image.Rect(x-1, y+t.MM(0.3), x+max(t.Line()/2, 2)-1, y+face.Ascent+face.Descent), gfx.Black)
		}
	}
}

// wrapRunes разбивает текст на строки не шире width по словам. Пробел, на котором перенесли,
// остаётся в конце строки и не виден; слово длиннее строки режется по буквам. adv — ширина
// руны после предыдущей (prev < 0 — начало строки).
func wrapRunes(text []rune, width int, adv func(prev, r rune) float64) []textLine {
	var lines []textLine
	start := 0
	for {
		end := start
		for end < len(text) && text[end] != '\n' {
			end++
		}
		lineStart, lastSpace := start, -1
		x, prev := 0.0, rune(-1)
		for i := start; i < end; {
			r := text[i]
			w := adv(prev, r)
			if !unicode.IsSpace(r) && x+w > float64(width) && i > lineStart {
				cut := i
				if lastSpace >= lineStart {
					cut = lastSpace + 1
				}
				lines = append(lines, textLine{lineStart, cut})
				lineStart, lastSpace = cut, -1
				x, prev = 0, -1
				for j := lineStart; j < i; j++ {
					x += adv(prev, text[j])
					prev = text[j]
				}
				continue
			}
			if r == ' ' {
				lastSpace = i
			}
			x += w
			prev = r
			i++
		}
		lines = append(lines, textLine{lineStart, end})
		if end >= len(text) {
			return lines
		}
		start = end + 1
	}
}
