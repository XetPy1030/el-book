package ui

import (
	"fmt"
	"image"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
)

// BlockKind — вид абзаца документа.
type BlockKind int

const (
	Para       BlockKind = iota // обычный абзац
	Heading                     // заголовок раздела
	Subheading                  // подзаголовок
	Quote                       // цитата: серым с отступом и чертой слева
	Item                        // пункт списка с маркером
	Meta                        // мелко серым: дата, источник, подпись
)

// Block — абзац документа.
type Block struct {
	Kind BlockKind
	Text string
}

// Reader — длинный текст со страницами: заголовки, абзацы, цитаты, пункты списка. Прокрутки
// нет: текст листается свайпом и кнопками листания (Next, Prev), страница перерисовывается
// целиком одним обновлением — так на e-ink не бывает полос от прокрутки. Заголовок не
// остаётся последней строкой страницы.
type Reader struct {
	Box
	Blocks []Block
	Size   float64 // кегль основного текста; 0 — 13 pt
	OnPage func(page, pages int)

	theme *Theme
	pages [][]readerLine
	page  int
}

type readerLine struct {
	text  string
	face  *gfx.Face
	x, y  int // левый край и базовая линия относительно страницы
	color uint8
	quote bool // черта цитаты слева
	h     int
}

// NewReader создаёт пустой документ.
func NewReader(t *Theme) *Reader { return &Reader{theme: t} }

// SetBlocks заменяет текст и возвращает на первую страницу.
func (r *Reader) SetBlocks(bs []Block) {
	r.Blocks = bs
	r.page = 0
	r.layout()
}

func (r *Reader) SetBounds(rect image.Rectangle) {
	r.R = rect
	r.layout()
	r.page = min(r.page, max(len(r.pages)-1, 0))
}

func (r *Reader) size() float64 {
	if r.Size <= 0 {
		return 13
	}
	return r.Size
}

// face — шрифт и цвет вида абзаца.
func (r *Reader) face(k BlockKind) (*gfx.Face, uint8) {
	t, pt := r.theme, r.size()
	switch k {
	case Heading:
		return t.Face(gfx.Bold, pt*1.35), gfx.Black
	case Subheading:
		return t.Face(gfx.Bold, pt*1.1), gfx.Black
	case Quote:
		return t.Face(gfx.Regular, pt), gfx.Gray16(4)
	case Meta:
		return t.Face(gfx.Regular, pt*0.8), gfx.Gray16(5)
	}
	return t.Face(gfx.Regular, pt), gfx.Black
}

func (r *Reader) layout() {
	r.pages = nil
	if r.theme == nil || r.R.Empty() {
		return
	}
	t := r.theme
	width, height := r.R.Dx(), r.R.Dy()
	var page []readerLine
	y := 0
	flush := func() {
		r.pages = append(r.pages, page)
		page, y = nil, 0
	}
	for bi, b := range r.Blocks {
		face, color := r.face(b.Kind)
		indent := 0
		switch b.Kind {
		case Quote:
			indent = t.MM(4)
		case Item:
			indent = t.MM(5)
		}
		lines := face.Wrap(b.Text, width-indent)
		gap := face.Height / 2
		if b.Kind == Heading || b.Kind == Subheading {
			gap = face.Height * 2 / 3
		}
		// Заголовок переносится на новую страницу, если под ним не помещаются две строки.
		if (b.Kind == Heading || b.Kind == Subheading) && y > 0 {
			need := len(lines)*face.Height + gap
			if bi+1 < len(r.Blocks) {
				nf, _ := r.face(r.Blocks[bi+1].Kind)
				need += 2 * nf.Height
			}
			if y+need > height {
				flush()
			}
		}
		for li, text := range lines {
			if y+face.Height > height && y > 0 {
				flush()
			}
			l := readerLine{text: text, face: face, x: indent, y: y + face.Ascent, color: color, quote: b.Kind == Quote, h: face.Height}
			if b.Kind == Item && li == 0 {
				l.text = text
				page = append(page, readerLine{text: "•", face: face, x: t.MM(1.5), y: y + face.Ascent, color: color, h: face.Height})
			}
			page = append(page, l)
			y += face.Height
		}
		y += gap
	}
	if len(page) > 0 || len(r.pages) == 0 {
		flush()
	}
}

func (r *Reader) Draw(c *gfx.Canvas, t *Theme) {
	if r.page >= len(r.pages) {
		return
	}
	for _, l := range r.pages[r.page] {
		x, base := r.R.Min.X+l.x, r.R.Min.Y+l.y
		if l.quote {
			top := base - l.face.Ascent
			c.Fill(image.Rect(r.R.Min.X+t.MM(1), top, r.R.Min.X+t.MM(1)+t.Line()*2, top+l.h), gfx.Gray16(9))
		}
		l.face.Draw(c, l.text, x, base, l.color)
	}
}

// Page — открытая страница, с нуля.
func (r *Reader) Page() int { return r.page }

// Pages — число страниц.
func (r *Reader) Pages() int { return max(len(r.pages), 1) }

// PageText — «2 / 5».
func (r *Reader) PageText() string { return fmt.Sprintf("%d / %d", r.page+1, r.Pages()) }

// SetPage открывает страницу n.
func (r *Reader) SetPage(h Host, n int) bool {
	if n < 0 || n >= r.Pages() || n == r.page {
		return false
	}
	r.page = n
	if r.OnPage != nil {
		r.OnPage(r.page, r.Pages())
	}
	h.Invalidate(r.R, eink.Content)
	return true
}

// Next и Prev листают страницы — для кнопок листания.
func (r *Reader) Next(h Host) bool { return r.SetPage(h, r.page+1) }
func (r *Reader) Prev(h Host) bool { return r.SetPage(h, r.page-1) }

func (r *Reader) Swipe(h Host, dir Direction) bool {
	switch dir {
	case DirLeft, DirUp:
		return r.Next(h)
	case DirRight, DirDown:
		return r.Prev(h)
	}
	return false
}

// Tap листает: правая треть — вперёд, левая — назад, как в читалке.
func (r *Reader) Tap(h Host, p image.Point) {
	switch {
	case p.X > r.R.Min.X+r.R.Dx()*2/3:
		r.Next(h)
	case p.X < r.R.Min.X+r.R.Dx()/3:
		r.Prev(h)
	}
}

func (r *Reader) Enabled() bool    { return true }
func (r *Reader) ShowsPress() bool { return false }
