package tiles

import (
	"testing"

	"el-book/sdk/ui"
)

func TestSlide(t *testing.T) {
	cases := []struct {
		in, out [4]int
		score   int
	}{
		{[4]int{2, 2, 2, 2}, [4]int{4, 4, 0, 0}, 8},
		{[4]int{2, 0, 2, 4}, [4]int{4, 4, 0, 0}, 4},
		{[4]int{4, 4, 8, 0}, [4]int{8, 8, 0, 0}, 8}, // сложенное в этом ходу не складывается снова
		{[4]int{0, 0, 0, 2}, [4]int{2, 0, 0, 0}, 0},
	}
	for _, c := range cases {
		out, score := slide(c.in)
		if out != c.out || score != c.score {
			t.Errorf("%v → %v (%d), ждали %v (%d)", c.in, out, score, c.out, c.score)
		}
	}
}

func TestMoveAndStuck(t *testing.T) {
	cells := [16]int{2, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	out, score, ok := move(cells, ui.DirUp)
	if !ok || out[0] != 4 || out[4] != 0 || score != 4 {
		t.Errorf("вверх: %v %d %v", out, score, ok)
	}
	if _, _, ok := move(cells, ui.DirLeft); ok {
		t.Error("влево ничего не сдвигается, а ход засчитан")
	}
	full := [16]int{2, 4, 2, 4, 4, 2, 4, 2, 2, 4, 2, 4, 4, 2, 4, 2}
	if !stuck(full) {
		t.Error("ходов нет, а stuck = false")
	}
}
