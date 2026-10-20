package icons

import (
	"image"
	"math"

	"el-book/sdk/gfx"
)

// Иконки второй очереди: сеть, погода, приложения, инструменты рисования, клавиатура.
// Сетка и толщина линий — те же, что у первых (newPen).

// More — три точки: ещё действия.
func More(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	for _, x := range []float64{5, 12, 19} {
		p.disc(x, 12, 1.9)
	}
}

// Menu — три линии.
func Menu(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	for _, y := range []float64{6, 12, 18} {
		p.line(4, y, 20, y)
	}
}

// Search — лупа.
func Search(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.ring(10.5, 10.5, 6.5)
	p.line(15.5, 15.5, 20.5, 20.5)
}

// Wifi рисует значок Wi-Fi с уровнем сигнала level 0…3; level < 0 — Wi-Fi выключен.
func Wifi(level int) Drawer {
	return func(c *gfx.Canvas, r image.Rectangle, v uint8) {
		p := newPen(c, r, v)
		faint := p
		faint.v = gfx.Gray16(10)
		if v > 128 {
			faint.v = gfx.Gray16(6)
		}
		arcs := []float64{4.5, 9, 13.5}
		for i, rad := range arcs {
			q := p
			if level >= 0 && i >= level {
				q = faint
			}
			q.arc(12, 19, rad, -135, -45)
		}
		p.disc(12, 19, 1.8)
		if level < 0 {
			p.line(4, 4, 20, 20)
		}
	}
}

// Refresh — две стрелки по кругу: обновить.
func Refresh(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	const rad = 7.5
	p.arc(12, 12, rad, -150, -20)
	p.arc(12, 12, rad, 30, 160)
	end := -20.0 * math.Pi / 180
	p.arrowHead(12+rad*math.Cos(end), 12+rad*math.Sin(end), -math.Sin(end), math.Cos(end), 2.8)
	end = 160.0 * math.Pi / 180
	p.arrowHead(12+rad*math.Cos(end), 12+rad*math.Sin(end), -math.Sin(end), math.Cos(end), 2.8)
}

// Calendar — лист календаря.
func Calendar(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(3.5, 5, 20.5, 20.5, 2)
	p.line(3.5, 9.5, 20.5, 9.5)
	p.line(8, 3, 8, 6.5)
	p.line(16, 3, 16, 6.5)
	for _, x := range []float64{8, 12, 16} {
		p.disc(x, 13.5, 1.2)
		p.disc(x, 17, 1.2)
	}
}

// Note — лист с текстом: заметки.
func Note(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(4.5, 3, 19.5, 21, 2)
	for _, y := range []float64{8, 12, 16} {
		p.line(8, y, 16, y)
	}
}

// Checklist — строки с галочками: задачи.
func Checklist(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 3.5, 6.5, 5.5, 8.5, 9, 4.5)
	p.poly(false, 3.5, 13.5, 5.5, 15.5, 9, 11.5)
	p.line(12, 6.5, 20.5, 6.5)
	p.line(12, 13.5, 20.5, 13.5)
	p.roundRect(4, 18, 8.5, 22, 1)
	p.line(12, 20, 20.5, 20)
}

// Calculator — калькулятор.
func Calculator(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(5, 2.5, 19, 21.5, 2)
	p.roundRect(8, 5.5, 16, 9.5, 0.8)
	for _, y := range []float64{13.5, 17.5} {
		for _, x := range []float64{8.5, 12, 15.5} {
			p.disc(x, y, 1.2)
		}
	}
}

// News — газета.
func News(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 6, 4, 20.5, 4, 20.5, 18, 18.5, 20, 4.5, 20, 3.5, 19, 3.5, 9, 6, 9)
	p.line(6, 4, 6, 19)
	p.roundRect(9, 7, 13.5, 11, 0.5)
	p.line(15.5, 8, 18, 8)
	p.line(15.5, 11, 18, 11)
	p.line(9, 14.5, 18, 14.5)
	p.line(9, 17, 18, 17)
}

