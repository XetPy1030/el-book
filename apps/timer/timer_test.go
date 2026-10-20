package timer

import (
	"testing"
	"time"
)

func TestClock(t *testing.T) {
	cases := map[int64]string{0: "00:00", 59_999: "00:59", 61_000: "01:01", 3_723_000: "1:02:03", -5: "00:00"}
	for in, want := range cases {
		if got := clock(in, false); got != want {
			t.Errorf("clock(%d) = %q, ждали %q", in, got, want)
		}
	}
	if got := clock(61_300, true); got != "01:01,3" {
		t.Errorf("с десятыми: %q", got)
	}
}

func TestStateMath(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	s := state{Duration: 60_000, EndsAt: 1_030_000}
	if got := s.timerLeft(now); got != 30_000 {
		t.Errorf("осталось %d", got)
	}
	s = state{SwStart: 990_000, SwAcc: 5_000}
	if got := s.swElapsed(now); got != 15_000 {
		t.Errorf("секундомер %d", got)
	}
}

func TestDigitsChangeOnTick(t *testing.T) {
	start := time.UnixMilli(1_000_000)
	timer := state{Duration: 300_000, EndsAt: 1_300_000}
	if got := timer.digits(start); got != "05:00" {
		t.Errorf("таймер на старте показывает %q, ждали 05:00", got)
	}
	if got := timer.digits(time.UnixMilli(1_299_999)); got != "00:01" {
		t.Errorf("за миллисекунду до сигнала — %q, ждали 00:01", got)
	}
	watch := state{Stopwatch: true, SwStart: 1_000_000, SwAcc: 1_300}
	if got := watch.digits(start); got != "00:01" {
		t.Errorf("секундомер на ходу — %q, ждали 00:01 без десятых", got)
	}
	// Тик приходит ровно к смене цифр: до него они те же, на нём — другие.
	for _, s := range []state{timer, watch} {
		for _, off := range []int64{0, 1, 399, 999, 1_000, 61_234} {
			now := start.Add(time.Duration(off) * time.Millisecond)
			next := now.Add(s.untilChange(now))
			if s.digits(next.Add(-time.Millisecond)) != s.digits(now) || s.digits(next) == s.digits(now) {
				t.Errorf("секундомер=%v, через %d мс: тик в %v не совпал со сменой цифр %q", s.Stopwatch, off, next.Sub(now), s.digits(now))
			}
		}
	}
}
