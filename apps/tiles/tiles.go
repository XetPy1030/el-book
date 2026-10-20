// Package tiles — игра 2048: свайп сдвигает плитки, одинаковые складываются. Счёт и рекорд,
// отмена последнего хода, партия сохраняется. Плитки различаются оттенком: чем больше
// число, тем темнее.
package tiles

import (
	"fmt"
	"image"
	"math/rand/v2"
	"strconv"

	"el-book/sdk/app"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ui"
)

// App — приложение «2048»; оно же его экран.
type App struct {
	g     *kit.Prefs[game]
	page  ui.Page
	bar   *ui.TitleBar
	board *board
	rng   *rand.Rand
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "2048", Title: "2048", Icon: icons.Tiles}
}

const gameVersion = 1

// game — партия: 16 клеток построчно, 0 — пусто.
type game struct {
	Cells [16]int  `json:"cells"`
	Score int      `json:"score"`
	Best  int      `json:"best"`
	Prev  *[16]int `json:"prev,omitempty"` // до последнего хода: для отмены
	PrevS int      `json:"prev_score"`
	Won   bool     `json:"won"` // 2048 уже собрали: поздравление не повторять
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.g = kit.LoadPrefs(ctx.Storage(), gameVersion, game{})
	a.rng = rand.New(rand.NewPCG(uint64(ctx.Now().UnixNano()), 7))
	if a.g.Value.Cells == ([16]int{}) {
		a.spawn()
		a.spawn()
	}
	return a
}

func (a *App) save(ctx *app.Context) {
	if err := a.g.Save(); err != nil {
		ctx.Log().Error("2048", "err", err)
	}
}

// spawn ставит 2 (или изредка 4) в случайную пустую клетку.
func (a *App) spawn() {
	g := &a.g.Value
	var empty []int
	for i, v := range g.Cells {
		if v == 0 {
			empty = append(empty, i)
		}
	}
	if len(empty) == 0 {
		return
	}
	v := 2
	if a.rng.IntN(10) == 0 {
		v = 4
	}
	g.Cells[empty[a.rng.IntN(len(empty))]] = v
}

// slide сдвигает ряд из четырёх клеток к началу и складывает пары; возвращает очки.
func slide(row [4]int) ([4]int, int) {
	var out [4]int
	n, score := 0, 0
	for _, v := range row {
		if v == 0 {
			continue
		}
		if n > 0 && out[n-1] == v && out[n-1] > 0 {
			out[n-1] = -2 * v // отрицательное — уже сложено в этом ходу
			score += 2 * v
			continue
		}
		out[n] = v
		n++
	}
	for i := range out {
		if out[i] < 0 {
			out[i] = -out[i]
		}
	}
	return out, score
}

// move — ход в направлении dir; false — ничего не сдвинулось.
func move(cells [16]int, dir ui.Direction) ([16]int, int, bool) {
	var out [16]int
	total := 0
	for k := 0; k < 4; k++ {
		var idx [4]int
		for j := 0; j < 4; j++ {
			switch dir {
			case ui.DirLeft:
				idx[j] = k*4 + j
			case ui.DirRight:
				idx[j] = k*4 + 3 - j
			case ui.DirUp:
				idx[j] = j*4 + k
			case ui.DirDown:
				idx[j] = (3-j)*4 + k
			}
		}
		var row [4]int
		for j, i := range idx {
			row[j] = cells[i]
		}
		moved, score := slide(row)
		total += score
		for j, i := range idx {
			out[i] = moved[j]
		}
	}
	return out, total, out != cells
}

// stuck — ходов нет.
func stuck(cells [16]int) bool {
	for _, d := range []ui.Direction{ui.DirLeft, ui.DirRight, ui.DirUp, ui.DirDown} {
		if _, _, ok := move(cells, d); ok {
			return false
		}
	}
	return true
}

func (a *App) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	a.page.Clear()
	r := b
	a.bar = ui.NewTitleBar(t, "2048", func() { ctx.Home() },
		&ui.Button{Icon: icons.Undo, Style: ui.Flat, OnTap: func() { a.undo(ctx) }},
		&ui.Button{Icon: icons.Plus, Style: ui.Flat, OnTap: func() {
			ctx.Confirm("Новая игра?", "Текущая партия закончится.", "Начать", func() { a.restart(ctx) })
		}},
	)
	a.bar.BackIcon = icons.Home
	a.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	r = ui.Pad(r, t.Pad(), t.MM(6), t.Pad(), t.Pad())
	hint := &ui.Label{Text: "Свайпом сдвигайте плитки: одинаковые складываются. Соберите 2048.", Wrap: true,
		Align: gfx.AlignCenter, Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}
	hint.SetBounds(ui.CutBottom(&r, 2*t.Caption().Height))
	side := min(r.Dx(), r.Dy()-t.MM(4))
	a.board = &board{a: a}
	a.board.SetBounds(image.Rect(r.Min.X+(r.Dx()-side)/2, r.Min.Y, r.Min.X+(r.Dx()+side)/2, r.Min.Y+side))
	a.page.Add(a.bar, a.board, hint)
	a.updateTitle(ctx)
}