// Globe — глобус: интернет, язык клавиатуры.
func Globe(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.ring(12, 12, 8.5)
	p.line(3.5, 12, 20.5, 12)
	// Меридиан — эллипс, собранный из дуг окружности, сжатой по горизонтали.
	pts := make([]float64, 0, 2*33)
	for i := 0; i <= 32; i++ {
		a := float64(i) / 32 * 2 * math.Pi
		pts = append(pts, 12+4*math.Cos(a), 12+8.5*math.Sin(a))
	}
	p.poly(false, pts...)
}

// Cloud — облако.
func Cloud(c *gfx.Canvas, r image.Rectangle, v uint8) {
	cloudAt(newPen(c, r, v), 0, 0)
}

// cloudAt рисует облако со сдвигом (dx, dy) по сетке.
func cloudAt(p pen, dx, dy float64) {
	p.arc(8+dx, 14+dy, 4, 90, 270)
	p.arc(11.5+dx, 10+dy, 5, 200, 335)
	p.arc(16.5+dx, 13.5+dy, 4.5, -95, 90)
	p.line(8+dx, 18+dy, 16.5+dx, 18+dy)
}

// CloudSun — солнце за облаком: переменная облачность.
func CloudSun(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.arc(9, 8, 3.5, 120, 360)
	for _, a := range []float64{-160, -110, -60, -10} {
		rad := a * math.Pi / 180
		p.line(9+5.5*math.Cos(rad), 8+5.5*math.Sin(rad), 9+7.5*math.Cos(rad), 8+7.5*math.Sin(rad))
	}
	cloudAt(p, 2.5, 2)
}

// Rain — облако с дождём.
func Rain(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	cloudAt(p, 0, -3)
	for _, x := range []float64{8, 12, 16} {
		p.line(x, 17.5, x-1.5, 21)
	}
}

// Snow — облако со снегом.
func Snow(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	cloudAt(p, 0, -3)
	for _, x := range []float64{8, 12, 16} {
		p.disc(x, 19.5, 1.4)
	}
}

// Storm — облако с молнией.
func Storm(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	cloudAt(p, 0, -3)
	p.fill(13, 15.5, 9.5, 19.5, 12, 19.5, 10.5, 23, 15, 18.5, 12.5, 18.5)
}

// Fog — туман.
func Fog(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	cloudAt(p, 0, -4)
	p.line(4, 17.5, 20, 17.5)
	p.line(6, 21, 18, 21)
}

// Clock — циферблат: время, недавнее.
func Clock(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.ring(12, 12, 8.5)
	p.poly(false, 12, 7, 12, 12, 15.5, 14)
}

// Timer — секундомер.
func Timer(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.ring(12, 13.5, 7.5)
	p.line(12, 13.5, 12, 9.5)
	p.line(10, 3, 14, 3)
	p.line(12, 3, 12, 6)
	p.line(18, 6.5, 19.5, 5)
}

// Sudoku — поле 3×3 с цифрой.
func Sudoku(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(3.5, 3.5, 20.5, 20.5, 1.5)
	thin := p
	thin.w = p.w * 0.55
	for _, k := range []float64{9.2, 14.8} {
		thin.line(k, 4, k, 20)
		thin.line(4, k, 20, k)
	}
	p.disc(12, 12, 1.6)
	p.disc(6.3, 17.7, 1.2)
	p.disc(17.7, 6.3, 1.2)
}

// Mine — мина: сапёр.
func Mine(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.disc(12, 12, 5.5)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		p.line(12+6*math.Cos(a), 12+6*math.Sin(a), 12+8.5*math.Cos(a), 12+8.5*math.Sin(a))
	}
	back := p
	back.v = 255 - v
	back.disc(10, 10, 1.4)
}

// Flag — флажок: отметить мину.
func Flag(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(7, 3.5, 7, 20.5)
	p.fill(7, 4, 18, 8, 7, 12.5)
	p.line(4, 20.5, 12, 20.5)
}

