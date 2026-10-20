// Package calc — калькулятор: выражение целиком со скобками и приоритетом действий,
// результат виден ещё до «=», проценты как в обычных калькуляторах, история вычислений.
package calc

import (
	"encoding/json"
	"image"
	"strings"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/kit"
	"el-book/sdk/ui"
)

// App — приложение «Калькулятор»; оно же его экран.
type App struct {
	expr    string
	result  string // показанный после «=» результат
	errText string
	done    bool // только что нажали «=»: следующая цифра начинает новое выражение
	history []entry
	page    ui.Page
	display *ui.Custom
}

type entry struct {
	Expr   string `json:"expr"`
	Result string `json:"result"`
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "calc", Title: "Калькулятор", Icon: icons.Calculator}
}

const historyFile = "history.json"

func (a *App) Start(ctx *app.Context) app.Screen {
	if data, err := ctx.Storage().ReadFile(historyFile); err == nil {
		_ = json.Unmarshal(data, &a.history)
	}
	return a
}

func (a *App) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	a.page.Clear()
	r := b
	bar := ui.NewTitleBar(t, "Калькулятор", func() { ctx.Home() },
		&ui.Button{Icon: icons.Clock, Style: ui.Flat, OnTap: func() { a.openHistory(ctx) }})
	bar.BackIcon = icons.Home
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	a.display = &ui.Custom{Paint: a.paintDisplay}
	a.display.SetBounds(ui.Pad(ui.CutTop(&r, t.MM(40)), t.Pad(), t.MM(3), t.Pad(), t.MM(2)))
	keys := [][]string{
		{"C", "()", "%", "÷"},
		{"7", "8", "9", "×"},
		{"4", "5", "6", "−"},
		{"1", "2", "3", "+"},
		{"⌫", "0", ",", "="},
	}
	pad := ui.Pad(r, t.Pad(), t.MM(2), t.Pad(), t.Pad())
	rows := ui.Rows(pad, len(keys), t.Gap())
	a.page.Add(bar, a.display)
	for i, row := range keys {
		cols := ui.Columns(rows[i], len(row), t.Gap())
		for j, k := range row {
			k := k
			btn := &ui.Button{Text: k, Face: keyFace, OnTap: func() { a.press(ctx, k) }}
			switch {
			case k == "=":
				btn.Style = ui.Primary
			case strings.ContainsAny(k, "÷×−+%()C⌫"):
				btn.Style = ui.Flat
			}
			if k == "⌫" {
				btn.Text, btn.Icon = "", icons.Backspace
			}
			btn.SetBounds(cols[j])
			a.page.Add(btn)
		}
	}
}

// keyFace — подписи клавиш: крупно, как на настоящем калькуляторе.
func keyFace(t *ui.Theme) *gfx.Face { return t.Face(gfx.Medium, 22) }

func (a *App) Draw(c *gfx.Canvas, t *ui.Theme) {
	a.page.Draw(c, t)
	c.Fill(image.Rect(a.display.R.Min.X, a.display.R.Max.Y, a.display.R.Max.X, a.display.R.Max.Y+t.Line()), gfx.Gray16(10))
}

func (a *App) Handle(ctx *app.Context, ev app.Event) {
	if k, ok := ev.(device.KeyEvent); ok && k.Pressed && k.Key == device.KeyPageBack {
		a.press(ctx, "⌫")
		return
	}
	app.RoutePage(ctx, &a.page, ev)
}

// isOp — знак действия.
func isOp(r rune) bool { return strings.ContainsRune("÷×−+", r) }

func lastRune(s string) rune {
	r := []rune(s)
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}

// press — нажатие клавиши.
func (a *App) press(ctx *app.Context, k string) {
	a.errText = ""
	switch k {
	case "C":
		a.expr, a.result, a.done = "", "", false
	case "⌫":
		if a.done {
			a.expr, a.result, a.done = "", "", false
		} else if r := []rune(a.expr); len(r) > 0 {
			a.expr = string(r[:len(r)-1])
		}
	case "=":
		a.equals(ctx)
	case "()":
		a.paren()
	default:
		r := []rune(k)[0]
		if a.done {
			// После «=» знак действия продолжает с результатом, цифра — начинает заново.
			if !isOp(r) && r != '%' {
				a.expr = ""
			}
			a.done, a.result = false, ""
		}
		last := lastRune(a.expr)
		switch {
		case isOp(r) && isOp(last):
			a.expr = string([]rune(a.expr)[:len([]rune(a.expr))-1]) + k // знак заменяет знак
		case isOp(r) && a.expr == "" && r != '−':
			return // выражение не начинается с умножения
		case r == ',' && !strings.ContainsRune("0123456789", last):
			a.expr += "0,"
		case r == ',' && strings.ContainsRune(currentNumber(a.expr), ','):
			return // вторая запятая в числе
		default:
			a.expr += k
		}
	}
	ctx.Invalidate(a.display.R, eink.UI)
}