func (a *App) Draw(c *gfx.Canvas, t *ui.Theme) { a.page.Draw(c, t) }

func (a *App) Handle(ctx *app.Context, ev app.Event) {
	if g, ok := ev.(ui.Gesture); ok && g.Kind == ui.Swipe {
		a.play(ctx, g.Dir)
		return
	}
	app.RoutePage(ctx, &a.page, ev)
}

func (a *App) updateTitle(ctx *app.Context) {
	g := &a.g.Value
	a.bar.SetTitle(ctx, "2048", fmt.Sprintf("счёт %d · рекорд %d", g.Score, g.Best))
}

func (a *App) play(ctx *app.Context, dir ui.Direction) {
	g := &a.g.Value
	cells, score, ok := move(g.Cells, dir)
	if !ok {
		return
	}
	prev := g.Cells
	g.Prev, g.PrevS = &prev, g.Score
	g.Cells = cells
	g.Score += score
	g.Best = max(g.Best, g.Score)
	a.spawn()
	a.save(ctx)
	a.updateTitle(ctx)
	ctx.Invalidate(a.board.R, eink.UI)
	if !g.Won {
		for _, v := range g.Cells {
			if v >= 2048 {
				g.Won = true
				a.save(ctx)
				ctx.ShowOverlay(ui.NewDialog(ctx.Theme(), "2048!", "Собрали. Можно играть дальше — до 4096.",
					ui.DialogButton{Text: "Дальше", Primary: true}))
				return
			}
		}
	}
	if stuck(g.Cells) {
		ctx.ShowOverlay(ui.NewDialog(ctx.Theme(), "Ходов нет", fmt.Sprintf("Счёт %d, рекорд %d.", g.Score, g.Best),
			ui.DialogButton{Text: "Отменить ход", OnTap: func() { a.undo(ctx) }},
			ui.DialogButton{Text: "Заново", Primary: true, OnTap: func() { a.restart(ctx) }}))
	}
}

func (a *App) undo(ctx *app.Context) {
	g := &a.g.Value
	if g.Prev == nil {
		ctx.Toast("Отменять нечего")
		return
	}
	g.Cells, g.Score = *g.Prev, g.PrevS
	g.Prev = nil
	a.save(ctx)
	a.updateTitle(ctx)
	ctx.Invalidate(a.board.R, eink.UI)
}

func (a *App) restart(ctx *app.Context) {
	g := &a.g.Value
	*g = game{Best: g.Best}
	a.spawn()
	a.spawn()
	a.save(ctx)
	a.updateTitle(ctx)
	ctx.Invalidate(a.board.R, eink.Content)
}

// board — поле 4 × 4. Свайпы ловит экран целиком: сдвинуть плитки можно с любого места.
type board struct {
	ui.Box
	a *App
}

// shade — оттенок плитки: 2 — почти белая, 2048 и дальше — чёрная.
func shade(v int) (bg, fg uint8) {
	level := 0
	for x := v; x > 2; x /= 2 {
		level++
	}
	bg = gfx.Gray16(max(15-level*14/10, 0))
	fg = gfx.Black
	if level >= 5 {
		fg = gfx.White
	}
	return bg, fg
}

func (b *board) Draw(c *gfx.Canvas, t *ui.Theme) {
	g := &b.a.g.Value
	gap := t.MM(2)
	side := b.R.Dx()
	cell := (side - 5*gap) / 4
	c.FillRoundRect(b.R, t.MM(2), gfx.Gray16(12))
	for i, v := range g.Cells {
		x := b.R.Min.X + gap + i%4*(cell+gap)
		y := b.R.Min.Y + gap + i/4*(cell+gap)
		r := image.Rect(x, y, x+cell, y+cell)
		if v == 0 {
			c.FillRoundRect(r, t.MM(1.5), gfx.Gray16(14))
			continue
		}
		bg, fg := shade(v)
		c.FillRoundRect(r, t.MM(1.5), bg)
		if bg == gfx.White {
			c.StrokeRoundRect(r, t.MM(1.5), t.Line(), gfx.Gray16(8))
		}
		text := strconv.Itoa(v)
		pt := 30.0
		switch {
		case len(text) >= 4:
			pt = 20
		case len(text) == 3:
			pt = 25
		}
		t.Face(gfx.Bold, pt).DrawIn(c, text, r, gfx.AlignCenter, fg)
	}
}