// Tiles — четыре плитки: игра 2048.
func Tiles(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(3.5, 3.5, 11, 11, 1.5)
	p.roundRect(13, 3.5, 20.5, 11, 1.5)
	p.roundRect(3.5, 13, 11, 20.5, 1.5)
	full := newPen(c, r, v)
	a, b := full.pt(13, 13), full.pt(20.5, 20.5)
	c.FillRoundRect(image.Rect(int(a.X-full.w/2), int(a.Y-full.w/2), int(b.X+full.w/2), int(b.Y+full.w/2)), int(1.5*full.s), v)
}

// Keyboard — клавиатура.
func Keyboard(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(2.5, 6, 21.5, 18, 2)
	for _, x := range []float64{6.5, 10, 13.5, 17} {
		p.disc(x, 10, 1)
	}
	p.line(8, 14.5, 16, 14.5)
}

// Backspace — клавиша стирания.
func Backspace(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 9, 5.5, 21, 5.5, 21, 18.5, 9, 18.5, 2.5, 12)
	p.line(12.5, 9, 17.5, 15)
	p.line(17.5, 9, 12.5, 15)
}

// Shift — стрелка вверх контуром; filled — заглавные включены.
func Shift(filled bool) Drawer {
	return func(c *gfx.Canvas, r image.Rectangle, v uint8) {
		p := newPen(c, r, v)
		pts := []float64{12, 3.5, 21, 12.5, 16, 12.5, 16, 20, 8, 20, 8, 12.5, 3, 12.5}
		if filled {
			p.fill(pts...)
		}
		p.poly(true, pts...)
	}
}

// Enter — стрелка возврата ↵.
func Enter(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 19.5, 4.5, 19.5, 14, 5, 14)
	p.poly(false, 9.5, 9.5, 5, 14, 9.5, 18.5)
}

// Star — звезда; filled — закрашенная: избранное.
func Star(filled bool) Drawer {
	return func(c *gfx.Canvas, r image.Rectangle, v uint8) {
		p := newPen(c, r, v)
		pts := make([]float64, 0, 20)
		for i := 0; i < 10; i++ {
			rad := 9.0
			if i%2 == 1 {
				rad = 4
			}
			a := -math.Pi/2 + float64(i)*math.Pi/5
			pts = append(pts, 12+rad*math.Cos(a), 12.8+rad*math.Sin(a))
		}
		if filled {
			p.fill(pts...)
		}
		p.poly(true, pts...)
	}
}

// Pin — булавка: закрепить в доке.
func Pin(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 9, 3.5, 15, 3.5, 14, 9, 17.5, 13, 6.5, 13, 10, 9)
	p.line(12, 13, 12, 21)
}

// Folder — папка.
func Folder(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 3, 5.5, 9.5, 5.5, 11.5, 8, 21, 8, 21, 19, 3, 19)
}

// Brush — кисть.
func Brush(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(20, 4, 11.5, 12.5)
	p.fill(11, 11, 13, 13, 10.5, 17.5, 6.5, 20.5, 3.5, 20.5, 4.5, 16.5, 7.5, 13)
}

// Marker — маркер-выделитель.
func Marker(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 14, 3.5, 20.5, 10, 11.5, 19, 5, 12.5)
	p.poly(false, 8, 15.5, 4.5, 19, 8.5, 20.5, 10, 18.5)
	thick := p
	thick.w = p.w * 2.2
	thick.line(13.5, 20.5, 20.5, 20.5)
}

// Lasso — петля: выделить.
func Lasso(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	// Пунктирный эллипс.
	for i := 0; i < 12; i += 2 {
		a0 := float64(i) / 12 * 2 * math.Pi
		a1 := float64(i+1) / 12 * 2 * math.Pi
		pts := make([]float64, 0, 12)
		for k := 0; k <= 5; k++ {
			a := a0 + (a1-a0)*float64(k)/5
			pts = append(pts, 12+8.5*math.Cos(a), 9.5+6*math.Sin(a))
		}
		p.poly(false, pts...)
	}
	p.poly(false, 7, 14.5, 6, 18, 8.5, 21)
}

