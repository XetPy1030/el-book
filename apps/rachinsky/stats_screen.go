package rachinsky

import (
	"image"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
)

// statsScreen — статистика на двух страницах: числа и графики.
type statsScreen struct {
	a    *App
	page ui.Page
	b    image.Rectangle
	pg   int
	now  time.Time
}

func (s *statsScreen) Layout(ctx *app.Context, b image.Rectangle) {
	s.b = b
	s.build(ctx)
}

func (s *statsScreen) build(ctx *app.Context) {
	t := ctx.Theme()
	s.now = ctx.Now()
	s.page.Clear()
	r := s.b
	bar := ui.NewTitleBar(t, "Статистика", func() { ctx.Pop() })
	bar.Subtitle = itoa(s.pg+1) + " / 2"
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	s.page.Add(bar)
	body := ui.Pad(r, t.Pad()+t.MM(2), t.MM(3), t.Pad()+t.MM(2), t.MM(3))
	paint := s.paintNumbers
	if s.pg == 1 {
		paint = s.paintCharts
	}
	cst := &ui.Custom{Paint: func(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) { paint(c, t, r) }}
	cst.SetBounds(body)
	s.page.Add(cst)
}

// paintNumbers — страница чисел: прогресс, точность, серии, темп.
func (s *statsScreen) paintNumbers(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	st := s.a.st
	y := r.Min.Y

	// Прогресс.
	big := t.Face(gfx.Bold, 40)
	big.Draw(c, itoa(st.solved()), r.Min.X, y+big.Ascent, gfx.Black)
	t.Face(gfx.Regular, 14).Draw(c, "из "+itoa(st.n)+" решено", r.Min.X+big.Measure(itoa(st.solved()))+t.MM(3), y+big.Ascent-t.MM(2), gfx.Gray16(3))
	y += big.Height + t.MM(1)
	barR := image.Rect(r.Min.X, y, r.Max.X, y+t.MM(4))
	drawBar(c, t, barR, float64(st.solved())/float64(st.n))
	y = barR.Max.Y + t.MM(2)
	sub := ru.Count(st.n-st.solved(), "задача осталась", "задачи осталось", "задач осталось")
	if f := st.flagged(); f > 0 {
		sub += " · отложено " + itoa(f)
	}
	t.Caption().Draw(c, sub, r.Min.X, y+t.Caption().Ascent, gfx.Gray16(4))
	y += t.Caption().Height + t.MM(6)

	// Точность.
	s.heading(c, t, r.Min.X, &y, "Точность")
	if acc := st.accuracy(); acc >= 0 {
		s.line(c, t, r, &y, "Верных ответов", itoa(st.correct)+" из "+itoa(st.attempts)+" ("+itoa(acc)+"%)")
	} else {
		s.line(c, t, r, &y, "Верных ответов", "пока нет попыток")
	}
	y += t.MM(4)

	// Серии.
	s.heading(c, t, r.Min.X, &y, "Серии")
	s.line(c, t, r, &y, "Верно подряд", itoa(st.curStreak)+"  (рекорд "+itoa(st.bestStreak)+")")
	s.line(c, t, r, &y, "Дней подряд", itoa(st.curDays)+"  (рекорд "+itoa(st.bestDays)+")")
	y += t.MM(4)

	// Темп.
	s.heading(c, t, r.Min.X, &y, "Темп")
	today := s.now.Format("2006-01-02")
	s.line(c, t, r, &y, "Сегодня решено", itoa(st.day[today].Solved))
	s.line(c, t, r, &y, "За 7 дней", itoa(s.lastDays(7)))
	s.line(c, t, r, &y, "За 30 дней", itoa(s.lastDays(30)))
}

