package gfx

import (
	"image"
	"image/draw"
	"strings"
	"testing"
)

func white(w, h int) (*image.Gray, *Canvas) {
	img := image.NewGray(image.Rect(0, 0, w, h))
	c := NewCanvas(img)
	c.Fill(img.Bounds(), White)
	return img, c
}

func TestCapsuleBinaryHasOnlyTwoColors(t *testing.T) {
	img, c := white(100, 100)
	box := c.Capsule(10, 10, 90, 60, 4, Black, false)
	if !box.In(img.Bounds()) || box.Empty() {
		t.Fatalf("неверная область штриха %v", box)
	}
	black := 0
	for _, v := range img.Pix {
		switch v {
		case Black:
			black++
		case White:
		default:
			t.Fatalf("штрих без сглаживания дал серый пиксель %d", v)
		}
	}
	if black < 600 || black > 900 {
		t.Fatalf("неправдоподобная площадь штриха: %d пикселей", black)
	}
	if img.GrayAt(50, 35).Y != Black || img.GrayAt(50, 80).Y != White {
		t.Fatal("штрих лёг не туда")
	}
}

func TestClipIsRespected(t *testing.T) {
	img, c := white(100, 100)
	clipped := c.WithClip(image.Rect(0, 0, 50, 100))
	clipped.Fill(img.Bounds(), Black)
	clipped.FillRoundRect(image.Rect(0, 0, 100, 100), 20, Black)
	clipped.Capsule(0, 50, 100, 50, 10, Black, true)
	for y := 0; y < 100; y++ {
		if img.GrayAt(75, y).Y != White {
			t.Fatalf("рисование вышло за отсечение в (75, %d)", y)
		}
		if img.GrayAt(25, y).Y != Black {
			t.Fatalf("внутри отсечения не закрашено (25, %d)", y)
		}
	}
}

func TestRoundRectCornersAreRound(t *testing.T) {
	img, c := white(100, 100)
	c.FillRoundRect(image.Rect(0, 0, 100, 100), 30, Black)
	if img.GrayAt(1, 1).Y != White {
		t.Fatal("угол должен остаться белым")
	}
	if img.GrayAt(50, 50).Y != Black || img.GrayAt(50, 1).Y != Black {
		t.Fatal("середина и край должны быть закрашены")
	}
}

func TestTextMeasureWrapEllipsize(t *testing.T) {
	fonts, err := DefaultFonts()
	if err != nil {
		t.Fatal(err)
	}
	face := fonts.Face(Regular, 40)
	if face.Measure("Привет") <= face.Measure("При") {
		t.Fatal("длинная строка должна быть шире короткой")
	}
	if face.Ascent <= 0 || face.Height < face.Ascent {
		t.Fatalf("неправдоподобные метрики: %+v", face)
	}
	text := "Съешь же ещё этих мягких французских булок, да выпей чаю"
	lines := face.Wrap(text, 400)
	if len(lines) < 3 {
		t.Fatalf("ожидался перенос на несколько строк: %q", lines)
	}
	for _, l := range lines {
		if face.Measure(l) > 400 {
			t.Fatalf("строка шире ограничения: %q", l)
		}
	}
	if strings.Join(lines, " ") != text {
		t.Fatalf("перенос потерял слова: %q", lines)
	}
	long := face.Wrap("Оченьоченьоченьдлинноеслово", 150)
	for _, l := range long {
		if face.Measure(l) > 150 {
			t.Fatalf("длинное слово не разрезано: %q", long)
		}
	}
	e := face.Ellipsize(text, 300)
	if !strings.HasSuffix(e, "…") || face.Measure(e) > 300 {
		t.Fatalf("многоточие не сработало: %q", e)
	}
	if face.Ellipsize("Да", 300) != "Да" {
		t.Fatal("короткая строка не должна меняться")
	}
}

func TestDownscaleAverages(t *testing.T) {
	img, c := white(4, 4)
	c.Fill(image.Rect(0, 0, 2, 4), Black)
	small := Downscale(img, 2, 1)
	if small.Pix[0] != 0 || small.Pix[1] != 255 {
		t.Fatalf("Downscale = %v", small.Pix)
	}
}

func TestDither16UsesPanelLevels(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = uint8(i % 256)
	}
	for _, v := range Dither16(img).Pix {
		if v%17 != 0 {
			t.Fatalf("уровень %d не из 16 уровней панели", v)
		}
	}
}

func TestFit(t *testing.T) {
	got := Fit(image.Pt(200, 100), image.Rect(0, 0, 100, 100))
	if got != image.Rect(0, 25, 100, 75) {
		t.Fatalf("Fit = %v", got)
	}
}

