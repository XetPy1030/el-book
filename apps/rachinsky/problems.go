// Package rachinsky — тренажёр устного счёта по задачнику С. А. Рачинского «1001 задача
// для умственного счёта». Задача за задачей: прочитать, посчитать в уме, сверить ответ;
// подсказки по старым мерам, прогресс и статистика сохраняются между запусками.
package rachinsky

import (
	_ "embed"
	"strconv"
	"strings"
)

// problemsTSV — задачник: по строке на задачу, поля «номер\tответ\tтекст». Получен из
// текста задачника (см. local-artifacts) разбором: ответ всегда в конце в скобках.
//
//go:embed problems.tsv
var problemsTSV string

// Problem — одна задача: книжный номер, текст и ответ. Номер — как в задачнике; он не
// совпадает с позицией в списке (в источнике есть пропуски и повторы номеров), поэтому
// для хранения прогресса используется позиция, а номер только показывается.
type Problem struct {
	Num    int
	Answer string
	Text   string
}

// problems — все задачи в порядке задачника.
var problems = parseProblems(problemsTSV)

func parseProblems(s string) []Problem {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	out := make([]Problem, 0, len(lines))
	for _, ln := range lines {
		if ln == "" {
			continue
		}
		f := strings.SplitN(ln, "\t", 3)
		if len(f) != 3 {
			continue
		}
		num, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		out = append(out, Problem{Num: num, Answer: f[1], Text: f[2]})
	}
	return out
}