// paintCharts — страница графиков: по сотням и активность за две недели.
func (s *statsScreen) paintCharts(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	st := s.a.st
	y := r.Min.Y

	s.heading(c, t, r.Min.X, &y, "По сотням задач")
	buckets := (st.n + 99) / 100
	gap := t.MM(2)
	bw := (r.Dx() - gap*(buckets-1)) / buckets
	chartTop := y
	chartH := t.MM(34)
	for k := 0; k < buckets; k++ {
		lo, hi := k*100, min((k+1)*100, st.n)
		solved := 0
		for i := lo; i < hi; i++ {
			if st.status[i] == stSolved {
				solved++
			}
		}
		frac := float64(solved) / float64(hi-lo)
		x0 := r.Min.X + k*(bw+gap)
		col := image.Rect(x0, chartTop, x0+bw, chartTop+chartH)
		c.StrokeRect(col, t.Line(), gfx.Gray16(9))
		fh := int(float64(chartH) * frac)
		if solved > 0 && fh < t.Line()*2 {
			fh = t.Line() * 2
		}
		c.Fill(image.Rect(col.Min.X, col.Max.Y-fh, col.Max.X, col.Max.Y), gfx.Black)
		t.Caption().DrawIn(c, itoa(k+1), image.Rect(col.Min.X, col.Max.Y+t.MM(1), col.Max.X, col.Max.Y+t.MM(1)+t.Caption().Height), gfx.AlignCenter, gfx.Gray16(5))
	}
	y = chartTop + chartH + t.MM(1) + t.Caption().Height + t.MM(7)

	s.heading(c, t, r.Min.X, &y, "Активность за 14 дней")
	days := 14
	gap = t.MM(1.5)
	bw = (r.Dx() - gap*(days-1)) / days
	chartTop = y
	chartH = t.MM(34)
	maxv := 1
	vals := make([]int, days)
	labels := make([]string, days)
	for i := 0; i < days; i++ {
		d := s.now.AddDate(0, 0, -(days - 1 - i))
		vals[i] = st.day[d.Format("2006-01-02")].Solved
		labels[i] = ru.WeekdayShort(d.Weekday())
		if vals[i] > maxv {
			maxv = vals[i]
		}
	}
	for i := 0; i < days; i++ {
		x0 := r.Min.X + i*(bw+gap)
		col := image.Rect(x0, chartTop, x0+bw, chartTop+chartH)
		fh := chartH * vals[i] / maxv
		if vals[i] > 0 && fh < t.Line()*2 {
			fh = t.Line() * 2
		}
		c.Fill(image.Rect(col.Min.X, col.Max.Y-fh, col.Max.X, col.Max.Y), gfx.Black)
		base := col.Max.Y
		c.Fill(image.Rect(col.Min.X, base, col.Max.X, base+t.Line()), gfx.Gray16(7))
		lblCol := uint8(gfx.Gray16(6))
		if i == days-1 {
			lblCol = gfx.Black
		}
		t.Caption().DrawIn(c, labels[i], image.Rect(col.Min.X, base+t.MM(1), col.Max.X, base+t.MM(1)+t.Caption().Height), gfx.AlignCenter, lblCol)
	}
}

func (s *statsScreen) lastDays(n int) int {
	total := 0
	for i := 0; i < n; i++ {
		d := s.now.AddDate(0, 0, -i).Format("2006-01-02")
		total += s.a.st.day[d].Solved
	}
	return total
}

func (s *statsScreen) heading(c *gfx.Canvas, t *ui.Theme, x int, y *int, text string) {
	f := t.Face(gfx.Bold, 13)
	f.Draw(c, text, x, *y+f.Ascent, gfx.Black)
	*y += f.Height + t.MM(1)
}

func (s *statsScreen) line(c *gfx.Canvas, t *ui.Theme, r image.Rectangle, y *int, label, value string) {
	f := t.Body()
	base := *y + f.Ascent
	f.Draw(c, label, r.Min.X+t.MM(2), base, gfx.Gray16(4))
	f.DrawIn(c, value, image.Rect(r.Min.X, *y, r.Max.X, *y+f.Height), gfx.AlignRight, gfx.Black)
	*y += f.Height + t.MM(1)
}

func (s *statsScreen) Draw(c *gfx.Canvas, t *ui.Theme) { s.page.Draw(c, t) }

func (s *statsScreen) flip(ctx *app.Context, pg int) {
	if pg < 0 || pg > 1 || pg == s.pg {
		return
	}
	s.pg = pg
	s.build(ctx)
	ctx.InvalidateAll(eink.Clean)
}

func (s *statsScreen) Handle(ctx *app.Context, ev app.Event) {
	switch e := ev.(type) {
	case device.KeyEvent:
		if e.Pressed {
			switch e.Key {
			case device.KeyPageForward:
				s.flip(ctx, s.pg+1)
				return
			case device.KeyPageBack:
				s.flip(ctx, s.pg-1)
				return
			}
		}
	case ui.Gesture:
		if e.Kind == ui.Swipe {
			switch e.Dir {
			case ui.DirLeft:
				s.flip(ctx, s.pg+1)
				return
			case ui.DirRight:
				s.flip(ctx, s.pg-1)
				return
			}
		}
	}
	app.RoutePage(ctx, &s.page, ev)
}
