package ui

import (
	"image"
	"strings"
	"testing"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
)

// fakeHost запоминает запросы перерисовки.
type fakeHost struct {
	t     *Theme
	dirty []image.Rectangle
}

func (h *fakeHost) Invalidate(r image.Rectangle, _ eink.Class) { h.dirty = append(h.dirty, r) }
func (h *fakeHost) Theme() *Theme                              { return h.t }

func newHost(t *testing.T) *fakeHost {
	t.Helper()
	fonts, err := gfx.DefaultFonts()
	if err != nil {
		t.Fatal(err)
	}
	return &fakeHost{t: NewTheme(300, fonts)}
}

func TestCutLayout(t *testing.T) {
	r := image.Rect(0, 0, 100, 200)
	top := CutTop(&r, 30)
	bottom := CutBottom(&r, 20)
	left := CutLeft(&r, 10)
	if top != image.Rect(0, 0, 100, 30) || bottom != image.Rect(0, 180, 100, 200) ||
		left != image.Rect(0, 30, 10, 180) || r != image.Rect(10, 30, 100, 180) {
		t.Fatalf("раскладка: %v %v %v %v", top, bottom, left, r)
	}
	if got := CutTop(&r, 1000); got.Dy() != 150 || !r.Empty() {
		t.Fatalf("отрезали больше, чем было: %v, остаток %v", got, r)
	}
	cols := Columns(image.Rect(0, 0, 100, 10), 3, 5)
	if cols[0].Min.X != 0 || cols[2].Max.X != 100 || cols[1].Min.X-cols[0].Max.X != 5 {
		t.Fatalf("колонки: %v", cols)
	}
}

func TestWrapRunes(t *testing.T) {
	adv := func(prev, r rune) float64 { return 10 } // моноширинный: 10 пикселей на руну
	text := []rune("один два три\n\nчетыре")
	lines := wrapRunes(text, 75, adv)
	var got []string
	for _, l := range lines {
		got = append(got, string(text[l.start:l.end]))
	}
	want := []string{"один ", "два три", "", "четыре"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("перенос %q, нужно %q", got, want)
	}
	long := []rune("абвгдеёжзий")
	for _, l := range wrapRunes(long, 40, adv) {
		if l.end-l.start > 4 || l.end == l.start {
			t.Fatalf("длинное слово не разрезано: %v", wrapRunes(long, 40, adv))
		}
	}
	if l := wrapRunes(nil, 40, adv); len(l) != 1 || l[0] != (textLine{0, 0}) {
		t.Fatalf("пустой текст: %v", l)
	}
}

func TestKeyboardTypesRussian(t *testing.T) {
	h := newHost(t)
	field := &TextField{}
	kb := NewKeyboard(h.t, field)
	kb.SetBounds(image.Rect(0, 1300, 1440, 1920))
	tap := func(ch string) {
		t.Helper()
		for _, k := range kb.keys {
			if k.def.kind == kChar && k.def.ch == ch {
				k.Tap(h, image.Point{})
				return
			}
		}
		t.Fatalf("нет клавиши %q", ch)
	}
	special := func(kind keyKind, long bool) {
		for _, k := range kb.keys {
			if k.def.kind == kind {
				if long {
					k.LongPress(h, image.Point{})
				} else {
					k.Tap(h, image.Point{})
				}
				return
			}
		}
		t.Fatalf("нет клавиши %d", kind)
	}
	// Пустое поле начинается с заглавной.
	kb.SyncCaps(h)
	tap("п")
	tap("р")
	for _, k := range kb.keys {
		if k.def.ch == "е" {
			k.LongPress(h, image.Point{}) // «ё» долгим нажатием
		}
	}
	tap("т")
	special(kSpace, false)
	tap("м")
	tap("и")
	tap("р")
	if got := field.Text(); got != "Прёт мир" {
		t.Fatalf("набрано %q", got)
	}
	special(kBack, true) // стереть слово; пробел перед ним остаётся
	if got := field.Text(); got != "Прёт " {
		t.Fatalf("после стирания слова %q", got)
	}
	special(kBack, false)
	special(kLang, false)
	tap("o")
	tap("k")
	if got := field.Text(); got != "Прётok" {
		t.Fatalf("после смены раскладки %q", got)
	}
}

