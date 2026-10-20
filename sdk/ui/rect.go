package ui

import "image"

// Раскладка «отрезанием»: от прямоугольника экрана откусывают полосы сверху, снизу и по
// бокам, а остаток достаётся содержимому. Код раскладки читается сверху вниз, как сам экран:
//
//	r := bounds
//	bar := ui.CutTop(&r, t.Bar())
//	footer := ui.CutBottom(&r, t.Caption().Height)
//	list.SetBounds(r)

// CutTop отрезает от r полосу высотой h сверху и возвращает её; r уменьшается.
func CutTop(r *image.Rectangle, h int) image.Rectangle {
	h = clampSpan(h, r.Dy())
	out := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+h)
	r.Min.Y += h
	return out
}

// CutBottom отрезает полосу высотой h снизу.
func CutBottom(r *image.Rectangle, h int) image.Rectangle {
	h = clampSpan(h, r.Dy())
	out := image.Rect(r.Min.X, r.Max.Y-h, r.Max.X, r.Max.Y)
	r.Max.Y -= h
	return out
}

// CutLeft отрезает полосу шириной w слева.
func CutLeft(r *image.Rectangle, w int) image.Rectangle {
	w = clampSpan(w, r.Dx())
	out := image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y)
	r.Min.X += w
	return out
}

// CutRight отрезает полосу шириной w справа.
func CutRight(r *image.Rectangle, w int) image.Rectangle {
	w = clampSpan(w, r.Dx())
	out := image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y)
	r.Max.X -= w
	return out
}

func clampSpan(v, limit int) int {
	if v < 0 {
		return 0
	}
	if v > limit {
		return max(limit, 0)
	}
	return v
}

// Pad возвращает r с отступами слева, сверху, справа и снизу.
func Pad(r image.Rectangle, left, top, right, bottom int) image.Rectangle {
	out := image.Rect(r.Min.X+left, r.Min.Y+top, r.Max.X-right, r.Max.Y-bottom)
	if out.Dx() < 0 {
		out.Max.X = out.Min.X
	}
	if out.Dy() < 0 {
		out.Max.Y = out.Min.Y
	}
	return out
}

// Columns делит r на n колонок равной ширины с промежутком gap.
func Columns(r image.Rectangle, n, gap int) []image.Rectangle {
	if n <= 0 {
		return nil
	}
	out := make([]image.Rectangle, n)
	w := r.Dx() - gap*(n-1)
	for i := range out {
		x0 := r.Min.X + i*gap + w*i/n
		x1 := r.Min.X + i*gap + w*(i+1)/n
		out[i] = image.Rect(x0, r.Min.Y, x1, r.Max.Y)
	}
	return out
}

// Rows делит r на n строк равной высоты с промежутком gap.
func Rows(r image.Rectangle, n, gap int) []image.Rectangle {
	if n <= 0 {
		return nil
	}
	out := make([]image.Rectangle, n)
	h := r.Dy() - gap*(n-1)
	for i := range out {
		y0 := r.Min.Y + i*gap + h*i/n
		y1 := r.Min.Y + i*gap + h*(i+1)/n
		out[i] = image.Rect(r.Min.X, y0, r.Max.X, y1)
	}
	return out
}

// Center возвращает прямоугольник w×h по центру r.
func Center(r image.Rectangle, w, h int) image.Rectangle {
	x := r.Min.X + (r.Dx()-w)/2
	y := r.Min.Y + (r.Dy()-h)/2
	return image.Rect(x, y, x+w, y+h)
}
