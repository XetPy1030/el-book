package gfx

import (
	_ "embed"
	"fmt"
	"image"
	"image/draw"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Weight — начертание шрифта интерфейса.
type Weight int

const (
	Regular Weight = iota
	Medium
	Bold
	Mono
)

// Шрифт интерфейса — Fira Sans (лицензия SIL OFL 1.1, текст — fonts/OFL.txt): кириллица,
// несколько начертаний, обычный ноль. Моноширинный — Go Mono (лицензия BSD).
var (
	//go:embed fonts/FiraSans-Regular.ttf
	firaRegular []byte
	//go:embed fonts/FiraSans-Medium.ttf
	firaMedium []byte
	//go:embed fonts/FiraSans-SemiBold.ttf
	firaSemiBold []byte
)

// Fonts — семейство шрифтов интерфейса и кэш начертаний по размерам.
// Не потокобезопасно: пользуется только горутина интерфейса.
type Fonts struct {
	fonts [4]*opentype.Font
	faces map[faceKey]*Face
}

type faceKey struct {
	w  Weight
	px int
}

// DefaultFonts загружает встроенные шрифты.
func DefaultFonts() (*Fonts, error) {
	f := &Fonts{faces: map[faceKey]*Face{}}
	for w, data := range [][]byte{firaRegular, firaMedium, firaSemiBold, gomono.TTF} {
		parsed, err := opentype.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("разобрать встроенный шрифт %d: %w", w, err)
		}
		f.fonts[w] = parsed
	}
	return f, nil
}

// Face возвращает начертание размером px пикселей (кегль).
func (f *Fonts) Face(w Weight, px int) *Face {
	if px < 4 {
		px = 4
	}
	k := faceKey{w, px}
	if face, ok := f.faces[k]; ok {
		return face
	}
	of, err := opentype.NewFace(f.fonts[w], &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		// Встроенный шрифт уже разобран; ошибка здесь — ошибка программиста.
		panic(fmt.Sprintf("начертание %d/%dpx: %v", w, px, err))
	}
	m := of.Metrics()
	face := &Face{
		face:    of,
		glyphs:  map[rune]*glyph{},
		Ascent:  m.Ascent.Ceil(),
		Descent: m.Descent.Ceil(),
		Height:  m.Height.Ceil(),
	}
	f.faces[k] = face
	return face
}

// Face — шрифт одного размера с кэшем растеризованных глифов.
type Face struct {
	face    font.Face
	glyphs  map[rune]*glyph
	Ascent  int // от базовой линии вверх
	Descent int // от базовой линии вниз
	Height  int // рекомендуемый межстрочный интервал
}

type glyph struct {
	mask    *image.Alpha
	offset  image.Point // положение маски относительно точки на базовой линии
	advance fixed.Int26_6
}

func (f *Face) glyph(r rune) *glyph {
	if g, ok := f.glyphs[r]; ok {
		return g
	}
	dr, src, sp, adv, ok := f.face.Glyph(fixed.Point26_6{}, r)
	if !ok {
		if r != '?' {
			g := f.glyph('?')
			f.glyphs[r] = g
			return g
		}
		g := &glyph{mask: image.NewAlpha(image.Rectangle{}), advance: adv}
		f.glyphs[r] = g
		return g
	}
	mask := image.NewAlpha(image.Rect(0, 0, dr.Dx(), dr.Dy()))
	draw.Draw(mask, mask.Bounds(), src, sp, draw.Src)
	g := &glyph{mask: mask, offset: dr.Min, advance: adv}
	f.glyphs[r] = g
	return g
}

// Measure возвращает ширину строки в пикселях.
func (f *Face) Measure(s string) int {
	var w fixed.Int26_6
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			w += f.face.Kern(prev, r)
		}
		w += f.glyph(r).advance
		prev = r
	}
	return w.Ceil()
}

