package ui

import (
	"image"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
)

// Bar — высота строки заголовка экрана.
func (t *Theme) Bar() int { return t.Touch() + t.MM(3) }

// TitleBar — строка заголовка: кнопка «назад» слева, заголовок и кнопки действий справа,
// тонкая черта снизу. Одинакова во всех приложениях: путь назад всегда в одном месте.
type TitleBar struct {
	Box
	Title    string
	Subtitle string       // мелко под заголовком: «3 / 12», «обновлено в 14:05»
	BackIcon icons.Drawer // по умолчанию стрелка; домик — для первого экрана приложения
	NoLine   bool

	theme   *Theme
	back    *Button
	actions []*Button
}

// NewTitleBar создаёт строку заголовка. back = nil — без кнопки «назад». Действия —
// кнопки справа, слева направо; у кнопки с иконкой без текста ширина — одна цель касания.
func NewTitleBar(t *Theme, title string, back func(), actions ...*Button) *TitleBar {
	b := &TitleBar{Title: title, theme: t, actions: actions}
	if back != nil {
		b.back = &Button{Icon: icons.Back, Style: Flat, OnTap: back}
	}
	for _, a := range actions {
		if a.Style == Secondary && a.Text == "" {
			a.Style = Flat
		}
	}
	return b
}

// Actions — кнопки справа: их можно делать активными и неактивными.
func (b *TitleBar) Actions() []*Button { return b.actions }

func (b *TitleBar) SetBounds(r image.Rectangle) {
	b.R = r
	t := b.theme
	row := Pad(r, t.Pad()-t.MM(1), 0, t.Pad()-t.MM(1), 0)
	row = Center(row, row.Dx(), t.Touch())
	if b.back != nil {
		if b.BackIcon != nil {
			b.back.Icon = b.BackIcon
		}
		b.back.SetBounds(CutLeft(&row, t.Touch()))
	}
	for i := len(b.actions) - 1; i >= 0; i-- {
		a := b.actions[i]
		w := t.Touch()
		if a.Text != "" {
			w = t.Body().Measure(a.Text) + 2*t.MM(3)
			if a.Icon != nil {
				w += t.IconSize() + t.MM(1.5)
			}
		}
		a.SetBounds(CutRight(&row, w))
		if i > 0 {
			CutRight(&row, t.MM(1))
		}
	}
}

func (b *TitleBar) Children() []Widget {
	out := make([]Widget, 0, len(b.actions)+1)
	if b.back != nil {
		out = append(out, b.back)
	}
	for _, a := range b.actions {
		out = append(out, a)
	}
	return out
}

// titleRect — где пишется заголовок: между кнопкой «назад» и действиями.
func (b *TitleBar) titleRect(t *Theme) image.Rectangle {
	r := Pad(b.R, t.Pad(), 0, t.Pad(), t.Line())
	if b.back != nil {
		r.Min.X = b.back.R.Max.X + t.MM(2)
	}
	if len(b.actions) > 0 {
		r.Max.X = b.actions[0].R.Min.X - t.MM(2)
	}
	return r
}

// SetTitle меняет заголовок и перерисовывает его.
func (b *TitleBar) SetTitle(h Host, title, subtitle string) {
	if title == b.Title && subtitle == b.Subtitle {
		return
	}
	b.Title, b.Subtitle = title, subtitle
	h.Invalidate(b.titleRect(h.Theme()), eink.UI)
}

func (b *TitleBar) Draw(c *gfx.Canvas, t *Theme) {
	r := b.titleRect(t)
	title := t.Title()
	if b.Subtitle == "" {
		title.DrawIn(c, b.Title, r, gfx.AlignLeft, gfx.Black)
	} else {
		sub := t.Caption()
		h := title.Height + sub.Height
		y := r.Min.Y + (r.Dy()-h)/2
		title.DrawIn(c, b.Title, image.Rect(r.Min.X, y, r.Max.X, y+title.Height), gfx.AlignLeft, gfx.Black)
		sub.DrawIn(c, b.Subtitle, image.Rect(r.Min.X, y+title.Height, r.Max.X, y+h), gfx.AlignLeft, gfx.Gray16(5))
	}
	if !b.NoLine {
		c.Fill(image.Rect(b.R.Min.X, b.R.Max.Y-t.Line(), b.R.Max.X, b.R.Max.Y), gfx.Black)
	}
}
