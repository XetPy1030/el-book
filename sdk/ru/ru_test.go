package ru

import (
	"testing"
	"time"
)

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{1: "касание", 3: "касания", 5: "касаний", 11: "касаний", 21: "касание", 104: "касания"} {
		if got := Plural(n, "касание", "касания", "касаний"); got != want {
			t.Errorf("Plural(%d) = %s, нужно %s", n, got, want)
		}
	}
}

func TestDates(t *testing.T) {
	now := time.Date(2026, 10, 20, 14, 5, 0, 0, time.Local)
	cases := []struct{ got, want string }{
		{Date(now), "вторник, 20 октября"},
		{Short(now.Add(-time.Hour), now), "13:05"},
		{Short(now.AddDate(0, 0, -1), now), "вчера"},
		{Short(now.AddDate(0, -1, 0), now), "20 сен"},
		{Short(now.AddDate(-1, 0, 0), now), "20 окт 2025"},
		{Ago(now.Add(-30*time.Second), now), "только что"},
		{Ago(now.Add(-5*time.Minute), now), "5 минут назад"},
		{Ago(now.Add(-2*time.Hour), now), "2 часа назад"},
		{Ago(now.Add(-20*time.Hour), now), "вчера в 18:05"},
		{Duration(65 * time.Minute), "1 ч 05 мин"},
		{Duration(25 * time.Minute), "25 мин"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("получено %q, нужно %q", c.got, c.want)
		}
	}
}

func TestNumbers(t *testing.T) {
	cases := []struct{ got, want string }{
		{Decimal(3.40, 2), "3,4"},
		{Decimal(-0.5, 1), "−0,5"},
		{Decimal(-0.04, 1), "0"},
		{Decimal(12, 2), "12"},
		{Decimal(2.718, 2), "2,72"},
		{Size(512), "512 Б"},
		{Size(12 * 1024), "12 КБ"},
		{Size(3565158), "3,4 МБ"},
		{Size(300 << 20), "300 МБ"},
		{Size(5 << 30), "5 ГБ"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%q, ждали %q", c.got, c.want)
		}
	}
}
