// Package sudoku — судоку: три сложности, головоломка всегда с единственным решением,
// заметки карандашом, подсветка ошибок, подсказка, отмена, счётчик оставшихся цифр.
// Партия сохраняется и продолжается после выхода.
package sudoku

import (
	"fmt"
	"image"
	"strconv"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ru"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// App — приложение «Судоку»; оно же его экран.
type App struct {
	g      *kit.Prefs[game]
	page   ui.Page
	bar    *ui.TitleBar
	board  *board
	digits [9]*ui.Button
	pencil *ui.Button
	shown  time.Time // когда партия показана: от него считается время игры
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "sudoku", Title: "Судоку", Icon: icons.Sudoku}
}

const gameVersion = 1

// game — партия.
type game struct {
	Level    Level     `json:"level"`
	Puzzle   Grid      `json:"puzzle"`
	Solution Grid      `json:"solution"`
	Cells    Grid      `json:"cells"` // цифры игрока и подсказки
	Notes    [81]int16 `json:"notes"` // заметки: биты 1…9
	Selected int       `json:"selected"`
	Pencil   bool      `json:"pencil"`
	Elapsed  int64     `json:"elapsed"` // секунд в игре
	Solved   bool      `json:"solved"`
	Hints    int       `json:"hints"`
	History  []move    `json:"history"`
}

// move — ход для отмены: что было в клетке до него.
type move struct {
	Cell  int   `json:"c"`
	Value int8  `json:"v"`
	Notes int16 `json:"n"`
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.g = kit.LoadPrefs(ctx.Storage(), gameVersion, game{})
	if a.g.Value.Puzzle == (Grid{}) {
		a.newGame(ctx, Easy, false)
	}
	return a
}

func (a *App) save(ctx *app.Context) {
	if err := a.g.Save(); err != nil {
		ctx.Log().Error("судоку", "err", err)
	}
}

// newGame строит головоломку; announce — перерисовать экран.
func (a *App) newGame(ctx *app.Context, level Level, announce bool) {
	puzzle, solution := Generate(level, uint64(ctx.Now().UnixNano()))
	a.g.Value = game{Level: level, Puzzle: puzzle, Solution: solution, Cells: puzzle, Selected: -1}
	a.shown = ctx.Now()
	a.save(ctx)
	if announce {
		a.updateAll(ctx)
		ctx.InvalidateAll(eink.Content)
	}
}

func (a *App) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	a.page.Clear()
	r := b
	a.bar = ui.NewTitleBar(t, "Судоку", func() { ctx.Home() },
		&ui.Button{Icon: icons.Undo, Style: ui.Flat, OnTap: func() { a.undo(ctx) }},
		&ui.Button{Icon: icons.Plus, Style: ui.Flat, OnTap: func() { a.askNew(ctx) }},
	)
	a.bar.BackIcon = icons.Home
	a.bar.SetBounds(ui.CutTop(&r, t.Bar()))
	r = ui.Pad(r, t.Pad(), t.MM(3), t.Pad(), t.Pad())
	padH := 2*t.Touch() + t.Gap() + t.MM(14)
	side := min(r.Dx(), r.Dy()-padH)
	a.board = &board{a: a}
	a.board.SetBounds(image.Rect(r.Min.X+(r.Dx()-side)/2, r.Min.Y, r.Min.X+(r.Dx()+side)/2, r.Min.Y+side))
	a.page.Add(a.bar, a.board)
	keys := image.Rect(r.Min.X, r.Min.Y+side+t.MM(5), r.Max.X, r.Min.Y+side+t.MM(5)+2*t.Touch()+t.Gap())
	rows := ui.Rows(keys, 2, t.Gap())
	top := ui.Columns(rows[0], 6, t.Gap())
	bottom := ui.Columns(rows[1], 6, t.Gap())
	for d := 1; d <= 9; d++ {
		d := d
		btn := &ui.Button{Text: strconv.Itoa(d), Face: func(t *ui.Theme) *gfx.Face { return t.Face(gfx.Medium, 18) },
			OnTap: func() { a.put(ctx, int8(d)) }}
		if d <= 5 {
			btn.SetBounds(top[d-1])
		} else {
			btn.SetBounds(bottom[d-6])
		}
		a.digits[d-1] = btn
		a.page.Add(btn)
	}
	erase := &ui.Button{Icon: icons.Eraser, Style: ui.Flat, OnTap: func() { a.put(ctx, 0) }}
	erase.SetBounds(top[5])
	a.pencil = &ui.Button{Icon: icons.Pencil, Style: ui.Flat, OnTap: func() {
		a.g.Value.Pencil = !a.g.Value.Pencil
		a.save(ctx)
		a.updateKeys(ctx)
		if a.g.Value.Pencil {
			ctx.Toast("Заметки: цифры ставятся мелко, для себя")
		}
	}}
	a.pencil.SetBounds(bottom[4])
	hint := &ui.Button{Icon: icons.Sparkle, Style: ui.Flat, OnTap: func() { a.hint(ctx) }}
	hint.SetBounds(bottom[5])
	a.page.Add(erase, a.pencil, hint)
	a.updateAll(ctx)
}

