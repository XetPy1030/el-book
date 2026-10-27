package rachinsky

import "strings"

// checkTyped сверяет набранный пользователем ответ typed (только цифры) с книжным ответом
// stored. Возвращает decisive=false, когда ответ нельзя надёжно сверить автоматически —
// он составной: дробь, несколько величин, рубли с копейками. Тогда остаётся самопроверка.
// Иначе decisive=true и ok говорит, совпало ли число.
func checkTyped(stored, typed string) (ok, decisive bool) {
	n, single := singleInt(stored)
	if !single {
		return false, false
	}
	t := strings.TrimSpace(typed)
	if t == "" {
		return false, true
	}
	return t == itoa(n), true
}

// singleInt возвращает единственное целое число книжного ответа, если ответ простой:
// одно число, возможно с названием единицы. Составные ответы (дробь «/», перечисление «;»,
// рубли вместе с копейками) простыми не считаются.
func singleInt(stored string) (int, bool) {
	s := strings.ToLower(stored)
	if strings.ContainsAny(s, ";/") {
		return 0, false
	}
	if strings.Contains(s, "руб") && strings.Contains(s, "коп") {
		return 0, false
	}
	nums := ints(stored)
	if len(nums) != 1 {
		return 0, false
	}
	return nums[0], true
}

// ints вытаскивает целые числа из строки по порядку.
func ints(s string) []int {
	var out []int
	cur := 0
	has := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			cur = cur*10 + int(r-'0')
			has = true
			continue
		}
		if has {
			out = append(out, cur)
			cur, has = 0, false
		}
	}
	if has {
		out = append(out, cur)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
