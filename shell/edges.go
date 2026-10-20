package shell

import (
	"image"
	"time"

	"el-book/sdk/device"
)

// edge — край экрана, от которого начался жест оболочки.
type edge int

const (
	edgeNone   edge = iota
	edgeTop         // сверху вниз — быстрые настройки
	edgeBottom      // снизу вверх — домой
)

// edgeSwipe распознаёт жесты от верхнего и нижнего края. Касание первого пальца, начатое
// в узкой полосе у края, придерживается: приложение его пока не видит. Палец быстро ушёл
// от края — жест оболочки, касание до отпускания поглощается. Иначе — палец стоит, ушёл
// вбок, коснулся второй палец или прошло время — придержанные события отдаются приложению
// как были, и дальше касание идёт обычным путём.
type edgeSwipe struct {
	zone     int           // ширина полосы у края, пиксели
	distance int           // насколько уйти от края
	maxTime  time.Duration // за какое время

	held    []device.TouchEvent
	from    edge // край придержанного касания; edgeNone — ничего не держим
	start   device.TouchEvent
	swallow bool // жест сработал: остаток касания приложению не нужен
}

// feed принимает касание. consumed — событие забрано (придержано или часть жеста); replay —
// придержанные события, которые надо отдать приложению; fired — сработавший жест.
func (e *edgeSwipe) feed(ev device.TouchEvent, screen image.Rectangle) (consumed bool, replay []device.TouchEvent, fired edge) {
	if e.swallow {
		if ev.Slot == 0 && ev.Phase == device.TouchUp {
			e.swallow = false
		}
		return ev.Slot == 0, nil, edgeNone
	}
	if e.from == edgeNone {
		if ev.Slot != 0 || ev.Phase != device.TouchDown {
			return false, nil, edgeNone
		}
		switch {
		case ev.Y < screen.Min.Y+e.zone:
			e.from = edgeTop
		case ev.Y >= screen.Max.Y-e.zone:
			e.from = edgeBottom
		default:
			return false, nil, edgeNone
		}
		e.start = ev
		e.held = append(e.held[:0], ev)
		return true, nil, edgeNone
	}
	if ev.Slot != 0 {
		return true, e.release(ev), edgeNone // второй палец: это не жест
	}
	if ev.Phase == device.TouchUp {
		return true, e.release(ev), edgeNone // тап у края — приложению
	}
	inward := ev.Y - e.start.Y
	if e.from == edgeBottom {
		inward = -inward
	}
	sideways := abs(ev.X - e.start.X)
	late := !e.start.Time.IsZero() && ev.Time.Sub(e.start.Time) > e.maxTime
	switch {
	case inward >= e.distance && sideways <= inward && !late:
		fired = e.from
		e.from = edgeNone
		e.held = e.held[:0]
		e.swallow = true
		return true, nil, fired
	case late || sideways > e.distance:
		return true, e.release(ev), edgeNone
	}
	e.held = append(e.held, ev)
	return true, nil, edgeNone
}

// holding сообщает, придержано ли касание.
func (e *edgeSwipe) holding() bool { return e.from != edgeNone }

// expire отпускает касание, которое стоит у края дольше maxTime: долгое нажатие у края
// должно дойти до приложения, даже если палец не двигается.
func (e *edgeSwipe) expire() []device.TouchEvent {
	if e.from == edgeNone {
		return nil
	}
	return e.release()
}

// release отдаёт придержанные события вместе с extra и перестаёт держать.
func (e *edgeSwipe) release(extra ...device.TouchEvent) []device.TouchEvent {
	out := append(append([]device.TouchEvent(nil), e.held...), extra...)
	e.held = e.held[:0]
	e.from = edgeNone
	return out
}

// reset забывает придержанное касание: книга уснула, экран сменился.
func (e *edgeSwipe) reset() {
	e.held = e.held[:0]
	e.from = edgeNone
	e.swallow = false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