// Shapes — квадрат и круг: фигуры.
func Shapes(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(3.5, 9, 14, 20.5, 1)
	p.ring(15.5, 8.5, 5.5)
}

// Line — отрезок.
func Line(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(4.5, 19.5, 19.5, 4.5)
}

// Rectangle — прямоугольник.
func Rectangle(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).roundRect(3.5, 6, 20.5, 18, 0.5)
}

// Ellipse — эллипс.
func Ellipse(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	pts := make([]float64, 0, 2*41)
	for i := 0; i <= 40; i++ {
		a := float64(i) / 40 * 2 * math.Pi
		pts = append(pts, 12+8.5*math.Cos(a), 12+6*math.Sin(a))
	}
	p.poly(false, pts...)
}

// Triangle — треугольник.
func Triangle(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).poly(true, 12, 4, 20.5, 19.5, 3.5, 19.5)
}

// Arrow — стрелка вправо.
func Arrow(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(4, 12, 19.5, 12)
	p.poly(false, 13.5, 6, 19.5, 12, 13.5, 18)
}

// Pages — стопка листов: страницы тетради.
func Pages(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(7.5, 3, 20, 17.5, 1.5)
	p.poly(false, 4, 7, 4, 21, 16, 21)
}

// PageAdd — лист с плюсом: новая страница.
func PageAdd(c *gfx.Canvas, r image.Rectangle, v uint8) {
	Page(c, r, v)
	p := newPen(c, r, v)
	p.line(12, 11, 12, 17.5)
	p.line(8.75, 14.25, 15.25, 14.25)
}

// Tone — круг, закрашенный наполовину: оттенок, ночной режим.
func Tone(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.ring(12, 12, 8.5)
	pts := []float64{12, 3.5}
	for i := 0; i <= 16; i++ {
		a := -math.Pi/2 + float64(i)/16*math.Pi
		pts = append(pts, 12+8.5*math.Cos(a), 12+8.5*math.Sin(a))
	}
	p.fill(pts...)
}

// Copy — два листа: дублировать.
func Copy(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(8.5, 8.5, 20.5, 20.5, 1.5)
	p.poly(false, 15.5, 5.5, 15.5, 3.5, 3.5, 3.5, 3.5, 15.5, 5.5, 15.5)
}

// Edit — карандаш над чертой: переименовать.
func Edit(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 5, 17, 6, 13, 15.5, 3.5, 19, 7, 9.5, 16.5)
	p.line(4, 21, 20, 21)
}

// Download — стрелка вниз в лоток.
func Download(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(false, 4, 14, 4, 20, 20, 20, 20, 14)
	p.line(12, 4, 12, 15)
	p.poly(false, 8, 11, 12, 15, 16, 11)
}

// Checkbox — флажок: квадрат, с галочкой, если checked.
func Checkbox(checked bool) Drawer {
	return func(c *gfx.Canvas, r image.Rectangle, v uint8) {
		p := newPen(c, r, v)
		if checked {
			full := newPen(c, r, v)
			a, b := full.pt(3.5, 3.5), full.pt(20.5, 20.5)
			c.FillRoundRect(image.Rect(int(a.X-full.w/2), int(a.Y-full.w/2), int(b.X+full.w/2), int(b.Y+full.w/2)), int(2.5*full.s), v)
			back := p
			back.v = 255 - v
			back.poly(false, 7, 12.5, 10.5, 16, 17, 8.5)
			return
		}
		p.roundRect(3.5, 3.5, 20.5, 20.5, 2)
	}
}

