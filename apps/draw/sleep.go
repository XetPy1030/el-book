package draw

import (
	"image"

	"el-book/sdk/gfx"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// paintSleep рисует на экране сна открытую страницу тетради, которую меняли последней.
// Тетрадей нет — false: оболочка покажет обычный экран сна.
func paintSleep(st *storage.App, c *gfx.Canvas, t *ui.Theme) bool {
	dirs, err := st.List("books")
	if err != nil {
		return false
	}
	var last *Book
	for _, d := range dirs {
		if !d.Dir {
			continue
		}
		b, err := loadBook(st, d.Name)
		if err != nil {
			continue
		}
		if last == nil || b.Modified > last.Modified {
			last = b
		}
	}
	if last == nil {
		return false
	}
	img, err := pageImage(st, last, last.Pages[last.Current])
	if err != nil {
		return false
	}
	screen := c.Bounds()
	fit := gfx.Fit(img.Bounds().Size(), screen)
	if fit.Size() == img.Bounds().Size() {
		c.DrawGray(fit, img, image.Point{})
	} else {
		c.Scale(fit, img)
	}
	return true
}