func (a *App) Draw(c *gfx.Canvas, t *ui.Theme) { a.page.Draw(c, t) }

func (a *App) Handle(ctx *app.Context, ev app.Event) {
	if e, ok := ev.(app.Lifecycle); ok {
		switch e {
		case app.Shown, app.Resumed:
			a.shown = ctx.Now()
		case app.Hidden, app.Paused:
			a.addTime(ctx)
			a.save(ctx)
		}
	}
	app.RoutePage(ctx, &a.page, ev)
}

// addTime прибавляет к партии время с показа экрана.
func (a *App) addTime(ctx *app.Context) {
	if !a.shown.IsZero() && !a.g.Value.Solved {
		a.g.Value.Elapsed += int64(ctx.Now().Sub(a.shown).Seconds())
	}
	a.shown = ctx.Now()
}

func (a *App) updateAll(ctx *app.Context) {
	a.updateTitle(ctx)
	a.updateKeys(ctx)
	ctx.Invalidate(a.board.R, eink.UI)
}

func (a *App) updateTitle(ctx *app.Context) {
	g := &a.g.Value
	sub := g.Level.title()
	if g.Solved {
		sub += " · решено за " + ru.Duration(time.Duration(g.Elapsed)*time.Second)
	} else if left := a.left(); left > 0 {
		sub += " · ещё " + ru.Count(left, "клетка", "клетки", "клеток") // «ещё» согласуется с любым числом
	}
	a.bar.SetTitle(ctx, "Судоку", sub)
}

func (a *App) left() int {
	n := 0
	for _, v := range a.g.Value.Cells {
		if v == 0 {
			n++
		}
	}
	return n
}

// updateKeys — сколько каждой цифры ещё ставить; поставленные все девять гасятся.
func (a *App) updateKeys(ctx *app.Context) {
	g := &a.g.Value
	var count [10]int
	for _, v := range g.Cells {
		count[v]++
	}
	for d := 1; d <= 9; d++ {
		btn := a.digits[d-1]
		btn.Disabled = count[d] >= 9 || g.Solved
		ctx.Invalidate(btn.R, eink.UI)
	}
	a.pencil.Active = g.Pencil
	ctx.Invalidate(a.pencil.R, eink.UI)
}

// put ставит цифру (0 — стирает) в выбранную клетку: заметкой или цифрой.
func (a *App) put(ctx *app.Context, d int8) {
	g := &a.g.Value
	i := g.Selected
	if g.Solved || i < 0 || g.Puzzle[i] != 0 {
		if i < 0 {
			ctx.Toast("Выберите клетку")
		}
		return
	}
	g.History = append(g.History, move{Cell: i, Value: g.Cells[i], Notes: g.Notes[i]})
	if len(g.History) > 200 {
		g.History = g.History[len(g.History)-200:]
	}
	switch {
	case d == 0:
		g.Cells[i], g.Notes[i] = 0, 0
	case g.Pencil && g.Cells[i] == 0:
		g.Notes[i] ^= 1 << d
	default:
		g.Cells[i] = d
		g.Notes[i] = 0
		a.clearNotes(i, d)
	}
	a.after(ctx)
}

