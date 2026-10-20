package gfx

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "golang.org/x/image/draw"
)

// ToGray переводит любое изображение в оттенки серого.
func ToGray(src image.Image) *image.Gray {
	if g, ok := src.(*image.Gray); ok {
		return g
	}
	b := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// Downscale уменьшает изображение до размера w×h усреднением пикселей: так получаются
// миниатюры без ряби.
func Downscale(src *image.Gray, w, h int) *image.Gray {
	dst := image.NewGray(image.Rect(0, 0, w, h))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if w <= 0 || h <= 0 || sw == 0 || sh == 0 {
		return dst
	}
	for y := 0; y < h; y++ {
		y0, y1 := sb.Min.Y+y*sh/h, sb.Min.Y+(y+1)*sh/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := sb.Min.X+x*sw/w, sb.Min.X+(x+1)*sw/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sum, n uint32
			for yy := y0; yy < y1; yy++ {
				i := src.PixOffset(x0, yy)
				for xx := x0; xx < x1; xx++ {
					sum += uint32(src.Pix[i])
					i++
				}
				n += uint32(x1 - x0)
			}
			dst.Pix[dst.PixOffset(x, y)] = uint8(sum / n)
		}
	}
	return dst
}

// Scale масштабирует изображение в прямоугольник r холста. Оттенки серого масштабируются
// быстрым путём (Resize); остальные форматы — качественной интерполяцией x/image.
// Картинку, которую рисуют часто, лучше уменьшить один раз через Resize и копировать DrawGray.
func (c *Canvas) Scale(r image.Rectangle, src image.Image) {
	clipped := r.Intersect(c.clip)
	if clipped.Empty() || r.Empty() {
		return
	}
	if g, ok := src.(*image.Gray); ok {
		c.DrawGray(r, Resize(g, r.Dx(), r.Dy()), image.Point{})
		return
	}
	tmp := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	xdraw.CatmullRom.Scale(tmp, tmp.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	c.DrawGray(r, tmp, image.Point{})
}

// Resize возвращает копию src размером w×h: при уменьшении — среднее по площади (без ряби
// на штрихах и тексте), при увеличении — билинейная интерполяция.
func Resize(src *image.Gray, w, h int) *image.Gray {
	sb := src.Bounds()
	if w <= 0 || h <= 0 {
		return image.NewGray(image.Rect(0, 0, max(w, 0), max(h, 0)))
	}
	if w <= sb.Dx() && h <= sb.Dy() {
		return Downscale(src, w, h)
	}
	dst := image.NewGray(image.Rect(0, 0, w, h))
	sw, sh := sb.Dx(), sb.Dy()
	if sw == 0 || sh == 0 {
		return dst
	}
	for y := 0; y < h; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(h) - 0.5
		y0 := int(math.Floor(fy))
		ty := fy - float64(y0)
		y0c, y1c := clampInt(y0, 0, sh-1), clampInt(y0+1, 0, sh-1)
		r0 := src.Pix[src.PixOffset(sb.Min.X, sb.Min.Y+y0c):]
		r1 := src.Pix[src.PixOffset(sb.Min.X, sb.Min.Y+y1c):]
		out := dst.Pix[dst.PixOffset(0, y):]
		for x := 0; x < w; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(w) - 0.5
			x0 := int(math.Floor(fx))
			tx := fx - float64(x0)
			x0c, x1c := clampInt(x0, 0, sw-1), clampInt(x0+1, 0, sw-1)
			top := float64(r0[x0c])*(1-tx) + float64(r0[x1c])*tx
			bot := float64(r1[x0c])*(1-tx) + float64(r1[x1c])*tx
			out[x] = uint8(top*(1-ty) + bot*ty + 0.5)
		}
	}
	return dst
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Fit — прямоугольник с пропорциями size, вписанный в r по центру.
func Fit(size image.Point, r image.Rectangle) image.Rectangle {
	if size.X <= 0 || size.Y <= 0 {
		return image.Rectangle{Min: r.Min, Max: r.Min}
	}
	w, h := r.Dx(), r.Dy()
	if w*size.Y > h*size.X {
		w = h * size.X / size.Y
	} else {
		h = w * size.Y / size.X
	}
	min := image.Pt(r.Min.X+(r.Dx()-w)/2, r.Min.Y+(r.Dy()-h)/2)
	return image.Rectangle{Min: min, Max: min.Add(image.Pt(w, h))}
}

// Dither16 приводит изображение к 16 оттенкам панели диффузией ошибки Флойда — Стейнберга:
// фотографии на e-ink без полос.
func Dither16(src *image.Gray) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewGray(image.Rect(0, 0, w, h))
	errs := make([]int, (w+2)*2)
	cur, next := errs[:w+2], errs[w+2:]
	for y := 0; y < h; y++ {
		for i := range next {
			next[i] = 0
		}
		for x := 0; x < w; x++ {
			old := int(src.GrayAt(b.Min.X+x, b.Min.Y+y).Y) + cur[x+1]/16
			if old < 0 {
				old = 0
			} else if old > 255 {
				old = 255
			}
			q := (old + 8) / 17 * 17
			if q > 255 {
				q = 255
			}
			dst.SetGray(x, y, color.Gray{Y: uint8(q)})
			e := old - q
			cur[x+2] += e * 7
			next[x] += e * 3
			next[x+1] += e * 5
			next[x+2] += e
		}
		cur, next = next, cur
	}
	return dst
}
