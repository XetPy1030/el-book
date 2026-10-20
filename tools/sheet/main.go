// Sheet рисует контрольный лист: все иконки и шрифты интерфейса в нескольких размерах.
// Нужен, чтобы оценить их глазами без книги: go run ./tools/sheet -out build/sheet.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	"el-book/sdk/gfx"
	"el-book/sdk/icons"
)

func main() {
	out := flag.String("out", "build/sheet.png", "куда записать PNG")
	dpi := flag.Int("dpi", 300, "плотность экрана")
	flag.Parse()

	img := image.NewGray(image.Rect(0, 0, 1440, 2900))
	c := gfx.NewCanvas(img)
	c.Fill(img.Bounds(), gfx.White)
	fonts, err := gfx.DefaultFonts()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	all := []struct {
		name string
		draw icons.Drawer
	}{
		{"Apps", icons.Apps}, {"Settings", icons.Settings}, {"Moon", icons.Moon}, {"Exit", icons.Exit},
		{"Sun", icons.Sun}, {"Warmth", icons.Warmth}, {"Back", icons.Back}, {"Home", icons.Home},
		{"Close", icons.Close}, {"Check", icons.Check}, {"ChevronL", icons.ChevronLeft}, {"ChevronR", icons.ChevronRight},
		{"Plus", icons.Plus}, {"Trash", icons.Trash}, {"Pencil", icons.Pencil}, {"Eraser", icons.Eraser},
		{"Undo", icons.Undo}, {"Redo", icons.Redo}, {"Page", icons.Page}, {"ClearPage", icons.ClearPage},
		{"GridPaper", icons.GridPaper}, {"Gallery", icons.Gallery}, {"Export", icons.Export}, {"Power", icons.Power},
		{"Rotate", icons.Rotate}, {"Laptop", icons.Laptop}, {"Bolt", icons.Bolt}, {"Info", icons.Info},
		{"Bat 15", icons.Battery(15, false)}, {"Bat 60+", icons.Battery(60, true)}, {"Bat 100", icons.Battery(100, false)},
		{"More", icons.More}, {"Menu", icons.Menu}, {"Search", icons.Search}, {"Wifi 0", icons.Wifi(0)},
		{"Wifi 2", icons.Wifi(2)}, {"Wifi 3", icons.Wifi(3)}, {"Wifi off", icons.Wifi(-1)}, {"Refresh", icons.Refresh},
		{"Calendar", icons.Calendar}, {"Note", icons.Note}, {"Checklist", icons.Checklist}, {"Calculator", icons.Calculator},
		{"News", icons.News}, {"Globe", icons.Globe}, {"Cloud", icons.Cloud}, {"CloudSun", icons.CloudSun},
		{"Rain", icons.Rain}, {"Snow", icons.Snow}, {"Storm", icons.Storm}, {"Fog", icons.Fog},
		{"Clock", icons.Clock}, {"Timer", icons.Timer}, {"Sudoku", icons.Sudoku}, {"Mine", icons.Mine},
		{"Flag", icons.Flag}, {"Tiles", icons.Tiles}, {"Keyboard", icons.Keyboard}, {"Backspace", icons.Backspace},
		{"Shift", icons.Shift(false)}, {"Shift on", icons.Shift(true)}, {"Enter", icons.Enter}, {"Star", icons.Star(false)},
		{"Star on", icons.Star(true)}, {"Pin", icons.Pin}, {"Folder", icons.Folder}, {"Brush", icons.Brush},
		{"Marker", icons.Marker}, {"Lasso", icons.Lasso}, {"Shapes", icons.Shapes}, {"Line", icons.Line},
		{"Rectangle", icons.Rectangle}, {"Ellipse", icons.Ellipse}, {"Triangle", icons.Triangle}, {"Arrow", icons.Arrow},
		{"Pages", icons.Pages}, {"PageAdd", icons.PageAdd}, {"Tone", icons.Tone}, {"Copy", icons.Copy},
		{"Edit", icons.Edit}, {"Download", icons.Download}, {"Box", icons.Checkbox(false)}, {"Box on", icons.Checkbox(true)},
		{"Radio", icons.Radio(false)}, {"Radio on", icons.Radio(true)}, {"ChevronU", icons.ChevronUp}, {"ChevronD", icons.ChevronDown},
		{"Text", icons.Text}, {"Sparkle", icons.Sparkle}, {"Rss", icons.Rss}, {"Drop", icons.Drop},
		{"Wind", icons.Wind}, {"Book", icons.Book}, {"Dice", icons.Dice}, {"Bell", icons.Bell},
		{"Hand", icons.Hand}, {"Play", icons.Play}, {"Pause", icons.Pause}, {"Stop", icons.Stop},
		{"Minus", icons.Minus},
	}
	label := fonts.Face(gfx.Regular, gfx.PxFromPt(7, *dpi))
	cell := 144
	for i, ic := range all {
		x, y := (i%10)*cell, (i/10)*cell+8
		box := image.Rect(x+cell/2-40, y+8, x+cell/2+40, y+88)
		ic.draw(c, box, gfx.Black)
		label.DrawIn(c, ic.name, image.Rect(x, y+94, x+cell, y+130), gfx.AlignCenter, gfx.Black)
	}
	// Мелкие размеры: так иконки выглядят в статус-баре и на кнопках.
	y := (len(all)+9)/10*cell + 24
	for i, ic := range all[:16] {
		for j, size := range []int{36, 56, 80} {
			x := 8 + i*88
			r := image.Rect(x, y+j*96, x+size, y+j*96+size)
			ic.draw(c, r, gfx.Black)
		}
	}
	y += 3*96 + 24
	text := "Рисовалка · Настройки · Сон 14:05 — Ёжик съел 99% «щей»"
	for i, pt := range []float64{7, 9, 11, 14, 20, 28} {
		face := fonts.Face(gfx.Regular, gfx.PxFromPt(pt, *dpi))
		face.Draw(c, fmt.Sprintf("%gpt %s", pt, text), 16, y+face.Ascent, gfx.Black)
		y += face.Height + 12
		if i == 3 {
			bold := fonts.Face(gfx.Bold, gfx.PxFromPt(pt, *dpi))
			bold.Draw(c, "Жирный: "+text, 16, y+bold.Ascent, gfx.Black)
			y += bold.Height + 12
		}
	}
	for i := 0; i < 16; i++ {
		c.Fill(image.Rect(16+i*88, y, 16+i*88+80, y+80), gfx.Gray16(i))
	}

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(*out)
}
