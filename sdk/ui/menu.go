package ui

import (
	"image"

	"el-book/sdk/gfx"
	"el-book/sdk/icons"
)

// MenuItem — пункт меню.
type MenuItem struct {
	Text     string
	Icon     icons.Drawer
	Disabled bool
	Checked  bool // пункт выбран: справа галочка
	OnTap    func()
}

// NewMenu создаёт меню действий: карточку со строками по центру экрана. Тап по пункту
// закрывает меню и выполняет действие, тап мимо — просто закрывает.
func NewMenu(t *Theme, title string, items ...MenuItem) *Panel {
	p := &Panel{Title: title, Dismissable: true}
	for _, it := range items {
		it := it
		p.Rows = append(p.Rows, Row{W: &menuRow{item: it, onTap: func() {
			p.Close()
			if it.OnTap != nil {
				it.OnTap()
			}
		}}})
	}
	return p
}

// NewChoice создаёт меню выбора одного варианта: выбранный отмечен галочкой.
func NewChoice(t *Theme, title string, options []string, selected int, choose func(i int)) *Panel {
	items := make([]MenuItem, len(options))
	for i, o := range options {
		i := i
		items[i] = MenuItem{Text: o, Checked: i == selected, OnTap: func() { choose(i) }}
	}
	return NewMenu(t, title, items...)
}

// menuRow — строка меню: иконка и текст слева, галочка справа.
type menuRow struct {
	Box
	item  MenuItem
	onTap func()
}

func (m *menuRow) Enabled() bool         { return !m.item.Disabled }
func (m *menuRow) Tap(Host, image.Point) { m.onTap() }

func (m *menuRow) Draw(c *gfx.Canvas, t *Theme) {
	r := Pad(m.Bounds(), t.MM(1), 0, t.MM(1), 0)
	color := gfx.Black
	if m.item.Disabled {
		color = gfx.Gray16(8)
	}
	if m.item.Icon != nil {
		ic := CutLeft(&r, t.IconSize())
		m.item.Icon(c, Center(ic, t.IconSize(), t.IconSize()), color)
		CutLeft(&r, t.MM(3))
	}
	if m.item.Checked {
		ch := CutRight(&r, t.IconSize())
		icons.Check(c, Center(ch, t.IconSize(), t.IconSize()), color)
	}
	t.Body().DrawIn(c, m.item.Text, r, gfx.AlignLeft, color)
}
