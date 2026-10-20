package ui

import (
	"image"
	"time"

	"el-book/sdk/device"
)

// GestureKind — вид жеста.
type GestureKind int

const (
	Tap GestureKind = iota
	LongPress
	PanStart
	PanMove
	PanEnd
	Swipe
)

func (k GestureKind) String() string {
	return [...]string{"tap", "longpress", "panstart", "panmove", "panend", "swipe"}[k]
}

// Direction — направление свайпа.
type Direction int

const (
	DirLeft Direction = iota
	DirRight
	DirUp
	DirDown
)

// Gesture — распознанный жест первого пальца.
type Gesture struct {
	Kind  GestureKind
	Pos   image.Point // текущая точка
	Start image.Point // где палец опустился
	Dir   Direction   // для Swipe
	Time  time.Time
}

// Recognizer собирает жесты из касаний первого пальца: тап, долгое нажатие, перетаскивание, свайп.
// Остальные пальцы игнорируются.
type Recognizer struct {
	Slop          int           // смещение, после которого касание — уже перетаскивание
	SwipeMin      int           // минимальная длина свайпа
	LongPressTime time.Duration // время до долгого нажатия
	SwipeMaxTime  time.Duration // свайп дольше этого — просто перетаскивание

	schedule func(time.Duration, func()) (cancel func())
	emit     func(Gesture)

	active    bool
	slot      int
	start     image.Point
	startTime time.Time
	last      image.Point
	panning   bool
	longFired bool
	cancelLP  func()
}

// NewRecognizer создаёт распознаватель с порогами по теме. schedule откладывает функцию
// (в горутине интерфейса), emit получает жесты.
func NewRecognizer(t *Theme, schedule func(time.Duration, func()) func(), emit func(Gesture)) *Recognizer {
	return &Recognizer{
		Slop:          t.MM(3),
		SwipeMin:      t.MM(12),
		LongPressTime: 550 * time.Millisecond,
		SwipeMaxTime:  800 * time.Millisecond,
		schedule:      schedule,
		emit:          emit,
	}
}

// Feed передаёт касание.
func (r *Recognizer) Feed(ev device.TouchEvent) {
	switch ev.Phase {
	case device.TouchDown:
		if r.active {
			return
		}
		r.active, r.slot = true, ev.Slot
		r.start, r.last, r.startTime = ev.Point(), ev.Point(), ev.Time
		r.panning, r.longFired = false, false
		if r.schedule != nil {
			r.cancelLP = r.schedule(r.LongPressTime, r.fireLongPress)
		}
	case device.TouchMove:
		if !r.active || ev.Slot != r.slot {
			return
		}
		r.last = ev.Point()
		if !r.panning && !r.longFired && dist2(r.start, r.last) > r.Slop*r.Slop {
			r.panning = true
			r.stopLongPress()
			r.emit(Gesture{Kind: PanStart, Pos: r.last, Start: r.start, Time: ev.Time})
		}
		if r.panning {
			r.emit(Gesture{Kind: PanMove, Pos: r.last, Start: r.start, Time: ev.Time})
		}
	case device.TouchUp:
		if !r.active || ev.Slot != r.slot {
			return
		}
		r.active = false
		r.stopLongPress()
		if ev.X != 0 || ev.Y != 0 {
			r.last = ev.Point()
		}
		switch {
		case r.panning:
			r.emit(Gesture{Kind: PanEnd, Pos: r.last, Start: r.start, Time: ev.Time})
			d := r.last.Sub(r.start)
			if ev.Time.Sub(r.startTime) <= r.SwipeMaxTime && dist2(r.start, r.last) >= r.SwipeMin*r.SwipeMin {
				r.emit(Gesture{Kind: Swipe, Pos: r.last, Start: r.start, Dir: direction(d), Time: ev.Time})
			}
		case !r.longFired:
			r.emit(Gesture{Kind: Tap, Pos: r.start, Start: r.start, Time: ev.Time})
		}
	}
}

func (r *Recognizer) fireLongPress() {
	r.cancelLP = nil
	if r.active && !r.panning {
		r.longFired = true
		r.emit(Gesture{Kind: LongPress, Pos: r.start, Start: r.start, Time: time.Now()})
	}
}

func (r *Recognizer) stopLongPress() {
	if r.cancelLP != nil {
		r.cancelLP()
		r.cancelLP = nil
	}
}

// Reset забывает текущее касание: например, после сна.
func (r *Recognizer) Reset() {
	r.stopLongPress()
	r.active = false
}

func dist2(a, b image.Point) int {
	d := a.Sub(b)
	return d.X*d.X + d.Y*d.Y
}

func direction(d image.Point) Direction {
	if abs(d.X) >= abs(d.Y) {
		if d.X < 0 {
			return DirLeft
		}
		return DirRight
	}
	if d.Y < 0 {
		return DirUp
	}
	return DirDown
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
