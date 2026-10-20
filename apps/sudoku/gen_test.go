package sudoku

import "testing"

func TestGenerateUnique(t *testing.T) {
	for _, level := range []Level{Easy, Medium, Hard} {
		for seed := uint64(1); seed <= 3; seed++ {
			p, full := Generate(level, seed)
			clues := 0
			for i := range p {
				if p[i] != 0 {
					clues++
					if p[i] != full[i] {
						t.Fatalf("подсказка не совпадает с решением")
					}
				}
			}
			if clues > level.clues()+6 {
				t.Errorf("%s, зерно %d: %d подсказок — легче заявленного", level.title(), seed, clues)
			}
			sol, ok := p.Solution()
			if !ok || sol != full {
				t.Fatalf("%s, зерно %d: решение не единственное", level.title(), seed)
			}
			if !valid(full) {
				t.Fatalf("решение нарушает правила")
			}
		}
	}
}

func valid(g Grid) bool {
	for i := 0; i < 81; i++ {
		v := g[i]
		g[i] = 0
		ok := v != 0 && g.candidates(i)&(1<<v) != 0
		g[i] = v
		if !ok {
			return false
		}
	}
	return true
}

func BenchmarkGenerateHard(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Generate(Hard, uint64(i)+1)
	}
}
