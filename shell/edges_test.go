package shell

import (
	"image"
	"testing"
	"time"

	"el-book/sdk/device"
)

func newTestEdges() *edgeSwipe {
	return &edgeSwipe{zone: 40, distance: 120, maxTime: 500 * time.Millisecond}
}

var edgeScreen = image.Rect(0, 0, 1440, 1920)

func touch(ms int, slot int, phase device.TouchPhase, x, y int) device.TouchEvent {
	return device.TouchEvent{Time: time.Unix(0, 0).Add(time.Duration(ms) * time.Millisecond), Slot: slot, Phase: phase, X: x, Y: y}
}

// run подаёт касания и собирает, что дошло бы до приложения и какие жесты сработали.
func run(e *edgeSwipe, evs ...device.TouchEvent) (passed []device.TouchEvent, fired []edge) {
	for _, ev := range evs {
		consumed, replay, f := e.feed(ev, edgeScreen)
		passed = append(passed, replay...)
		if !consumed {
			passed = append(passed, ev)
		}
		if f != edgeNone {
			fired = append(fired, f)
		}
	}
	return passed, fired
}

func TestEdgeSwipeFromTopAndBottom(t *testing.T) {
	cases := []struct {
		name     string
		from, to int
		want     edge
	}{
		{"сверху вниз", 10, 200, edgeTop},
		{"снизу вверх", 1910, 1700, edgeBottom},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEdges()
			mid := (c.from + c.to) / 2
			passed, fired := run(e,
				touch(0, 0, device.TouchDown, 700, c.from),
				touch(50, 0, device.TouchMove, 705, mid),
				touch(100, 0, device.TouchMove, 710, c.to),
				touch(150, 0, device.TouchMove, 715, c.to),
				touch(200, 0, device.TouchUp, 715, c.to),
			)
			if len(passed) != 0 {
				t.Errorf("приложению дошло %d событий жеста", len(passed))
			}
			if len(fired) != 1 || fired[0] != c.want {
				t.Errorf("жесты %v, ждали %v", fired, c.want)
			}
		})
	}
}

func TestEdgeTapReachesApp(t *testing.T) {
	e := newTestEdges()
	down := touch(0, 0, device.TouchDown, 300, 1900)
	up := touch(80, 0, device.TouchUp, 300, 1900)
	passed, fired := run(e, down, up)
	if len(fired) != 0 {
		t.Fatalf("тап у края стал жестом %v", fired)
	}
	if len(passed) != 2 || passed[0] != down || passed[1] != up {
		t.Errorf("приложение получило %v, ждали нажатие и отпускание", passed)
	}
}

func TestEdgeSlowOrSidewaysIsNotGesture(t *testing.T) {
	cases := []struct {
		name string
		evs  []device.TouchEvent
	}{
		{"медленно", []device.TouchEvent{
			touch(0, 0, device.TouchDown, 700, 1900),
			touch(400, 0, device.TouchMove, 700, 1880),
			touch(900, 0, device.TouchMove, 700, 1700),
			touch(950, 0, device.TouchUp, 700, 1700),
		}},
		{"вбок вдоль края", []device.TouchEvent{
			touch(0, 0, device.TouchDown, 200, 1900),
			touch(50, 0, device.TouchMove, 400, 1890),
			touch(100, 0, device.TouchMove, 600, 1880),
			touch(150, 0, device.TouchUp, 600, 1880),
		}},
		{"второй палец", []device.TouchEvent{
			touch(0, 0, device.TouchDown, 700, 10),
			touch(30, 1, device.TouchDown, 900, 600),
			touch(60, 0, device.TouchMove, 700, 300),
			touch(90, 0, device.TouchUp, 700, 300),
			touch(90, 1, device.TouchUp, 900, 600),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			passed, fired := run(newTestEdges(), c.evs...)
			if len(fired) != 0 {
				t.Errorf("сработал жест %v", fired)
			}
			if len(passed) != len(c.evs) {
				t.Fatalf("приложение получило %d событий из %d", len(passed), len(c.evs))
			}
			for i := range passed {
				if passed[i] != c.evs[i] {
					t.Errorf("событие %d: %v, ждали %v — порядок нарушен", i, passed[i], c.evs[i])
				}
			}
		})
	}
}

func TestEdgeMiddleOfScreenUntouched(t *testing.T) {
	e := newTestEdges()
	consumed, replay, fired := e.feed(touch(0, 0, device.TouchDown, 700, 900), edgeScreen)
	if consumed || replay != nil || fired != edgeNone || e.holding() {
		t.Error("касание посреди экрана придержано")
	}
}

func TestEdgeExpireReleasesStillFinger(t *testing.T) {
	e := newTestEdges()
	down := touch(0, 0, device.TouchDown, 700, 1900)
	if consumed, _, _ := e.feed(down, edgeScreen); !consumed {
		t.Fatal("касание у края не придержано")
	}
	got := e.expire()
	if len(got) != 1 || got[0] != down || e.holding() {
		t.Errorf("expire вернул %v", got)
	}
	// Дальше касание идёт обычным путём.
	if consumed, _, _ := e.feed(touch(900, 0, device.TouchUp, 700, 1900), edgeScreen); consumed {
		t.Error("отпускание после expire придержано")
	}
}
