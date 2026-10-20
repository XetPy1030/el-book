package ui

import "testing"

func TestPlural(t *testing.T) {
	cases := map[int]string{
		0: "касаний", 1: "касание", 2: "касания", 4: "касания", 5: "касаний",
		11: "касаний", 12: "касаний", 14: "касаний", 21: "касание", 22: "касания",
		101: "касание", 111: "касаний", 1000: "касаний", -1: "касание",
	}
	for n, want := range cases {
		if got := Plural(n, "касание", "касания", "касаний"); got != want {
			t.Errorf("%d: %q, нужно %q", n, got, want)
		}
	}
	if got := Count(3, "рисунок", "рисунка", "рисунков"); got != "3 рисунка" {
		t.Errorf("Count: %q", got)
	}
}
