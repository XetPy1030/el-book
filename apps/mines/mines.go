// Package mines — сапёр: три размера поля, первый ход всегда безопасен, флажок долгим
// нажатием или в режиме флажков, тап по открытой цифре открывает соседей, когда флажков
// вокруг столько же. Партия сохраняется.
package mines

import (
	"fmt"
	"image"
	"math/rand/v2"
	"strconv"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/ui"
)

// App — приложение «Сапёр»; оно же его экран.
type App struct {
	g     *kit.Prefs[game]
	page  ui.Page
	bar   *ui.TitleBar
	field *field
	flag  *ui.Button
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "mines", Title: "Сапёр", Icon: icons.Mine}
}

// size — размер поля и число мин.
type size struct {
	Cols, Rows, Mines int
	Title             string
}

var sizes = []size{
	{9, 12, 14, "Маленькое"},
	{12, 16, 32, "Среднее"},
	{15, 20, 60, "Большое"},
}

// Состояния клетки.
const (
	closed  = 0
	opened  = 1
	flagged = 2
)

const gameVersion = 1

// game — партия.
type game struct {
	Size     int     `json:"size"`
	Mines    []bool  `json:"mines"` // пусто — мины ещё не расставлены: ждут первого хода
	State    []uint8 `json:"state"`
	FlagMode bool    `json:"flag_mode"`
	Over     bool    `json:"over"`
	Won      bool    `json:"won"`
	Exploded int     `json:"exploded"` // клетка, на которой подорвались
	Started  int64   `json:"started"`
	Finished int64   `json:"finished"`
}

func (g *game) sz() size { return sizes[min(max(g.Size, 0), len(sizes)-1)] }

func (a *App) Start(ctx *app.Context) app.Screen {
	a.g = kit.LoadPrefs(ctx.Storage(), gameVersion, game{})
	if len(a.g.Value.State) == 0 {
		a.reset(0)
	}
	return a
}

func (a *App) reset(sizeIndex int) {
	s := sizes[sizeIndex]
	a.g.Value = game{Size: sizeIndex, State: make([]uint8, s.Cols*s.Rows), Exploded: -1}
}

func (a *App) save(ctx *app.Context) {
	if err := a.g.Save(); err != nil {
		ctx.Log().Error("сапёр", "err", err)
	}
}

func (a *App) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	a.page.Clear()
	r := b
	a.flag = &ui.Button{Icon: icons.Flag, Style: ui.Flat, OnTap: func() {
		a.g.Value.FlagMode = !a.g.Value.FlagMode
		a.save(ctx)
		a.update(ctx, false)
		if a.g.Value.FlagMode {
			ctx.Toast("Режим флажков: тап ставит флажок")
		}
	}}
	a.bar = ui.NewTitleBar(t, "Сапёр", func() { ctx.Home() }, a.flag,
		&ui.Button{Icon: icons.Plus, Style: ui.Flat, OnTap: func() { a.askNew(ctx) }})
	a.bar.BackIcon = icons.Home
	a.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	r = ui.Pad(r, t.Pad(), t.MM(3), t.Pad(), t.Pad())
	s := a.g.Value.sz()
	cell := min(r.Dx()/s.Cols, r.Dy()/s.Rows)
	w, h := cell*s.Cols, cell*s.Rows
	a.field = &field{a: a, ctx: ctx, cell: cell}
	a.field.SetBounds(image.Rect(r.Min.X+(r.Dx()-w)/2, r.Min.Y, r.Min.X+(r.Dx()+w)/2, r.Min.Y+h))
	a.page.Add(a.bar, a.field)
	a.update(ctx, false)
}

func (a *App) Draw(c *gfx.Canvas, t *ui.Theme) { a.page.Draw(c, t) }

func (a *App) Handle(ctx *app.Context, ev app.Event) { app.RoutePage(ctx, &a.page, ev) }

func (a *App) update(ctx *app.Context, board bool) {
	g := &a.g.Value
	s := g.sz()
	flags := 0
	for _, st := range g.State {
		if st == flagged {
			flags++
		}
	}
	sub := fmt.Sprintf("%s · мин %d из %d", s.Title, s.Mines-flags, s.Mines)
	switch {
	case g.Won:
		sub = s.Title + " · победа за " + ru.Duration(time.Duration(g.Finished-g.Started)*time.Second)
	case g.Over:
		sub = s.Title + " · подорвались"
	}
	a.bar.SetTitle(ctx, "Сапёр", sub)
	a.flag.Active = g.FlagMode
	ctx.Invalidate(a.flag.R, eink.UI)
	if board {
		ctx.Invalidate(a.field.R, eink.UI)
	}
}

