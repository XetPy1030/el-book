package ui

import (
	"image"
	"strings"
	"unicode"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
)

// TextField — однострочное поле ввода. Печатает в него Keyboard: поле — её KeyTarget.
// Касание ставит курсор; длинный текст сдвигается так, чтобы курсор был виден.
type TextField struct {
	Box
	Placeholder string
	Secret      bool // пароль: вместо букв точки
	OnChange    func(text string)
	OnSubmit    func(text string) // клавиша ввода
	// NoCaps — не начинать с заглавной: адреса, логины, поиск.
	NoCaps bool

	text   []rune
	cursor int
	scroll int // на сколько пикселей текст сдвинут влево
}

// Text — введённый текст.
func (f *TextField) Text() string { return string(f.text) }

// SetText заменяет текст; курсор — в конец.
func (f *TextField) SetText(s string) {
	f.text = []rune(s)
	f.cursor = len(f.text)
}

func (f *TextField) face(t *Theme) *gfx.Face { return t.Face(gfx.Regular, 13) }

// Height — высота поля.
func (f *TextField) Height(t *Theme, _ int) int { return f.face(t).Height + t.MM(5) }

func (f *TextField) shown() []rune {
	if !f.Secret {
		return f.text
	}
	return []rune(strings.Repeat("•", len(f.text)))
}

func (f *TextField) changed(h Host) {
	if f.OnChange != nil {
		f.OnChange(string(f.text))
	}
	h.Invalidate(f.R, eink.UI)
}

// Insert вставляет текст в позицию курсора.
func (f *TextField) Insert(h Host, s string) {
	s = strings.ReplaceAll(s, "\n", " ")
	rs := []rune(s)
	f.text = append(f.text[:f.cursor], append(rs, f.text[f.cursor:]...)...)
	f.cursor += len(rs)
	f.changed(h)
}

// Backspace стирает символ или слово перед курсором.
func (f *TextField) Backspace(h Host, word bool) {
	if f.cursor == 0 {
		return
	}
	from := f.cursor - 1
	if word {
		from = wordStart(f.text, f.cursor)
	}
	f.text = append(f.text[:from], f.text[f.cursor:]...)
	f.cursor = from
	f.changed(h)
}

// Enter отдаёт текст OnSubmit.
func (f *TextField) Enter(Host) {
	if f.OnSubmit != nil {
		f.OnSubmit(string(f.text))
	}
}

// WantCaps — заглавная нужна в пустом поле, если поле не просит обратного.
func (f *TextField) WantCaps() bool { return !f.NoCaps && !f.Secret && len(f.text) == 0 }

// wordStart — где начинается слово перед позицией i: пробелы перед словом стираются вместе с ним.
func wordStart(text []rune, i int) int {
	for i > 0 && unicode.IsSpace(text[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(text[i-1]) {
		i--
	}
	return i
}

func (f *TextField) Enabled() bool    { return true }
func (f *TextField) ShowsPress() bool { return false }

// Tap ставит курсор к ближайшей границе букв.
func (f *TextField) Tap(h Host, p image.Point) {
	t := h.Theme()
	inner := f.inner(t)
	offs := f.face(t).Offsets(f.shown())
	x := p.X - inner.Min.X + f.scroll
	best := 0
	for i, o := range offs {
		if abs(o-x) < abs(offs[best]-x) {
			best = i
		}
	}
	if best != f.cursor {
		f.cursor = best
		h.Invalidate(f.R, eink.UI)
	}
}

func (f *TextField) inner(t *Theme) image.Rectangle {
	return Pad(f.R, t.MM(2.5), 0, t.MM(2.5), 0)
}

func (f *TextField) Draw(c *gfx.Canvas, t *Theme) {
	r := f.Bounds()
	c.StrokeRoundRect(r, t.Radius(), t.Line(), gfx.Black)
	inner := f.inner(t)
	face := f.face(t)
	if len(f.text) == 0 && f.Placeholder != "" {
		face.DrawIn(c, f.Placeholder, inner, gfx.AlignLeft, gfx.Gray16(8))
	}
	offs := face.Offsets(f.shown())
	cx := offs[f.cursor]
	margin := t.MM(2)
	switch {
	case cx-f.scroll > inner.Dx()-margin:
		f.scroll = cx - inner.Dx() + margin
	case cx-f.scroll < 0:
		f.scroll = max(cx-margin, 0)
	}
	if offs[len(offs)-1]-f.scroll < inner.Dx()-margin {
		f.scroll = max(offs[len(offs)-1]-inner.Dx()+margin, 0)
	}
	baseline := inner.Min.Y + (inner.Dy()+face.Ascent-face.Descent)/2
	face.Draw(c.WithClip(inner), string(f.shown()), inner.Min.X-f.scroll, baseline, gfx.Black)
	x := inner.Min.X + cx - f.scroll
	c.WithClip(inner.Inset(-t.MM(0.5))).Fill(image.Rect(x-1, baseline-face.Ascent, x+max(t.Line()/2, 2)-1, baseline+face.Descent), gfx.Black)
}
