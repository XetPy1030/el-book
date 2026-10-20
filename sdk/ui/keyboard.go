package ui

import (
	"image"
	"strings"
	"time"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
)

// KeyTarget — получатель ввода с экранной клавиатуры: однострочное поле или редактор.
type KeyTarget interface {
	Insert(h Host, s string)
	// Backspace стирает символ перед курсором; word — слово целиком.
	Backspace(h Host, word bool)
	// Enter — клавиша ввода: в редакторе перевод строки, в поле — «готово».
	Enter(h Host)
}

// CapsHinter — получатель знает, что курсор в начале предложения и нужна заглавная буква.
type CapsHinter interface {
	WantCaps() bool
}

type keyKind uint8

const (
	kChar  keyKind = iota
	kShift         // заглавные: одна или все
	kBack          // стереть
	kMode          // буквы ↔ цифры и знаки
	kMore          // первая ↔ вторая страница знаков
	kLang          // русская ↔ английская раскладка
	kSpace
	kEnter
)

type keyDef struct {
	kind  keyKind
	ch    string  // символ клавиши kChar
	alt   string  // символ по долгому нажатию
	units float64 // ширина в долях обычной клавиши
}

type kbLayout struct {
	name string // пишется на пробеле
	rows [][]keyDef
}

func chars(s string, alts map[string]string) []keyDef {
	var out []keyDef
	for _, r := range s {
		ch := string(r)
		out = append(out, keyDef{kind: kChar, ch: ch, alt: alts[ch], units: 1})
	}
	return out
}

