package rachinsky

import (
	"strings"
	"testing"
	"time"
)

func TestProblemsParsed(t *testing.T) {
	if len(problems) < 990 {
		t.Fatalf("задач разобрано %d, ожидалось ~999", len(problems))
	}
	for i, p := range problems {
		if p.Num <= 0 {
			t.Errorf("задача %d: номер %d", i, p.Num)
		}
		if strings.TrimSpace(p.Text) == "" {
			t.Errorf("задача №%d: пустой текст", p.Num)
		}
		if strings.TrimSpace(p.Answer) == "" {
			t.Errorf("задача №%d: пустой ответ", p.Num)
		}
	}
	if problems[0].Num != 1 || !strings.HasPrefix(problems[0].Text, "В течение февраля") {
		t.Errorf("первая задача не та: %+v", problems[0])
	}
	if problems[0].Answer != "1008" {
		t.Errorf("ответ первой задачи %q, ожидался 1008", problems[0].Answer)
	}
}

func TestUnitsIn(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"Я купил 11 десятин земли по 23 руб.", []string{"Десятина"}},
		{"Сколько вершков в 375 аршинах?", []string{"Аршин", "Вершок"}},
		{"7 четвертей 7 четвериков 7 гарнцев. — Сколько гарнцев?", []string{"Четверть", "Четверик (мера)", "Гарнец"}},
		{"Из 1 стопы 1 дести (24 листа) бумаги сделаны тетради", []string{"Стопа", "Десть"}},
		{"Один продавал меру по 1 руб.", []string{"Четверик (мера)"}},
		{"Некто тратит 40 коп. в день. Сколько он тратит в год?", nil}, // деньги не считаются
	}
	for _, c := range cases {
		got := names(unitsIn(c.text))
		if !equal(got, c.want) {
			t.Errorf("unitsIn(%q) = %v, ожидалось %v", c.text, got, c.want)
		}
	}
}

// TestUnitDetectionNoFalseVerb — «мерил дорогу» не должно считаться мерой.
func TestUnitDetectionNoFalseVerb(t *testing.T) {
	got := names(unitsIn("Крестьянин мерил дорогу шестом"))
	if len(got) != 0 {
		t.Errorf("ложная мера в «мерил»: %v", got)
	}
}

func TestCheckTyped(t *testing.T) {
	cases := []struct {
		stored, typed        string
		wantOK, wantDecisive bool
	}{
		{"1008", "1008", true, true},
		{"1008", "1007", false, true},
		{"50 аршин", "50", true, true},
		{"146 руб.", "146", true, true},
		{"По 28 верст", "28", true, true},
		{"6 руб. 25 коп.", "625", false, false}, // рубли с копейками — самопроверка
		{"7; 10 1/2", "7", false, false},        // перечисление — самопроверка
		{"1440; 10080", "1440", false, false},   // два ответа — самопроверка
		{"1008", "", false, true},               // пусто — не совпало, но решение однозначно
	}
	for _, c := range cases {
		ok, dec := checkTyped(c.stored, c.typed)
		if ok != c.wantOK || dec != c.wantDecisive {
			t.Errorf("checkTyped(%q,%q) = (%v,%v), ожидалось (%v,%v)", c.stored, c.typed, ok, dec, c.wantOK, c.wantDecisive)
		}
	}
}

func TestStreaks(t *testing.T) {
	s := newState(5)
	day := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	s.record(0, true, day)
	s.record(1, true, day)
	s.record(2, false, day)
	if s.curStreak != 0 || s.bestStreak != 2 {
		t.Errorf("серия: cur=%d best=%d, ожидалось 0 и 2", s.curStreak, s.bestStreak)
	}
	s.record(3, true, day)
	if s.curStreak != 1 {
		t.Errorf("серия после верного: %d, ожидалось 1", s.curStreak)
	}
	if s.solved() != 3 || s.wrong() != 1 {
		t.Errorf("статусы: solved=%d wrong=%d", s.solved(), s.wrong())
	}
	if s.accuracy() != 75 {
		t.Errorf("точность %d%%, ожидалось 75", s.accuracy())
	}
}

func TestResolveWrongToSolved(t *testing.T) {
	s := newState(3)
	day := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	s.record(0, false, day)
	if s.status[0] != stWrong || s.wrong() != 1 {
		t.Fatalf("после ошибки статус %d", s.status[0])
	}
	s.record(0, true, day)
	if s.status[0] != stSolved || s.wrong() != 0 || s.solved() != 1 {
		t.Errorf("после исправления: solved=%d wrong=%d", s.solved(), s.wrong())
	}
}

func TestDayStreak(t *testing.T) {
	s := newState(10)
	d := func(day int) time.Time { return time.Date(2026, 10, day, 10, 0, 0, 0, time.UTC) }
	s.record(0, true, d(1))
	s.record(1, true, d(2)) // следующий день
	s.record(2, true, d(2)) // тот же день — без изменений
	s.record(3, true, d(4)) // пропуск дня — серия сбрасывается
	if s.curDays != 1 {
		t.Errorf("дней подряд cur=%d, ожидалось 1", s.curDays)
	}
	if s.bestDays != 2 {
		t.Errorf("рекорд дней=%d, ожидалось 2", s.bestDays)
	}
	if s.day["2026-10-02"].Attempts != 2 {
		t.Errorf("за 2-е число попыток %d, ожидалось 2", s.day["2026-10-02"].Attempts)
	}
}

func TestNextAndReview(t *testing.T) {
	s := newState(5)
	day := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	s.record(0, true, day)
	s.record(1, true, day)
	if got := s.nextInOrder(0); got != 2 {
		t.Errorf("nextInOrder=%d, ожидалось 2", got)
	}
	s.record(3, false, day)
	s.toggleFlag(4)
	rev := s.review()
	if !equalInt(rev, []int{3, 4}) {
		t.Errorf("review=%v, ожидалось [3 4]", rev)
	}
}

func names(us []Unit) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