// Advance — сколько пикселей занимает руна r после руны prev, с кернингом; prev < 0 —
// начало строки. Сумма Advance по строке совпадает с Measure до округления.
func (f *Face) Advance(prev, r rune) float64 {
	a := f.glyph(r).advance
	if prev >= 0 {
		a += f.face.Kern(prev, r)
	}
	return float64(a) / 64
}

// Offsets возвращает смещения границ рун строки от её начала: len = число рун + 1.
// По ним ставят курсор и находят руну под касанием.
func (f *Face) Offsets(rs []rune) []int {
	out := make([]int, len(rs)+1)
	var x float64
	prev := rune(-1)
	for i, r := range rs {
		x += f.Advance(prev, r)
		out[i+1] = int(math.Round(x))
		prev = r
	}
	return out
}

// Draw пишет строку, начиная с точки (x, baseline) на базовой линии. Возвращает ширину.
func (f *Face) Draw(c *Canvas, s string, x, baseline int, v uint8) int {
	dot := fixed.I(x)
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			dot += f.face.Kern(prev, r)
		}
		g := f.glyph(r)
		if !g.mask.Bounds().Empty() {
			c.DrawMask(g.mask, image.Pt(dot.Round()+g.offset.X, baseline+g.offset.Y), v)
		}
		dot += g.advance
		prev = r
	}
	return (dot - fixed.I(x)).Ceil()
}

// Align — выравнивание строки в прямоугольнике.
type Align int

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// DrawIn пишет одну строку в прямоугольнике, по вертикали — по центру; длинная строка
// обрезается многоточием.
func (f *Face) DrawIn(c *Canvas, s string, r image.Rectangle, align Align, v uint8) {
	s = f.Ellipsize(s, r.Dx())
	w := f.Measure(s)
	x := r.Min.X
	switch align {
	case AlignCenter:
		x = r.Min.X + (r.Dx()-w)/2
	case AlignRight:
		x = r.Max.X - w
	}
	baseline := r.Min.Y + (r.Dy()+f.Ascent-f.Descent)/2
	f.Draw(c.WithClip(r), s, x, baseline, v)
}

// Ellipsize укорачивает строку до ширины maxW, заканчивая её многоточием.
func (f *Face) Ellipsize(s string, maxW int) string {
	if f.Measure(s) <= maxW {
		return s
	}
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if f.Measure(strings.TrimRightFunc(string(runes[:mid]), unicode.IsSpace)+"…") <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimRightFunc(string(runes[:lo]), unicode.IsSpace) + "…"
}

// Wrap разбивает текст на строки не шире maxW, по словам; слово длиннее строки режется.
// Переводы строк в тексте сохраняются.
func (f *Face) Wrap(s string, maxW int) []string {
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, w := range words {
			candidate := w
			if line != "" {
				candidate = line + " " + w
			}
			if f.Measure(candidate) <= maxW {
				line = candidate
				continue
			}
			if line != "" {
				lines = append(lines, line)
			}
			for f.Measure(w) > maxW {
				cut := f.fit(w, maxW)
				lines = append(lines, w[:cut])
				w = w[cut:]
			}
			line = w
		}
		lines = append(lines, line)
	}
	return lines
}

// fit — сколько байтов начала строки помещается в maxW (не меньше одной руны).
func (f *Face) fit(s string, maxW int) int {
	end := 0
	for i := range s {
		if i > 0 && f.Measure(s[:i]) > maxW {
			break
		}
		end = i
	}
	if end == 0 {
		_, size := utf8.DecodeRuneInString(s)
		return size
	}
	return end
}

// PxFromPt переводит пункты в пиксели при плотности dpi.
func PxFromPt(pt float64, dpi int) int {
	return int(math.Round(pt * float64(dpi) / 72))
}

// PxFromMM переводит миллиметры в пиксели при плотности dpi.
func PxFromMM(mm float64, dpi int) int {
	return int(math.Round(mm * float64(dpi) / 25.4))
}