func row(parts ...[]keyDef) []keyDef {
	var out []keyDef
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func special(k keyKind, units float64) []keyDef { return []keyDef{{kind: k, units: units}} }

// bottomRow — нижний ряд всех раскладок; ширина подобрана под ряд из n обычных клавиш.
func bottomRow(n float64) []keyDef {
	side := 1.5
	if n < 12 {
		side = 1.25
	}
	space := n - 2*side - 2 - 2
	return row(special(kMode, side), special(kLang, side), chars(",", nil),
		special(kSpace, space), chars(".", nil), special(kEnter, 2))
}

var enDigits = map[string]string{"q": "1", "w": "2", "e": "3", "r": "4", "t": "5", "y": "6", "u": "7", "i": "8", "o": "9", "p": "0"}

// Раскладки: русская ЙЦУКЕН, английская QWERTY и две страницы знаков. Долгое нажатие
// на «е» даёт «ё», на «ь» — «ъ»; в английской верхний ряд даёт цифры.
var (
	layoutRU = kbLayout{name: "Русский", rows: [][]keyDef{
		chars("йцукенгшщзхъ", map[string]string{"е": "ё"}),
		chars("фывапролджэ", nil),
		row(special(kShift, 1.5), chars("ячсмитьбю", map[string]string{"ь": "ъ"}), special(kBack, 1.5)),
		bottomRow(12),
	}}
	layoutEN = kbLayout{name: "English", rows: [][]keyDef{
		chars("qwertyuiop", enDigits),
		chars("asdfghjkl", nil),
		row(special(kShift, 1.5), chars("zxcvbnm", nil), special(kBack, 1.5)),
		bottomRow(10),
	}}
	layoutSym = kbLayout{rows: [][]keyDef{
		chars("1234567890", nil),
		chars("-/:;()₽&@\"", nil),
		row(special(kMore, 1.5), chars("?!'%№*_", nil), special(kBack, 1.5)),
		bottomRow(10),
	}}
	layoutSym2 = kbLayout{rows: [][]keyDef{
		chars("[]{}#^<>=~", nil),
		chars("_\\|€$£+•«»", nil),
		row(special(kMore, 1.5), chars("…—–°§±×", nil), special(kBack, 1.5)),
		bottomRow(10),
	}}
)

// Keyboard — экранная клавиатура. Её кладут на страницу экрана вместе с полем ввода:
// клавиши — отдельные виджеты, и нажатие каждой показывается инверсией.
type Keyboard struct {
	Box
	Target KeyTarget
	// EnterLabel — подпись клавиши ввода: «Готово», «Найти»; пусто — стрелка перевода строки.
	EnterLabel string
	// English — начать с английской раскладки: адреса, пароли.
	English bool

	theme     *Theme
	mode      int // 0 — буквы, 1 и 2 — страницы знаков
	shift     int // 0 — строчные, 1 — одна заглавная, 2 — все заглавные
	lastShift time.Time
	keys      []*kbKey
}

// NewKeyboard создаёт клавиатуру, которая печатает в target.
func NewKeyboard(t *Theme, target KeyTarget) *Keyboard {
	k := &Keyboard{Target: target, theme: t}
	return k
}

// Height — высота клавиатуры: четыре ряда клавиш.
func (k *Keyboard) Height(t *Theme, width int) int {
	return 4*k.keyHeight() + 3*k.gap() + 2*t.MM(1.5)
}

func (k *Keyboard) keyHeight() int { return k.theme.MM(10.5) }
func (k *Keyboard) gap() int       { return k.theme.MM(1.2) }

func (k *Keyboard) layout() kbLayout {
	switch k.mode {
	case 1:
		return layoutSym
	case 2:
		return layoutSym2
	}
	if k.English {
		return layoutEN
	}
	return layoutRU
}

func (k *Keyboard) SetBounds(r image.Rectangle) {
	k.R = r
	k.place()
}

// place раскладывает клавиши текущей раскладки.
func (k *Keyboard) place() {
	t := k.theme
	lay := k.layout()
	area := Pad(k.R, t.MM(1), t.MM(1.5), t.MM(1), t.MM(1.5))
	maxUnits := 0.0
	for _, rw := range lay.rows {
		maxUnits = max(maxUnits, units(rw))
	}
	gap := k.gap()
	unit := float64(area.Dx()+gap) / maxUnits
	k.keys = k.keys[:0]
	for i, rw := range lay.rows {
		y := area.Min.Y + i*(k.keyHeight()+gap)
		x := float64(area.Min.X) + (maxUnits-units(rw))*unit/2
		for _, d := range rw {
			w := d.units * unit
			key := &kbKey{kb: k, def: d}
			key.R = image.Rect(int(x+0.5), y, int(x+w+0.5)-gap, y+k.keyHeight())
			k.keys = append(k.keys, key)
			x += w
		}
	}
}

func units(rw []keyDef) float64 {
	u := 0.0
	for _, d := range rw {
		u += d.units
	}
	return u
}

func (k *Keyboard) Children() []Widget {
	out := make([]Widget, len(k.keys))
	for i, key := range k.keys {
		out[i] = key
	}
	return out
}

func (k *Keyboard) Draw(c *gfx.Canvas, t *Theme) {
	c.Fill(k.R, gfx.Gray16(14))
	c.Fill(image.Rect(k.R.Min.X, k.R.Min.Y, k.R.Max.X, k.R.Min.Y+t.Line()), gfx.Black)
}

func (k *Keyboard) redraw(h Host) { h.Invalidate(k.R, eink.UI) }

// SyncCaps включает заглавную, если получатель в начале предложения: после смены текста извне.
func (k *Keyboard) SyncCaps(h Host) {
	want := 0
	if ch, ok := k.Target.(CapsHinter); ok && ch.WantCaps() && k.mode == 0 {
		want = 1
	}
	if k.shift != 2 && k.shift != want {
		k.shift = want
		k.redraw(h)
	}
}

func (k *Keyboard) press(h Host, d keyDef, long bool) {
	if k.Target == nil {
		return
	}
	switch d.kind {
	case kChar:
		s := d.ch
		if long && d.alt != "" {
			s = d.alt
		}
		if k.shift > 0 && k.mode == 0 {
			s = strings.ToUpper(s)
		}
		k.Target.Insert(h, s)
		if k.shift == 1 {
			k.shift = 0
			k.redraw(h)
		}
		k.SyncCaps(h)
	case kShift:
		switch {
		case long:
			k.shift = 2
		case k.shift == 0:
			k.shift = 1
		case k.shift == 1 && time.Since(k.lastShift) < 450*time.Millisecond:
			k.shift = 2
		default:
			k.shift = 0
		}
		k.lastShift = time.Now()
		k.redraw(h)
	case kBack:
		k.Target.Backspace(h, long)
		k.SyncCaps(h)
	case kMode:
		if k.mode == 0 {
			k.mode = 1
		} else {
			k.mode = 0
		}
		k.place()
		k.redraw(h)
		k.SyncCaps(h)
	case kMore:
		k.mode = 3 - k.mode
		k.place()
		k.redraw(h)
	case kLang:
		k.English = !k.English
		k.mode = 0
		k.place()
		k.redraw(h)
	case kSpace:
		if long {
			k.press(h, keyDef{kind: kLang}, false)
			return
		}
		k.Target.Insert(h, " ")
		k.SyncCaps(h)
	case kEnter:
		k.Target.Enter(h)
		k.SyncCaps(h)
	}
}

// kbKey — клавиша.
type kbKey struct {
	Box
	kb  *Keyboard
	def keyDef
}

func (key *kbKey) Enabled() bool                   { return true }
func (key *kbKey) Tap(h Host, _ image.Point)       { key.kb.press(h, key.def, false) }
func (key *kbKey) LongPress(h Host, _ image.Point) { key.kb.press(h, key.def, true) }

func (key *kbKey) Draw(c *gfx.Canvas, t *Theme) {
	k := key.kb
	r := key.Bounds()
	d := key.def
	fill, fg := gfx.White, gfx.Black
	switch d.kind {
	case kChar, kSpace:
	case kShift:
		fill = gfx.Gray16(12)
		if k.shift > 0 {
			fill, fg = gfx.Black, gfx.White
		}
	case kEnter:
		fill, fg = gfx.Black, gfx.White
	default:
		fill = gfx.Gray16(12)
	}
	rad := t.MM(1.2)
	c.FillRoundRect(r, rad, fill)
	if fill == gfx.White {
		c.StrokeRoundRect(r, rad, max(t.Line()/2, 2), gfx.Gray16(8))
	}
	icon := func(ic icons.Drawer) {
		s := t.IconSize()
		ic(c, Center(r, s, s), fg)
	}
	label := func(s string, face *gfx.Face) { face.DrawIn(c, s, r, gfx.AlignCenter, fg) }
	switch d.kind {
	case kChar:
		s := d.ch
		if k.shift > 0 && k.mode == 0 {
			s = strings.ToUpper(s)
		}
		label(s, t.Face(gfx.Regular, 14))
		if d.alt != "" {
			hint := t.Face(gfx.Regular, 7)
			hr := image.Rect(r.Max.X-hint.Measure(d.alt)-t.MM(1), r.Min.Y+t.MM(0.3), r.Max.X-t.MM(0.8), r.Min.Y+t.MM(0.3)+hint.Height)
			hint.DrawIn(c, d.alt, hr, gfx.AlignRight, gfx.Gray16(6))
		}
	case kShift:
		icon(icons.Shift(k.shift == 2))
	case kBack:
		icon(icons.Backspace)
	case kEnter:
		if k.EnterLabel != "" {
			label(k.EnterLabel, t.Strong())
		} else {
			icon(icons.Enter)
		}
	case kMode:
		text := "123"
		if k.mode != 0 {
			text = "АБВ"
			if k.English {
				text = "ABC"
			}
		}
		label(text, t.Body())
	case kMore:
		text := "#+="
		if k.mode == 2 {
			text = "123"
		}
		label(text, t.Body())
	case kLang:
		text := "EN"
		if k.English {
			text = "RU"
		}
		label(text, t.Strong())
	case kSpace:
		name := layoutRU.name
		if k.English {
			name = layoutEN.name
		}
		t.Caption().DrawIn(c, name, r, gfx.AlignCenter, gfx.Gray16(7))
	}
}
