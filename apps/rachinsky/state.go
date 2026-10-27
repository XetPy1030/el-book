package rachinsky

import (
	"encoding/json"
	"math/rand"
	"time"

	"el-book/sdk/storage"
)

// Статусы задачи.
const (
	stUnseen byte = 0 // ещё не отвечена
	stSolved byte = 1 // последний ответ — верный
	stWrong  byte = 2 // последний ответ — неверный
)

const progressFile = "progress.json"

// state — прогресс и статистика тренажёра. Держится в памяти, сохраняется в progress.json
// после каждого ответа и при сворачивании приложения.
type state struct {
	n      int
	status []byte
	flags  []bool

	cursor   int // позиция режима «по порядку»: с неё продолжаем
	typed    bool
	attempts int
	correct  int

	curStreak, bestStreak int // ответов верно подряд
	curDays, bestDays     int // дней подряд с занятиями
	lastDay               string
	day                   map[string]dayStat // "2006-01-02" → попытки и верные за день
}

type dayStat struct{ Attempts, Solved int }

// persisted — то, что уходит в JSON.
type persisted struct {
	Status     string            `json:"status"`
	Flags      string            `json:"flags"`
	Cursor     int               `json:"cursor"`
	Typed      bool              `json:"typed,omitempty"`
	Attempts   int               `json:"attempts"`
	Correct    int               `json:"correct"`
	CurStreak  int               `json:"cur_streak"`
	BestStreak int               `json:"best_streak"`
	CurDays    int               `json:"cur_days"`
	BestDays   int               `json:"best_days"`
	LastDay    string            `json:"last_day,omitempty"`
	Day        map[string][2]int `json:"day,omitempty"`
}

func newState(n int) *state {
	return &state{n: n, status: make([]byte, n), flags: make([]bool, n), day: map[string]dayStat{}}
}

func loadState(st *storage.App, n int) *state {
	s := newState(n)
	data, err := st.ReadFile(progressFile)
	if err != nil {
		return s
	}
	var p persisted
	if json.Unmarshal(data, &p) != nil {
		return s
	}
	for i := 0; i < n && i < len(p.Status); i++ {
		switch p.Status[i] {
		case '1':
			s.status[i] = stSolved
		case '2':
			s.status[i] = stWrong
		}
	}
	for i := 0; i < n && i < len(p.Flags); i++ {
		if p.Flags[i] == '1' {
			s.flags[i] = true
		}
	}
	s.cursor = clamp(p.Cursor, 0, n)
	s.typed = p.Typed
	s.attempts, s.correct = p.Attempts, p.Correct
	s.curStreak, s.bestStreak = p.CurStreak, p.BestStreak
	s.curDays, s.bestDays = p.CurDays, p.BestDays
	s.lastDay = p.LastDay
	for d, v := range p.Day {
		s.day[d] = dayStat{Attempts: v[0], Solved: v[1]}
	}
	return s
}

func (s *state) save(st *storage.App) error {
	status := make([]byte, s.n)
	flags := make([]byte, s.n)
	for i := 0; i < s.n; i++ {
		status[i] = '0' + s.status[i]
		flags[i] = '0'
		if s.flags[i] {
			flags[i] = '1'
		}
	}
	p := persisted{
		Status: string(status), Flags: string(flags), Cursor: s.cursor, Typed: s.typed,
		Attempts: s.attempts, Correct: s.correct,
		CurStreak: s.curStreak, BestStreak: s.bestStreak,
		CurDays: s.curDays, BestDays: s.bestDays, LastDay: s.lastDay,
		Day: map[string][2]int{},
	}
	for d, v := range s.day {
		p.Day[d] = [2]int{v.Attempts, v.Solved}
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return st.WriteFile(progressFile, data)
}

// record отмечает ответ на задачу idx и обновляет серии и дневную статистику.
func (s *state) record(idx int, correct bool, now time.Time) {
	s.attempts++
	if correct {
		s.correct++
		s.status[idx] = stSolved
		s.curStreak++
		if s.curStreak > s.bestStreak {
			s.bestStreak = s.curStreak
		}
	} else {
		s.status[idx] = stWrong
		s.curStreak = 0
	}
	s.advanceCursor(idx)
	s.bumpDay(now, correct)
}

// skip отмечает, что задачу idx пропустили: статистику не трогает, но двигает закладку.
func (s *state) skip(idx int) { s.advanceCursor(idx) }

func (s *state) advanceCursor(idx int) {
	if idx+1 > s.cursor {
		s.cursor = idx + 1
	}
}

func (s *state) bumpDay(now time.Time, correct bool) {
	d := now.Format("2006-01-02")
	if s.lastDay == "" {
		s.curDays = 1
	} else if d != s.lastDay {
		if dayDiff(s.lastDay, d) == 1 {
			s.curDays++
		} else {
			s.curDays = 1
		}
	}
	if s.curDays > s.bestDays {
		s.bestDays = s.curDays
	}
	s.lastDay = d
	rec := s.day[d]
	rec.Attempts++
	if correct {
		rec.Solved++
	}
	s.day[d] = rec
}

func (s *state) toggleFlag(idx int) { s.flags[idx] = !s.flags[idx] }

// count считает задачи с нужным статусом.
func (s *state) count(st byte) int {
	n := 0
	for _, v := range s.status {
		if v == st {
			n++
		}
	}
	return n
}

func (s *state) solved() int { return s.count(stSolved) }
func (s *state) wrong() int  { return s.count(stWrong) }
func (s *state) flagged() int {
	n := 0
	for _, f := range s.flags {
		if f {
			n++
		}
	}
	return n
}

// nextInOrder — первая задача от позиции from, которую ещё не решили; -1, если решены все.
func (s *state) nextInOrder(from int) int {
	for i := from; i < s.n; i++ {
		if s.status[i] != stSolved {
			return i
		}
	}
	for i := 0; i < from && i < s.n; i++ {
		if s.status[i] != stSolved {
			return i
		}
	}
	return -1
}

// randomUnsolved — случайная ещё не решённая задача; -1, если решены все.
func (s *state) randomUnsolved(now time.Time) int {
	var pool []int
	for i := 0; i < s.n; i++ {
		if s.status[i] != stSolved {
			pool = append(pool, i)
		}
	}
	if len(pool) == 0 {
		return -1
	}
	r := rand.New(rand.NewSource(now.UnixNano()))
	return pool[r.Intn(len(pool))]
}

// review — отложенные и ошибочные задачи для режима повторения, по порядку.
func (s *state) review() []int {
	var out []int
	for i := 0; i < s.n; i++ {
		if s.flags[i] || s.status[i] == stWrong {
			out = append(out, i)
		}
	}
	return out
}

// accuracy — доля верных ответов в процентах; -1, если попыток не было.
func (s *state) accuracy() int {
	if s.attempts == 0 {
		return -1
	}
	return (s.correct*100 + s.attempts/2) / s.attempts
}

func dayDiff(a, b string) int {
	ta, err1 := time.Parse("2006-01-02", a)
	tb, err2 := time.Parse("2006-01-02", b)
	if err1 != nil || err2 != nil {
		return 1 << 30
	}
	return int(tb.Sub(ta).Hours()/24 + 0.5)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