// clearNotes убирает цифру d из заметок в строке, столбце и квадрате клетки i.
func (a *App) clearNotes(i int, d int8) {
	g := &a.g.Value
	r, c := i/9, i%9
	for k := 0; k < 81; k++ {
		kr, kc := k/9, k%9
		if kr == r || kc == c || (kr/3 == r/3 && kc/3 == c/3) {
			g.Notes[k] &^= 1 << d
		}
	}
}

func (a *App) after(ctx *app.Context) {
	g := &a.g.Value
	if g.Cells == g.Solution && !g.Solved {
		a.addTime(ctx)
		g.Solved = true
		msg := fmt.Sprintf("%s уровень за %s.", g.Level.title(), ru.Duration(time.Duration(g.Elapsed)*time.Second))
		if g.Hints > 0 {
			msg += " Подсказок: " + strconv.Itoa(g.Hints) + "."
		}
		ctx.ShowOverlay(ui.NewDialog(ctx.Theme(), "Решено!", msg,
			ui.DialogButton{Text: "Посмотреть"},
			ui.DialogButton{Text: "Новая игра", Primary: true, OnTap: func() { a.askNew(ctx) }}))
	}
	a.save(ctx)
	a.updateAll(ctx)
}

func (a *App) undo(ctx *app.Context) {
	g := &a.g.Value
	if len(g.History) == 0 || g.Solved {
		ctx.Toast("Отменять нечего")
		return
	}
	m := g.History[len(g.History)-1]
	g.History = g.History[:len(g.History)-1]
	g.Cells[m.Cell], g.Notes[m.Cell] = m.Value, m.Notes
	g.Selected = m.Cell
	a.after(ctx)
}

// hint открывает верную цифру выбранной клетки (или первой пустой).
func (a *App) hint(ctx *app.Context) {
	g := &a.g.Value
	if g.Solved {
		return
	}
	i := g.Selected
	if i < 0 || g.Cells[i] == g.Solution[i] {
		i = -1
		for k := range g.Cells {
			if g.Cells[k] != g.Solution[k] {
				i = k
				break
			}
		}
	}
	if i < 0 {
		return
	}
	g.History = append(g.History, move{Cell: i, Value: g.Cells[i], Notes: g.Notes[i]})
	g.Cells[i], g.Notes[i] = g.Solution[i], 0
	g.Selected = i
	g.Hints++
	a.clearNotes(i, g.Solution[i])
	a.after(ctx)
}

func (a *App) askNew(ctx *app.Context) {
	ctx.Menu("Новая игра",
		ui.MenuItem{Text: Easy.title(), OnTap: func() { a.newGame(ctx, Easy, true) }},
		ui.MenuItem{Text: Medium.title(), OnTap: func() { a.newGame(ctx, Medium, true) }},
		ui.MenuItem{Text: Hard.title(), OnTap: func() { a.newGame(ctx, Hard, true) }},
	)
}

// board — поле 9 × 9.
type board struct {
	ui.Box
	a *App
}

func (b *board) cell(i int) image.Rectangle {
	side := b.R.Dx()
	r, c := i/9, i%9
	return image.Rect(b.R.Min.X+c*side/9, b.R.Min.Y+r*side/9, b.R.Min.X+(c+1)*side/9, b.R.Min.Y+(r+1)*side/9)
}

func (b *board) Enabled() bool    { return true }
func (b *board) ShowsPress() bool { return false }

func (b *board) Tap(h ui.Host, p image.Point) {
	g := &b.a.g.Value
	side := b.R.Dx()
	c := (p.X - b.R.Min.X) * 9 / side
	r := (p.Y - b.R.Min.Y) * 9 / side
	if r < 0 || r > 8 || c < 0 || c > 8 {
		return
	}
	g.Selected = r*9 + c
	h.Invalidate(b.R, eink.UI)
}

