// Package builtin — список встроенных приложений. Его импортируют только программы в cmd:
// приложения не знают друг о друге, оболочка получает их списком.
package builtin

import (
	"el-book/apps/calc"
	"el-book/apps/calendar"
	"el-book/apps/draw"
	"el-book/apps/mines"
	"el-book/apps/news"
	"el-book/apps/notes"
	"el-book/apps/sudoku"
	"el-book/apps/tiles"
	"el-book/apps/timer"
	"el-book/apps/todo"
	"el-book/apps/weather"
	"el-book/apps/wiki"
	"el-book/sdk/app"
)

// Apps возвращает встроенные приложения.
func Apps() []app.App {
	return []app.App{
		draw.New(),
		notes.New(),
		todo.New(),
		calendar.New(),
		weather.New(),
		news.New(),
		wiki.New(),
		calc.New(),
		timer.New(),
		sudoku.New(),
		mines.New(),
		tiles.New(),
	}
}