// plant расставляет мины после первого хода: в клетке first и вокруг неё мин нет.
func (a *App) plant(first int, seed uint64) {
	g := &a.g.Value
	s := g.sz()
	rng := rand.New(rand.NewPCG(seed, seed>>1|1))
	g.Mines = make([]bool, s.Cols*s.Rows)
	safe := map[int]bool{first: true}
	for _, n := range neighbors(first, s) {
		safe[n] = true
	}
	for placed := 0; placed < s.Mines; {
		i := rng.IntN(len(g.Mines))
		if g.Mines[i] || safe[i] {
			continue
		}
		g.Mines[i] = true
		placed++
	}
}

func neighbors(i int, s size) []int {
	r, c := i/s.Cols, i%s.Cols
	var out []int
	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			nr, nc := r+dr, c+dc
			if (dr != 0 || dc != 0) && nr >= 0 && nr < s.Rows && nc >= 0 && nc < s.Cols {
				out = append(out, nr*s.Cols+nc)
			}
		}
	}
	return out
}

func (g *game) around(i int) int {
	n := 0
	for _, k := range neighbors(i, g.sz()) {
		if g.Mines[k] {
			n++
		}
	}
	return n
}

// open открывает клетку; пустые открываются волной. Возвращает false, если там мина.
func (g *game) open(i int) bool {
	if g.State[i] != closed {
		return true
	}
	if g.Mines[i] {
		g.Exploded = i
		return false
	}
	stack := []int{i}
	for len(stack) > 0 {
		k := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if g.State[k] != closed {
			continue
		}
		g.State[k] = opened
		if g.around(k) == 0 {
			for _, n := range neighbors(k, g.sz()) {
				if g.State[n] == closed && !g.Mines[n] {
					stack = append(stack, n)
				}
			}
		}
	}
	return true
}

// tap — ход в клетку i. long — долгое нажатие: флажок.
func (a *App) tap(ctx *app.Context, i int, long bool) {
	g := &a.g.Value
	if g.Over {
		return
	}
	flag := long != g.FlagMode
	switch {
	case flag && g.State[i] != opened:
		if g.State[i] == flagged {
			g.State[i] = closed
		} else {
			g.State[i] = flagged
		}
	case g.State[i] == flagged:
		return
	case g.Mines == nil:
		a.plant(i, uint64(ctx.Now().UnixNano()))
		g.Started = ctx.Now().Unix()
		g.open(i)
	case g.State[i] == opened:
		// Аккорд: флажков вокруг столько же, сколько мин, — открыть остальных соседей.
		flags := 0
		for _, n := range neighbors(i, g.sz()) {
			if g.State[n] == flagged {
				flags++
			}
		}
		if flags != g.around(i) {
			return
		}
		for _, n := range neighbors(i, g.sz()) {
			if !g.open(n) {
				a.lose(ctx)
				return
			}
		}
	default:
		if !g.open(i) {
			a.lose(ctx)
			return
		}
	}
	a.checkWin(ctx)
	a.save(ctx)
	a.update(ctx, true)
}

func (a *App) lose(ctx *app.Context) {
	g := &a.g.Value
	g.Over = true
	g.Finished = ctx.Now().Unix()
	a.save(ctx)
	a.update(ctx, true)
	ctx.ShowOverlay(ui.NewDialog(ctx.Theme(), "Мина!", "Попробуйте ещё раз: первый ход всегда безопасен.",
		ui.DialogButton{Text: "Посмотреть"},
		ui.DialogButton{Text: "Заново", Primary: true, OnTap: func() { a.restart(ctx, g.Size) }}))
}

