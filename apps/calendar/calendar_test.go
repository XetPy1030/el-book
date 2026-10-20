package calendar

import (
	"testing"
	"time"
)

func TestParseEventAndHolidays(t *testing.T) {
	for in, want := range map[string][2]string{
		"14:30 Встреча":  {"14:30", "Встреча"},
		"9.05 Зарядка":   {"09:05", "Зарядка"},
		"Позвонить маме": {"", "Позвонить маме"},
		"25:00 Не время": {"", "25:00 Не время"},
	} {
		at, rest := parseEvent(in)
		if at != want[0] || rest != want[1] {
			t.Errorf("%q → %q, %q", in, at, rest)
		}
	}
	if holiday(time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)) != "День народного единства" {
		t.Error("4 ноября — не праздник")
	}
	if holiday(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)) != "" {
		t.Error("19 октября — праздник")
	}
}

func TestGridStartsOnMonday(t *testing.T) {
	a := &App{month: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)} // 1 октября 2026 — четверг
	g := &monthGrid{a: a}
	if f := g.first(); f.Weekday() != time.Monday || f.Day() != 28 {
		t.Errorf("сетка начинается с %v", f)
	}
}
