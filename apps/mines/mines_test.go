package mines

import (
	"testing"

	"el-book/sdk/kit"
)

func TestFirstMoveSafeAndFlood(t *testing.T) {
	for seed := uint64(1); seed < 30; seed++ {
		p := &kit.Prefs[game]{Value: game{Size: 1, Exploded: -1}}
		a := &App{g: p}
		g := &p.Value
		s := g.sz()
		g.State = make([]uint8, s.Cols*s.Rows)
		first := 5*s.Cols + 5
		a.plant(first, seed)
		mines := 0
		for _, m := range g.Mines {
			if m {
				mines++
			}
		}
		if mines != s.Mines {
			t.Fatalf("мин %d, ждали %d", mines, s.Mines)
		}
		if g.Mines[first] || g.around(first) != 0 {
			t.Fatalf("зерно %d: первый ход не безопасен", seed)
		}
		if !g.open(first) {
			t.Fatal("подорвались на первом ходе")
		}
		open := 0
		for _, st := range g.State {
			if st == opened {
				open++
			}
		}
		if open < 9 {
			t.Errorf("зерно %d: волна открыла %d клеток", seed, open)
		}
	}
}