func (a *App) checkWin(ctx *app.Context) {
	g := &a.g.Value
	if g.Mines == nil {
		return
	}
	for i, st := range g.State {
		if st != opened && !g.Mines[i] {
			return
		}
	}
	g.Over, g.Won = true, true
	g.Finished = ctx.Now().Unix()
	for i := range g.State {
		if g.Mines[i] {
			g.State[i] = flagged
		}
	}
	ctx.ShowOverlay(ui.NewDialog(ctx.Theme(), "Победа!",
		fmt.Sprintf("Поле «%s» разминировано за %s.", lower(g.sz().Title), ru.Duration(time.Duration(g.Finished-g.Started)*time.Second)),
		ui.DialogButton{Text: "Посмотреть"},
		ui.DialogButton{Text: "Ещё раз", Primary: true, OnTap: func() { a.restart(ctx, g.Size) }}))
}

func lower(s string) string {
	r := []rune(s)
	if len(r) > 0 && r[0] >= 'А' && r[0] <= 'Я' {
		r[0] += 'а' - 'А'
	}
	return string(r)
}

func (a *App) restart(ctx *app.Context, sizeIndex int) {
	a.reset(sizeIndex)
	a.save(ctx)
	a.Layout(ctx, ctx.Bounds())
	ctx.InvalidateAll(eink.Content)
}

func (a *App) askNew(ctx *app.Context) {
	var items []ui.MenuItem
	for i, s := range sizes {
		i := i
		items = append(items, ui.MenuItem{Text: fmt.Sprintf("%s: %d × %d, мин %d", s.Title, s.Cols, s.Rows, s.Mines),
			Checked: i == a.g.Value.Size, OnTap: func() { a.restart(ctx, i) }})
	}
	ctx.Menu("Новая игра", items...)
}

// field — поле.
type field struct {
	ui.Box
	a    *App
	ctx  *app.Context
	cell int
}

func (f *field) at(p image.Point) int {
	s := f.a.g.Value.sz()
	c := (p.X - f.R.Min.X) / f.cell
	r := (p.Y - f.R.Min.Y) / f.cell
	if c < 0 || c >= s.Cols || r < 0 || r >= s.Rows {
		return -1
	}
	return r*s.Cols + c
}

func (f *field) Enabled() bool    { return true }
func (f *field) ShowsPress() bool { return false }

func (f *field) Tap(_ ui.Host, p image.Point) {
	if i := f.at(p); i >= 0 {
		f.a.tap(f.ctx, i, false)
	}
}

func (f *field) LongPress(_ ui.Host, p image.Point) {
	if i := f.at(p); i >= 0 {
		f.a.tap(f.ctx, i, true)
	}
}

func (f *field) Draw(c *gfx.Canvas, t *ui.Theme) {
	g := &f.a.g.Value
	s := g.sz()
	num := t.Face(gfx.Bold, float64(f.cell)*72/float64(t.DPI)*0.55)
	for i := range g.State {
		r := image.Rect(f.R.Min.X+i%s.Cols*f.cell, f.R.Min.Y+i/s.Cols*f.cell, f.R.Min.X+(i%s.Cols+1)*f.cell, f.R.Min.Y+(i/s.Cols+1)*f.cell)
		in := r.Inset(max(f.cell/24, 1))
		mine := g.Mines != nil && g.Mines[i]
		switch {
		case g.State[i] == opened:
			c.StrokeRect(r, 1, gfx.Gray16(12))
			if n := g.around(i); n > 0 {
				num.DrawIn(c, strconv.Itoa(n), r, gfx.AlignCenter, gfx.Black)
			}
		case g.Over && mine && g.State[i] != flagged:
			if i == g.Exploded {
				c.Fill(in, gfx.Black)
				c.Disc(float64(r.Min.X+r.Dx()/2), float64(r.Min.Y+r.Dy()/2), float64(f.cell)/4, gfx.White, true)
			} else {
				c.Fill(in, gfx.Gray16(12))
				c.Disc(float64(r.Min.X+r.Dx()/2), float64(r.Min.Y+r.Dy()/2), float64(f.cell)/4, gfx.Black, true)
			}
		default:
			c.FillRoundRect(in, f.cell/8, gfx.Gray16(10))
			if g.State[i] == flagged {
				icons.Flag(c, in.Inset(f.cell/5), gfx.Black)
				if g.Over && !g.Won && !mine {
					// Ошибочный флажок перечёркнут.
					c.Capsule(float64(in.Min.X), float64(in.Min.Y), float64(in.Max.X), float64(in.Max.Y), 2, gfx.Black, true)
				}
			}
		}
	}
}