func TestTextAreaEditsOnlyChangedLines(t *testing.T) {
	h := newHost(t)
	a := NewTextArea(h.t, 13)
	a.SetBounds(image.Rect(0, 0, 600, 600))
	a.SetText("Первая строка\nвторая строка\nтретья")
	h.dirty = nil
	a.Insert(h, "!")
	if len(h.dirty) != 1 || h.dirty[0].Dy() > a.lineHeight() {
		t.Fatalf("печать в конце должна обновить одну строку: %v", h.dirty)
	}
	if !strings.HasSuffix(a.Text(), "третья!") {
		t.Fatalf("текст %q", a.Text())
	}
	a.Backspace(h, true)
	if a.Text() != "Первая строка\nвторая строка\n" {
		t.Fatalf("после стирания слова %q", a.Text())
	}
	if !a.WantCaps() {
		t.Fatal("в начале абзаца нужна заглавная")
	}
}

func TestListPages(t *testing.T) {
	h := newHost(t)
	l := NewList(h.t)
	var items []ListItem
	for i := 0; i < 40; i++ {
		items = append(items, ListItem{Title: "строка", OnTap: func() {}})
	}
	l.SetItems(items)
	l.SetBounds(image.Rect(0, 0, 1440, 1200))
	if l.Pages() < 2 {
		t.Fatalf("40 строк на 1200 пикселей должны дать несколько страниц, получено %d", l.Pages())
	}
	if !l.Next(h) || l.Page() != 1 {
		t.Fatal("вперёд не листается")
	}
	if l.SetPage(h, l.Pages()) {
		t.Fatal("за последнюю страницу листать нельзя")
	}
	for _, r := range l.Children() {
		if r.Bounds().Max.Y > l.footer.Min.Y {
			t.Fatalf("строка %v залезла на подпись страниц %v", r.Bounds(), l.footer)
		}
	}
}

func TestSegmentedFitsLongLabels(t *testing.T) {
	h := newHost(t)
	s := &Segmented{Options: []string{"Чистый", "Линейка", "Клетка", "Точки", "Косая", "Ноты"}}
	s.SetBounds(image.Rect(0, 0, 1200, 100))
	c := gfx.NewCanvas(image.NewGray(image.Rect(0, 0, 1200, 100)))
	s.Draw(c, h.t)
	face := s.fit(h.t)
	for i, o := range s.Options {
		cell := s.cell(i)
		if inner := cell.Dx() - 2*h.t.MM(1); face.Measure(o) > inner {
			t.Errorf("«%s» не помещается: %d > %d", o, face.Measure(o), inner)
		}
		if i > 0 && cell.Min.X != s.cell(i-1).Max.X {
			t.Errorf("сегменты %d и %d не стыкуются", i-1, i)
		}
	}
	if last := s.cell(len(s.Options) - 1); last.Max.X != 1200 {
		t.Errorf("последний сегмент кончается на %d, а не у края", last.Max.X)
	}
	s.Selected = 0
	host := &fakeHost{t: h.t}
	s.Tap(host, image.Pt(s.cell(1).Min.X+2, 50))
	if s.Selected != 1 {
		t.Errorf("касание у левого края «Линейки» выбрало %d", s.Selected)
	}
	short := &Segmented{Options: []string{"2", "5", "15"}}
	short.SetBounds(image.Rect(0, 0, 900, 100))
	short.Draw(c, h.t)
	if short.cell(1) != image.Rect(300, 0, 600, 100) {
		t.Errorf("короткие надписи — равные доли, а второй сегмент %v", short.cell(1))
	}
}
