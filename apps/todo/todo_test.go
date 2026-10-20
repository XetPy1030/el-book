package todo

import (
	"testing"
	"time"
)

func TestSortedAndDue(t *testing.T) {
	now := time.Date(2026, 10, 19, 14, 0, 0, 0, time.UTC)
	tasks := []Task{
		{ID: "a", Text: "без срока", Added: 1},
		{ID: "b", Text: "завтра", Due: "2026-10-20", Added: 2},
		{ID: "c", Text: "сделано", Done: true, DoneAt: 5},
		{ID: "d", Text: "просрочено", Due: "2026-10-17", Added: 3},
		{ID: "e", Text: "сегодня", Due: "2026-10-19", Added: 4},
	}
	var got []string
	for _, x := range sorted(tasks, now) {
		got = append(got, x.ID)
	}
	if want := "d e b a c"; join(got) != want {
		t.Errorf("порядок %q, ждали %q", join(got), want)
	}
	cases := map[string]string{
		"2026-10-17": "просрочено на 2 дня", "2026-10-19": "сегодня", "2026-10-20": "завтра",
		"2026-10-23": "пятница", "2026-11-05": "5 ноября",
	}
	for due, want := range cases {
		if got := dueText(due, now); got != want {
			t.Errorf("срок %s: %q, ждали %q", due, got, want)
		}
	}
}

func join(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += " "
		}
		out += x
	}
	return out
}