// Radio — переключатель-кружок.
func Radio(on bool) Drawer {
	return func(c *gfx.Canvas, r image.Rectangle, v uint8) {
		p := newPen(c, r, v)
		p.ring(12, 12, 8)
		if on {
			p.disc(12, 12, 4.2)
		}
	}
}

// ChevronUp и ChevronDown — уголки вверх и вниз.
func ChevronUp(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).poly(false, 4.5, 15, 12, 7.5, 19.5, 15)
}

func ChevronDown(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).poly(false, 4.5, 9, 12, 16.5, 19.5, 9)
}

// Text — буква «Т»: текст.
func Text(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(5, 5, 19, 5)
	p.line(12, 5, 12, 20)
}

// Sparkle — четырёхлучевая звёздочка: художественные кисти.
func Sparkle(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.fill(10, 3, 11.8, 9.2, 18, 11, 11.8, 12.8, 10, 19, 8.2, 12.8, 2, 11, 8.2, 9.2)
	p.fill(18, 14, 18.9, 17.1, 22, 18, 18.9, 18.9, 18, 22, 17.1, 18.9, 14, 18, 17.1, 17.1)
}

// Rss — значок ленты новостей.
func Rss(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.disc(6, 18, 2)
	p.arc(5, 19, 7.5, -90, 0)
	p.arc(5, 19, 13, -90, 0)
}

// Drop — капля: осадки.
func Drop(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	pts := []float64{12, 3}
	for i := 0; i <= 16; i++ {
		a := -math.Pi/6 + float64(i)/16*(math.Pi+math.Pi/3)
		pts = append(pts, 12+6*math.Cos(a), 14.5+6*math.Sin(a))
	}
	p.poly(true, pts...)
}

// Wind — ветер.
func Wind(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.line(3, 9, 15, 9)
	p.arc(15, 6.5, 2.5, 90, 360)
	p.line(3, 13, 19, 13)
	p.arc(19, 15.5, 2.5, -360, -90)
	p.line(3, 17, 11, 17)
}

// Book — книга: вернуться к чтению в Nickel.
func Book(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.poly(true, 12, 6, 12, 20, 3, 18, 3, 4)
	p.poly(true, 12, 6, 12, 20, 21, 18, 21, 4)
}

// Dice — кубик: случайное.
func Dice(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.roundRect(4, 4, 20, 20, 3)
	for _, d := range [][2]float64{{8.5, 8.5}, {12, 12}, {15.5, 15.5}} {
		p.disc(d[0], d[1], 1.5)
	}
}

// Bell — колокольчик: сигнал таймера.
func Bell(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	p.arc(12, 10.5, 5.5, 180, 360)
	p.poly(false, 6.5, 10.5, 6.5, 15, 4.5, 17.5, 19.5, 17.5, 17.5, 15, 17.5, 10.5)
	p.arc(12, 19.5, 2, 0, 180)
	p.line(12, 3.5, 12, 5)
}

// Hand — ладонь: сдвинуть.
func Hand(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	for _, x := range []float64{8, 11, 14} {
		p.line(x, 12, x, 5.5)
	}
	p.line(17, 12, 17, 7.5)
	p.poly(false, 8, 12, 5.5, 10.5, 4.5, 12, 8.5, 18.5, 11, 21, 16, 21, 17, 18, 17, 12)
}

// Play и Pause — запуск и пауза таймера.
func Play(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).fill(7.5, 4.5, 19.5, 12, 7.5, 19.5)
}

func Pause(c *gfx.Canvas, r image.Rectangle, v uint8) {
	p := newPen(c, r, v)
	thick := p
	thick.w = p.w * 1.8
	thick.line(8.5, 5.5, 8.5, 18.5)
	thick.line(15.5, 5.5, 15.5, 18.5)
}

// Stop — квадрат: остановить.
func Stop(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).fill(6, 6, 18, 6, 18, 18, 6, 18)
}

// Minus — минус.
func Minus(c *gfx.Canvas, r image.Rectangle, v uint8) {
	newPen(c, r, v).line(4.5, 12, 19.5, 12)
}
