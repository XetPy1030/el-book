package ink

import (
	"image"
	"math"

	"el-book/sdk/gfx"
)

// Quality — как рисовать штрих.
type Quality int

const (
	// Fast — только чёрное и белое без сглаживания: для следа под пальцем, который панель
	// показывает быстрым двухцветным обновлением. Серый передаётся упорядоченным растром.
	Fast Quality = iota
	// Fine — со сглаживанием и оттенками серого: чистовой вид.
	Fine
)

// blendMode — как цвет штриха ложится на лист.
type blendMode int

const (
	normal blendMode = iota // закрашивает
	darken                  // только затемняет: маркер не перекрывает чёрное
)

// span — растеризатор капсулы: отрезок с радиусом r0 в начале и r1 в конце. Каждая строка
// обходится только там, где капсула её пересекает: длинная диагональ стоит пропорционально
// своей площади, а не охватывающего прямоугольника.
type span struct {
	img   *image.Gray
	clip  image.Rectangle
	tone  uint8
	mode  blendMode
	q     Quality
	grain uint32 // не 0 — зерно карандаша: покрытие точек неровное, как графит на бумаге
}

func newSpan(c *gfx.Canvas, tone uint8, mode blendMode, q Quality) span {
	return span{img: c.Image(), clip: c.Clip().Intersect(c.Image().Bounds()), tone: tone, mode: mode, q: q}
}

// capsule рисует отрезок (x0, y0)–(x1, y1) с радиусами r0 и r1 и возвращает задетую область.
func (s span) capsule(x0, y0, x1, y1, r0, r1 float64) image.Rectangle {
	rmax := math.Max(r0, r1)
	if rmax <= 0 {
		return image.Rectangle{}
	}
	pad := rmax + 1
	box := image.Rect(
		int(math.Floor(math.Min(x0, x1)-pad)), int(math.Floor(math.Min(y0, y1)-pad)),
		int(math.Ceil(math.Max(x0, x1)+pad))+1, int(math.Ceil(math.Max(y0, y1)+pad))+1,
	)
	area := box.Intersect(s.clip)
	if area.Empty() {
		return box.Intersect(s.img.Bounds())
	}
	dx, dy := x1-x0, y1-y0
	ll := dx*dx + dy*dy
	if ll < 1e-9 {
		ll = 0 // точка: у почти нулевого отрезка проекция скакала бы между концами
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		py := float64(y) + 0.5
		lo, hi := s.rowExtent(x0, y0, x1, y1, pad, py)
		xa := max(int(math.Floor(lo)), area.Min.X)
		xb := min(int(math.Ceil(hi))+1, area.Max.X)
		if xa >= xb {
			continue
		}
		i := s.img.PixOffset(xa, y)
		for x := xa; x < xb; x++ {
			px := float64(x) + 0.5
			t := 0.0
			if ll > 0 {
				t = ((px-x0)*dx + (py-y0)*dy) / ll
				t = math.Max(0, math.Min(1, t))
			}
			ex, ey := px-(x0+t*dx), py-(y0+t*dy)
			d := math.Sqrt(ex*ex + ey*ey)
			r := r0 + t*(r1-r0)
			s.plot(i, x, y, r+0.5-d)
			i++
		}
	}
	return box.Intersect(s.img.Bounds())
}

// rowExtent — где на строке py может лежать капсула радиуса pad: объединение двух кругов
// и полосы между ними. Капсула выпукла, поэтому это один отрезок.
func (s span) rowExtent(x0, y0, x1, y1, pad, py float64) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	circle := func(cx, cy float64) {
		if h := pad*pad - (py-cy)*(py-cy); h >= 0 {
			w := math.Sqrt(h)
			lo, hi = math.Min(lo, cx-w), math.Max(hi, cx+w)
		}
	}
	circle(x0, y0)
	circle(x1, y1)
	dx, dy := x1-x0, y1-y0
	if l := math.Hypot(dx, dy); l > 0 {
		// Полоса: |n·(p−p0)| ≤ pad и 0 ≤ u·(p−p0) ≤ l, где u — направление, n — нормаль.
		ux, uy := dx/l, dy/l
		a, b := math.Inf(-1), math.Inf(1)
		clamp := func(k, c, from, to float64) { // from ≤ k·px + c ≤ to
			if k == 0 {
				if c < from || c > to {
					a, b = 1, 0
				}
				return
			}
			p, q := (from-c)/k, (to-c)/k
			if p > q {
				p, q = q, p
			}
			a, b = math.Max(a, p), math.Min(b, q)
		}
		clamp(-uy, ux*(py-y0)+uy*x0, -pad, pad)
		clamp(ux, uy*(py-y0)-ux*x0, 0, l)
		if a <= b {
			lo, hi = math.Min(lo, a), math.Max(hi, b)
		}
	}
	return lo, hi
}

// plot кладёт точку с покрытием cov (больше 1 — полностью внутри).
func (s span) plot(i, x, y int, cov float64) {
	if cov <= 0 {
		return
	}
	if s.grain != 0 {
		cov = math.Min(cov, 1) * (0.35 + 0.65*float64(hash(s.grain, uint32(x), uint32(y)))/65535)
	}
	dst := &s.img.Pix[i]
	if s.q == Fast {
		if cov < 0.5 {
			return
		}
		v := gfx.Dither(s.tone, x, y)
		if s.mode == darken && v > *dst {
			return
		}
		*dst = v
		return
	}
	a := uint32(255)
	if cov < 1 {
		a = uint32(cov * 255)
	}
	d := uint32(*dst)
	v := uint8((d*(255-a) + uint32(s.tone)*a + 127) / 255)
	if s.mode == darken && v > *dst {
		return
	}
	*dst = v
}

// line — тонкая линия со сглаживанием для процедурных кистей: капсула радиуса w/2 с тоном
// tone, который смешивается затемнением.
func (s span) line(x0, y0, x1, y1, w float64) image.Rectangle {
	return s.capsule(x0, y0, x1, y1, w/2, w/2)
}