func TestDrawGrayMatchesImageDraw(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 50, 40))
	for i := range src.Pix {
		src.Pix[i] = uint8(i * 7)
	}
	img, c := white(64, 64)
	clipped := c.WithClip(image.Rect(5, 5, 60, 60))
	// Источник короче области: копируется только то, что в нём есть.
	clipped.DrawGray(image.Rect(10, 20, 70, 70), src, image.Pt(3, 2))
	want := image.NewGray(img.Bounds())
	for i := range want.Pix {
		want.Pix[i] = White
	}
	draw.Draw(want, image.Rect(10, 20, 70, 70).Intersect(image.Rect(5, 5, 60, 60)), src, image.Pt(3, 2), draw.Src)
	for i := range want.Pix {
		if want.Pix[i] != img.Pix[i] {
			t.Fatalf("пиксель %d: %d, ожидалось %d", i, img.Pix[i], want.Pix[i])
		}
	}
}

func TestStrokeRoundRectIsHollow(t *testing.T) {
	img, c := white(100, 60)
	c.StrokeRoundRect(image.Rect(0, 0, 100, 60), 12, 4, Black)
	if img.GrayAt(50, 30).Y != White {
		t.Fatal("рамка залила середину")
	}
	if img.GrayAt(50, 1).Y != Black || img.GrayAt(1, 30).Y != Black || img.GrayAt(98, 30).Y != Black {
		t.Fatal("стороны рамки не нарисованы")
	}
	if img.GrayAt(0, 0).Y != White {
		t.Fatal("скруглённый угол закрашен")
	}
	if v := img.GrayAt(3, 3).Y; v == White {
		t.Fatal("в углу нет дуги")
	}
}

func TestResizeUpAndDown(t *testing.T) {
	img, c := white(8, 8)
	c.Fill(image.Rect(0, 0, 4, 8), Black)
	up := Resize(img, 16, 16)
	if up.GrayAt(1, 8).Y != Black || up.GrayAt(14, 8).Y != White {
		t.Fatalf("увеличение исказило картинку: %d, %d", up.GrayAt(1, 8).Y, up.GrayAt(14, 8).Y)
	}
	down := Resize(img, 2, 2)
	if down.GrayAt(0, 0).Y != Black || down.GrayAt(1, 0).Y != White {
		t.Fatal("уменьшение исказило картинку")
	}
}

func BenchmarkDrawGrayFullScreen(b *testing.B) {
	src := image.NewGray(image.Rect(0, 0, 1440, 1800))
	img := image.NewGray(image.Rect(0, 0, 1440, 1920))
	c := NewCanvas(img)
	b.SetBytes(int64(len(src.Pix)))
	for i := 0; i < b.N; i++ {
		c.DrawGray(image.Rect(0, 120, 1440, 1920), src, image.Point{})
	}
}

func BenchmarkFillRoundRect(b *testing.B) {
	img := image.NewGray(image.Rect(0, 0, 1440, 200))
	c := NewCanvas(img)
	for i := 0; i < b.N; i++ {
		c.FillRoundRect(image.Rect(20, 20, 1420, 180), 18, Black)
	}
}

func TestDitherInto(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 64, 16))
	for x := 0; x < 64; x++ {
		for y := 0; y < 16; y++ {
			src.Pix[src.PixOffset(x, y)] = uint8(x * 4) // слева чёрный, справа почти белый
		}
	}
	src.Pix[src.PixOffset(0, 0)] = White
	dst := image.NewGray(src.Bounds())
	DitherInto(dst, src, src.Bounds())
	black := func(x0, x1 int) int {
		n := 0
		for x := x0; x < x1; x++ {
			for y := 0; y < 16; y++ {
				switch dst.Pix[dst.PixOffset(x, y)] {
				case Black:
					n++
				case White:
				default:
					t.Fatalf("точка (%d, %d) не двухцветная: %d", x, y, dst.Pix[dst.PixOffset(x, y)])
				}
			}
		}
		return n
	}
	if dark, light := black(4, 8), black(56, 60); dark <= light {
		t.Errorf("тёмная часть должна быть чернее светлой: %d и %d чёрных точек", dark, light)
	}
	if dst.Pix[0] != White {
		t.Error("белая точка стала чёрной")
	}
	again := image.NewGray(dst.Bounds())
	DitherInto(again, dst, dst.Bounds())
	for i := range dst.Pix {
		if again.Pix[i] != dst.Pix[i] {
			t.Fatal("двухцветная картинка изменилась при повторном дизеринге")
		}
	}
}