// currentNumber — число, которое сейчас набирается.
func currentNumber(expr string) string {
	r := []rune(expr)
	i := len(r)
	for i > 0 && strings.ContainsRune("0123456789,", r[i-1]) {
		i--
	}
	return string(r[i:])
}

// paren ставит открывающую скобку или закрывающую, если есть что закрыть.
func (a *App) paren() {
	if a.done {
		a.done, a.result = false, ""
	}
	open := strings.Count(a.expr, "(") - strings.Count(a.expr, ")")
	last := lastRune(a.expr)
	if open > 0 && last != '(' && !isOp(last) {
		a.expr += ")"
		return
	}
	if last != 0 && !isOp(last) && last != '(' {
		a.expr += "×" // «2(» — умножение на скобку
	}
	a.expr += "("
}

func (a *App) equals(ctx *app.Context) {
	if a.expr == "" || a.done {
		return
	}
	v, err := eval(a.expr)
	if err != nil {
		a.errText = err.Error()
		return
	}
	a.result = format(v)
	a.history = append(a.history, entry{Expr: a.expr, Result: a.result})
	if len(a.history) > 50 {
		a.history = a.history[len(a.history)-50:]
	}
	data, _ := json.Marshal(a.history)
	if err := ctx.Storage().WriteFile(historyFile, data); err != nil {
		ctx.Log().Warn("история калькулятора", "err", err)
	}
	// Результат — начало следующего выражения: без узких пробелов между тысячами.
	// Число с порядком («1,5·10^20») продолжить нельзя.
	a.expr = strings.ReplaceAll(a.result, "\u202f", "")
	if strings.Contains(a.expr, "·") {
		a.expr = ""
	}
	a.done = true
}

func (a *App) paintDisplay(c *gfx.Canvas, t *ui.Theme, r image.Rectangle) {
	exprFace := t.Face(gfx.Regular, 18)
	big := t.Face(gfx.Medium, 40)
	shown := a.expr
	if a.done && len(a.history) > 0 {
		shown = a.history[len(a.history)-1].Expr + " ="
	}
	lines := exprFace.Wrap(shown, r.Dx())
	if len(lines) > 2 {
		lines = lines[len(lines)-2:]
	}
	y := r.Min.Y
	for _, l := range lines {
		exprFace.DrawIn(c, l, image.Rect(r.Min.X, y, r.Max.X, y+exprFace.Height), gfx.AlignRight, gfx.Gray16(3))
		y += exprFace.Height
	}
	resultR := image.Rect(r.Min.X, r.Max.Y-big.Height, r.Max.X, r.Max.Y)
	switch {
	case a.errText != "":
		t.Header().DrawIn(c, a.errText, resultR, gfx.AlignRight, gfx.Black)
	case a.done:
		big.DrawIn(c, big.Ellipsize(a.result, r.Dx()), resultR, gfx.AlignRight, gfx.Black)
	case a.expr != "":
		// Предварительный результат серым: видно, что получится, ещё до «=».
		if v, err := eval(a.expr); err == nil {
			big.DrawIn(c, big.Ellipsize(format(v), r.Dx()), resultR, gfx.AlignRight, gfx.Gray16(8))
		}
	default:
		big.DrawIn(c, "0", resultR, gfx.AlignRight, gfx.Gray16(8))
	}
}

// openHistory — прошлые вычисления; тап подставляет результат в выражение.
func (a *App) openHistory(ctx *app.Context) {
	ctx.Push(&kit.ListScreen{
		Title: "История",
		Empty: "Здесь появятся вычисления после «=».",
		Items: func(ctx *app.Context) []ui.ListItem {
			var items []ui.ListItem
			for i := len(a.history) - 1; i >= 0; i-- {
				h := a.history[i]
				items = append(items, ui.ListItem{Title: h.Result, Subtitle: h.Expr, OnTap: func() {
					value := strings.ReplaceAll(h.Result, "\u202f", "")
					if a.done || a.expr == "" {
						a.expr = value
					} else if isOp(lastRune(a.expr)) || lastRune(a.expr) == '(' {
						a.expr += value
					} else {
						a.expr = value
					}
					a.done, a.result = false, ""
					ctx.Pop()
				}})
			}
			return items
		},
	})
}
