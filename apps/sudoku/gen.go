package sudoku

import (
	"math/bits"
	"math/rand/v2"
)

// Grid — поле 9 × 9 построчно: 0 — пустая клетка, 1…9 — цифра.
type Grid [81]int8

// candidates — какие цифры можно поставить в клетку i: биты 1…9.
func (g *Grid) candidates(i int) uint16 {
	r, c := i/9, i%9
	used := uint16(0)
	for k := 0; k < 9; k++ {
		used |= 1 << g[r*9+k]
		used |= 1 << g[k*9+c]
	}
	br, bc := r/3*3, c/3*3
	for dr := 0; dr < 3; dr++ {
		for dc := 0; dc < 3; dc++ {
			used |= 1 << g[(br+dr)*9+bc+dc]
		}
	}
	return ^used & 0x3FE
}

// solve ищет решения перебором с выбором клетки с наименьшим числом вариантов и считает
// их до limit. rng — порядок перебора цифр (nil — по возрастанию); найденное первое
// решение пишется в first.
func (g *Grid) solve(limit int, rng *rand.Rand, first *Grid) int {
	best, bestCount := -1, 10
	var bestCand uint16
	for i := 0; i < 81; i++ {
		if g[i] != 0 {
			continue
		}
		c := g.candidates(i)
		n := bits.OnesCount16(c)
		if n == 0 {
			return 0
		}
		if n < bestCount {
			best, bestCount, bestCand = i, n, c
			if n == 1 {
				break
			}
		}
	}
	if best < 0 {
		if first != nil && *first == (Grid{}) {
			*first = *g
		}
		return 1
	}
	digits := make([]int8, 0, 9)
	for d := int8(1); d <= 9; d++ {
		if bestCand&(1<<d) != 0 {
			digits = append(digits, d)
		}
	}
	if rng != nil {
		rng.Shuffle(len(digits), func(i, j int) { digits[i], digits[j] = digits[j], digits[i] })
	}
	count := 0
	for _, d := range digits {
		g[best] = d
		count += g.solve(limit-count, rng, first)
		g[best] = 0
		if count >= limit {
			break
		}
	}
	return count
}

// Solution — единственное решение головоломки; ok = false, если решений нет или их
// несколько.
func (g Grid) Solution() (Grid, bool) {
	var first Grid
	n := g.solve(2, nil, &first)
	return first, n == 1
}

// Level — сложность.
type Level int

const (
	Easy Level = iota
	Medium
	Hard
)

func (l Level) title() string {
	switch l {
	case Medium:
		return "Средний"
	case Hard:
		return "Сложный"
	}
	return "Лёгкий"
}

// clues — сколько цифр оставить открытыми.
func (l Level) clues() int {
	switch l {
	case Medium:
		return 32
	case Hard:
		return 26
	}
	return 40
}

// Generate строит головоломку с единственным решением: случайное заполненное поле, из
// которого убираются цифры, пока решение остаётся единственным и не достигнута сложность.
func Generate(level Level, seed uint64) (puzzle, solution Grid) {
	rng := rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
	var full Grid
	var empty Grid
	empty.solve(1, rng, &full)
	puzzle = full
	order := rng.Perm(81)
	left := 81
	for _, i := range order {
		if left <= level.clues() {
			break
		}
		saved := puzzle[i]
		puzzle[i] = 0
		probe := puzzle
		if probe.solve(2, nil, nil) != 1 {
			puzzle[i] = saved
			continue
		}
		left--
	}
	return puzzle, full
}
