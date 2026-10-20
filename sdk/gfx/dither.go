package gfx

import "image"

// bayer4 — пороги упорядоченного дизеринга Байера 4×4, 0…255.
var bayer4 = [16]uint8{
	8, 136, 40, 168,
	200, 72, 232, 104,
	56, 184, 24, 152,
	248, 120, 216, 88,
}

// Dither — точка (x, y) тона v в двух цветах: чёрная или белая по порогу Байера. Узор привязан
// к координатам, поэтому соседние области стыкуются без швов, а двухцветная картинка не меняется.
func Dither(v uint8, x, y int) uint8 {
	if v > bayer4[(y&3)<<2|(x&3)] {
		return White
	}
	return Black
}

// DitherInto пишет в dst двухцветную копию области r из src — для режимов панели, которые
// показывают только чёрное и белое: оттенки становятся узором, а не пропадают.
func DitherInto(dst, src *image.Gray, r image.Rectangle) {
	r = r.Intersect(src.Bounds()).Intersect(dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := src.Pix[src.PixOffset(r.Min.X, y):src.PixOffset(r.Max.X, y)]
		d := dst.Pix[dst.PixOffset(r.Min.X, y):dst.PixOffset(r.Max.X, y)]
		row := bayer4[(y&3)<<2 : (y&3)<<2+4]
		for i, v := range s {
			if v > row[(r.Min.X+i)&3] {
				d[i] = White
			} else {
				d[i] = Black
			}
		}
	}
}
