// Package gfx рисует в оттенках серого: холст с областью отсечения, фигуры, контуры, текст
// и изображения. Всё рисование — в *image.Gray, где 0 — чёрный, 255 — белый.
package gfx

import (
	"image"
	"image/draw"
)

// Основные цвета. Панель показывает 16 оттенков; промежуточные значения она округлит сама.
const (
	Black uint8 = 0
	White uint8 = 255
)

// Gray16 возвращает оттенок из 16 уровней панели: 0 — чёрный, 15 — белый.
func Gray16(level int) uint8 {
	if level < 0 {
		level = 0
	}
	if level > 15 {
		level = 15
	}
	return uint8(level * 17)
}

// Canvas — изображение и область, за которую рисование не выходит.
type Canvas struct {
	img  *image.Gray
	clip image.Rectangle
}

// NewCanvas создаёт холст на всё изображение.
func NewCanvas(img *image.Gray) *Canvas {
	return &Canvas{img: img, clip: img.Bounds()}
}

// Image — изображение под холстом.
func (c *Canvas) Image() *image.Gray { return c.img }

// Bounds — размер изображения.
func (c *Canvas) Bounds() image.Rectangle { return c.img.Bounds() }

// Clip — текущая область отсечения.
func (c *Canvas) Clip() image.Rectangle { return c.clip }

// WithClip возвращает холст на то же изображение с отсечением, суженным до r.
func (c *Canvas) WithClip(r image.Rectangle) *Canvas {
	return &Canvas{img: c.img, clip: c.clip.Intersect(r)}
}

// Fill заливает прямоугольник.
func (c *Canvas) Fill(r image.Rectangle, v uint8) {
	r = r.Intersect(c.clip)
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := c.img.Pix[c.img.PixOffset(r.Min.X, y):c.img.PixOffset(r.Max.X, y)]
		for i := range row {
			row[i] = v
		}
	}
}

// StrokeRect рисует рамку толщиной w внутрь прямоугольника.
func (c *Canvas) StrokeRect(r image.Rectangle, w int, v uint8) {
	if w <= 0 || r.Empty() {
		return
	}
	c.Fill(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), v)
	c.Fill(image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), v)
	c.Fill(image.Rect(r.Min.X, r.Min.Y+w, r.Min.X+w, r.Max.Y-w), v)
	c.Fill(image.Rect(r.Max.X-w, r.Min.Y+w, r.Max.X, r.Max.Y-w), v)
}

// Invert инвертирует прямоугольник: так показывается нажатие.
func (c *Canvas) Invert(r image.Rectangle) {
	r = r.Intersect(c.clip)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := c.img.Pix[c.img.PixOffset(r.Min.X, y):c.img.PixOffset(r.Max.X, y)]
		for i := range row {
			row[i] = 255 - row[i]
		}
	}
}

// DrawGray копирует src так, чтобы точка sp источника легла в r.Min. Копирует строками:
// в image/draw нет быстрого пути для *image.Gray, и общий путь в сотни раз медленнее.
func (c *Canvas) DrawGray(r image.Rectangle, src *image.Gray, sp image.Point) {
	clipped := r.Intersect(c.clip)
	if clipped.Empty() {
		return
	}
	sp = sp.Add(clipped.Min.Sub(r.Min))
	sr := image.Rectangle{Min: sp, Max: sp.Add(clipped.Size())}.Intersect(src.Bounds())
	if sr.Empty() {
		return
	}
	dst := image.Rectangle{Min: clipped.Min.Add(sr.Min.Sub(sp)), Max: clipped.Min.Add(sr.Max.Sub(sp))}
	w := dst.Dx()
	for y := 0; y < dst.Dy(); y++ {
		di := c.img.PixOffset(dst.Min.X, dst.Min.Y+y)
		si := src.PixOffset(sr.Min.X, sr.Min.Y+y)
		copy(c.img.Pix[di:di+w], src.Pix[si:si+w])
	}
}

// Draw рисует любое изображение в оттенках серого; *image.Gray копируется быстро.
func (c *Canvas) Draw(r image.Rectangle, src image.Image, sp image.Point) {
	if g, ok := src.(*image.Gray); ok {
		c.DrawGray(r, g, sp)
		return
	}
	clipped := r.Intersect(c.clip)
	if clipped.Empty() {
		return
	}
	draw.Draw(c.img, clipped, src, sp.Add(clipped.Min.Sub(r.Min)), draw.Src)
}

// blend смешивает пиксель с цветом v с покрытием a (0…255).
func blend(dst *uint8, v uint8, a uint32) {
	if a == 0 {
		return
	}
	if a >= 255 {
		*dst = v
		return
	}
	d := uint32(*dst)
	*dst = uint8((d*(255-a) + uint32(v)*a + 127) / 255)
}

// DrawMask заливает цветом v через маску покрытия mask, положенную началом в точку at.
func (c *Canvas) DrawMask(mask *image.Alpha, at image.Point, v uint8) {
	r := mask.Bounds().Sub(mask.Bounds().Min).Add(at).Intersect(c.clip)
	if r.Empty() {
		return
	}
	off := mask.Bounds().Min.Sub(at)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		di := c.img.PixOffset(r.Min.X, y)
		mi := mask.PixOffset(r.Min.X+off.X, y+off.Y)
		for x := r.Min.X; x < r.Max.X; x++ {
			blend(&c.img.Pix[di], v, uint32(mask.Pix[mi]))
			di++
			mi++
		}
	}
}
