package draw

import (
	"math"

	"el-book/sdk/gfx"
	"el-book/sdk/ink"
)

// Template — разметка листа.
type Template string

const (
	Plain   Template = "plain"
	Lines   Template = "lines"
	Grid    Template = "grid"
	Dots    Template = "dots"
	Oblique Template = "oblique" // косая линейка: строки с вспомогательной линией и наклоном 65°
	Music   Template = "music"   // нотный стан
)

var templates = []Template{Plain, Lines, Grid, Dots, Oblique, Music}

func (t Template) title() string {
	switch t {
	case Lines:
		return "Линейка"
	case Grid:
		return "Клетка"
	case Dots:
		return "Точки"
	case Oblique:
		return "Косая"
	case Music:
		return "Ноты"
	}
	return "Чистый"
}

// ruling — линии разметки листа w×h пикселей при плотности dpi. Разметка рисуется поверх
// штрихов затемнением: ластик возвращает лист с линейкой, а не белое пятно.
func ruling(t Template, w, h, dpi int) []ink.Seg {
	mm := float64(dpi) / 25.4
	ink2 := gfx.Gray16(11)
	faint := gfx.Gray16(13)
	var out []ink.Seg
	hline := func(y float64, tone uint8, r float32) {
		out = append(out, ink.Seg{X0: 0, Y0: float32(y), X1: float32(w), Y1: float32(y), R0: r, R1: r, Tone: tone, Darken: true})
	}
	vline := func(x float64, tone uint8, r float32) {
		out = append(out, ink.Seg{X0: float32(x), Y0: 0, X1: float32(x), Y1: float32(h), R0: r, R1: r, Tone: tone, Darken: true})
	}
	step := 5 * mm
	switch t {
	case Lines:
		for y := 2 * step; y < float64(h); y += step * 1.6 {
			hline(y, ink2, 1)
		}
	case Grid:
		for y := step; y < float64(h); y += step {
			hline(y, ink2, 1)
		}
		for x := step; x < float64(w); x += step {
			vline(x, ink2, 1)
		}
	case Dots:
		for y := step; y < float64(h); y += step {
			for x := step; x < float64(w); x += step {
				out = append(out, ink.Seg{X0: float32(x), Y0: float32(y), X1: float32(x), Y1: float32(y), R0: 3, R1: 3, Tone: gfx.Gray16(9), Darken: true})
			}
		}
	case Oblique:
		// Строка: основная линия, линия высоты строчных букв на 4 мм выше и бледная
		// вспомогательная посередине; наклонные под 65° через 10 мм.
		row := 14 * mm
		for base := 3 * row / 2; base < float64(h); base += row {
			hline(base, ink2, 1)
			hline(base-4*mm, ink2, 1)
			hline(base-2*mm, faint, 0.75)
		}
		slant := 1 / math.Tan(65*math.Pi/180)
		for x := -float64(h) * slant; x < float64(w); x += 10 * mm {
			out = append(out, ink.Seg{X0: float32(x + float64(h)*slant), Y0: 0, X1: float32(x), Y1: float32(h), R0: 0.75, R1: 0.75, Tone: faint, Darken: true})
		}
	case Music:
		gap := 2.5 * mm
		for top := 20 * mm; top+4*gap < float64(h)-10*mm; top += 4*gap + 14*mm {
			for k := 0; k < 5; k++ {
				hline(top+float64(k)*gap, gfx.Gray16(9), 1)
			}
		}
	}
	return out
}
