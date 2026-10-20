// Package app — контракт приложения и цикл событий платформы. Цикл владеет экраном:
// раздаёт события верхнему экрану, собирает повреждённые области, перерисовывает их и
// отправляет обновления на панель. Код приложения выполняется под перехватом паник:
// упавшее приложение закрывается, оболочка остаётся.
package app

import (
	"image"
	"time"

	"el-book/sdk/device"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// Коды выхода процесса; их понимает скрипт запуска scripts/device/launch.sh.
const (
	ExitNickel   = 0  // вернуть Nickel
	ExitPowerOff = 10 // выключить книгу
	ExitReboot   = 11 // перезагрузить книгу
	ExitCrash    = 70 // оболочка упала; Nickel возвращается
)

// Manifest — то, что оболочка знает о приложении до запуска.
type Manifest struct {
	ID        string // латиница, папка данных: "draw"
	Title     string // «Рисовалка»
	Icon      icons.Drawer
	Landscape bool // умеет работать в альбомной ориентации
}

// App — приложение платформы.
type App interface {
	Manifest() Manifest
	// Start вызывается при запуске и возвращает первый экран.
	Start(ctx *Context) Screen
}

// Stopper — приложение хочет знать, что его закрывают (сохранить данные).
type Stopper interface {
	Stop(ctx *Context)
}

// Pauser — приложение хочет сохраниться перед сном и выходом в Nickel.
type Pauser interface {
	Pause(ctx *Context)
}

// Summarizer — приложение показывает строку на домашнем экране, не запускаясь: погоду,
// задачи на сегодня, идущий таймер. Summary только читает данные приложения из хранилища —
// без сети и долгих вычислений: её зовут в цикле интерфейса при каждом показе домашнего экрана.
type Summarizer interface {
	Summary(st *storage.App, now time.Time) (Summary, bool)
}

// Summary — строка приложения на домашнем экране; тап по ней открывает приложение.
type Summary struct {
	Icon icons.Drawer // nil — иконка приложения
	Text string
}

// SleepPainter — приложение умеет рисовать экран сна: последний рисунок, план на день.
// Оболочка предлагает его в настройках сна. PaintSleep рисует на белом холсте во весь экран
// по данным из хранилища, не запуская приложение; false — рисовать нечего, оболочка покажет
// обычный экран сна.
type SleepPainter interface {
	PaintSleep(st *storage.App, c *gfx.Canvas, t *ui.Theme) bool
}

// Screen — экран: раскладывается под размер, рисует себя, получает события.
type Screen interface {
	// Layout вызывается перед первым показом и при смене размера экрана (поворот).
	Layout(ctx *Context, bounds image.Rectangle)
	// Draw рисует экран; холст уже залит белым и обрезан по перерисовываемой области.
	Draw(c *gfx.Canvas, t *ui.Theme)
	// Handle получает события: device.TouchEvent, device.KeyEvent, ui.Gesture, Lifecycle и др.
	Handle(ctx *Context, ev Event)
}

// Event — событие для экрана.
type Event any

// Lifecycle — событие жизненного цикла экрана.
type Lifecycle int

const (
	Shown   Lifecycle = iota // экран стал верхним
	Hidden                   // поверх открыли другой экран
	Paused                   // книга засыпает или выходит в Nickel
	Resumed                  // книга проснулась
)

// System — оболочка: корневой экран и системные события.
type System interface {
	// Home возвращает домашний экран; он всегда лежит в основании стека.
	Home(ctx *Context) Screen
	// Intercept получает события устройства раньше экранов; true — событие поглощено.
	Intercept(ctx *Context, ev device.Event) bool
	// Crashed сообщает, что приложение упало; его экраны уже убраны.
	Crashed(ctx *Context, m Manifest, err any, stack []byte)
}

// RoutePage передаёт касания и жесты в ui.Page. Возвращает true, если событие использовано.
func RoutePage(ctx *Context, p *ui.Page, ev Event) bool {
	switch e := ev.(type) {
	case device.TouchEvent:
		p.Touch(ctx, e)
		return true
	case ui.Gesture:
		return p.Gesture(ctx, e)
	}
	return false
}