func (b *board) Draw(c *gfx.Canvas, t *ui.Theme) {
	g := &b.a.g.Value
	var sel int8
	if g.Selected >= 0 {
		sel = g.Cells[g.Selected]
	}
	big := t.Face(gfx.Regular, 22)
	given := t.Face(gfx.Bold, 22)
	small := t.Face(gfx.Regular, 8)
	for i := 0; i < 81; i++ {
		r := b.cell(i)
		v := g.Cells[i]
		// Ошибка — цифра противоречит строке, столбцу или квадрату — видна сразу: светлая
		// цифра на тёмной клетке.
		bad := v != 0 && g.Puzzle[i] == 0 && conflict(&g.Cells, i)
		switch {
		case bad:
			c.Fill(r, gfx.Gray16(4))
			if i == g.Selected {
				c.StrokeRoundRect(r.Inset(t.MM(1)), t.MM(1), t.Line(), gfx.White)
			}
		case i == g.Selected:
			c.Fill(r, gfx.Gray16(11))
		case sel != 0 && v == sel:
			c.Fill(r, gfx.Gray16(14)) // та же цифра в других клетках
		}
		if v != 0 {
			face, fg := big, gfx.Black
			if g.Puzzle[i] != 0 {
				face = given
			}
			if bad {
				fg = gfx.White
			}
			face.DrawIn(c, strconv.Itoa(int(v)), r, gfx.AlignCenter, fg)
			continue
		}
		if n := g.Notes[i]; n != 0 {
			for d := 1; d <= 9; d++ {
				if n&(1<<d) == 0 {
					continue
				}
				sub := image.Rect(r.Min.X+(d-1)%3*r.Dx()/3, r.Min.Y+(d-1)/3*r.Dy()/3, r.Min.X+((d-1)%3+1)*r.Dx()/3, r.Min.Y+((d-1)/3+1)*r.Dy()/3)
				small.DrawIn(c, strconv.Itoa(d), sub, gfx.AlignCenter, gfx.Gray16(3))
			}
		}
	}
	side := b.R.Dx()
	for k := 0; k <= 9; k++ {
		w := t.Line()
		if k%3 == 0 {
			w = t.Line() * 3
		}
		x := b.R.Min.X + k*side/9
		y := b.R.Min.Y + k*side/9
		c.Fill(image.Rect(x-w/2, b.R.Min.Y, x-w/2+w, b.R.Max.Y), gfx.Black)
		c.Fill(image.Rect(b.R.Min.X, y-w/2, b.R.Max.X, y-w/2+w), gfx.Black)
	}
}

// conflict — повторяет ли цифра клетки i цифру в своей строке, столбце или квадрате.
func conflict(cells *Grid, i int) bool {
	v := cells[i]
	r, c := i/9, i%9
	for k := 0; k < 81; k++ {
		if k == i || cells[k] != v {
			continue
		}
		kr, kc := k/9, k%9
		if kr == r || kc == c || (kr/3 == r/3 && kc/3 == c/3) {
			return true
		}
	}
	return false
}

// Summary — недорешённая партия на домашнем экране.
func (a *App) Summary(st *storage.App, now time.Time) (app.Summary, bool) {
	p := kit.LoadPrefs(st, gameVersion, game{})
	g := &p.Value
	if g.Puzzle == (Grid{}) || g.Solved || g.Cells == g.Puzzle {
		return app.Summary{}, false
	}
	left := 0
	for _, v := range g.Cells {
		if v == 0 {
			left++
		}
	}
	return app.Summary{Text: fmt.Sprintf("Судоку: %s, ещё %s", lowerFirst(g.Level.title()), ru.Count(left, "клетка", "клетки", "клеток"))}, true
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 && r[0] >= 'А' && r[0] <= 'Я' {
		r[0] += 'а' - 'А'
	}
	return string(r)
}
