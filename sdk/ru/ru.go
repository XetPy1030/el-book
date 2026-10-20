// Package ru — русский язык интерфейса: склонение по числу, названия месяцев и дней недели,
// даты и «5 минут назад», дробные числа с запятой и размеры файлов. Все приложения пишут
// даты и числа одинаково.
package ru

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Plural выбирает форму слова для числа n: Plural(1, "касание", "касания", "касаний") —
// «касание», для 3 — «касания», для 11 — «касаний».
func Plural(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	switch d, dd := n%10, n%100; {
	case dd >= 11 && dd <= 14:
		return many
	case d == 1:
		return one
	case d >= 2 && d <= 4:
		return few
	}
	return many
}

// Count — число со словом в нужной форме: Count(5, "касание", "касания", "касаний") — «5 касаний».
func Count(n int, one, few, many string) string {
	return fmt.Sprintf("%d %s", n, Plural(n, one, few, many))
}

var (
	weekdays      = [...]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"}
	weekdaysShort = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
	monthsGen     = [...]string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	monthsNom     = [...]string{"январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"}
	monthsShort   = [...]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}
)

// Weekday — «понедельник».
func Weekday(d time.Weekday) string { return weekdays[d] }

// WeekdayShort — «пн».
func WeekdayShort(d time.Weekday) string { return weekdaysShort[d] }

// Month — «октябрь»: в заголовке календаря.
func Month(m time.Month) string { return monthsNom[m-1] }

// Date — «понедельник, 19 октября».
func Date(t time.Time) string {
	return fmt.Sprintf("%s, %d %s", weekdays[t.Weekday()], t.Day(), monthsGen[t.Month()-1])
}

// DayMonth — «19 октября»; другой год дописывается: «19 октября 2025».
func DayMonth(t, now time.Time) string {
	if t.Year() != now.Year() {
		return fmt.Sprintf("%d %s %d", t.Day(), monthsGen[t.Month()-1], t.Year())
	}
	return fmt.Sprintf("%d %s", t.Day(), monthsGen[t.Month()-1])
}

// Short — короткая метка времени для списков: «14:05» сегодня, «вчера», «19 окт»,
// «19 окт 2025» в другом году.
func Short(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return t.Format("15:04")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "вчера"
	case y1 == y2:
		return fmt.Sprintf("%d %s", d1, monthsShort[m1-1])
	}
	return fmt.Sprintf("%d %s %d", d1, monthsShort[m1-1], y1)
}

// ShortTime — «19 окт, 14:05».
func ShortTime(t time.Time) string {
	return fmt.Sprintf("%d %s, %s", t.Day(), monthsShort[t.Month()-1], t.Format("15:04"))
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// Ago — «только что», «5 минут назад», «2 часа назад», «вчера», дальше — дата.
func Ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "только что"
	case d < time.Hour:
		m := int(d / time.Minute)
		return Count(m, "минуту", "минуты", "минут") + " назад"
	case d < 12*time.Hour || sameDay(t, now):
		h := int(d / time.Hour)
		return Count(h, "час", "часа", "часов") + " назад"
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "вчера в " + t.Format("15:04")
	}
	return DayMonth(t, now)
}

// Duration — «1 ч 05 мин», «25 мин», «45 с».
func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d / time.Hour)
	m := int(d % time.Hour / time.Minute)
	s := int(d % time.Minute / time.Second)
	switch {
	case h > 0:
		return fmt.Sprintf("%d ч %02d мин", h, m)
	case m > 0:
		return fmt.Sprintf("%d мин", m)
	}
	return fmt.Sprintf("%d с", s)
}

// Decimal — число с digits знаками после запятой, без лишних нулей: Decimal(3.40, 2) —
// «3,4», Decimal(-0.5, 1) — «−0,5». Минус — типографский.
func Decimal(v float64, digits int) string {
	p := math.Pow(10, float64(digits))
	v = math.Round(v*p) / p
	if v == 0 {
		v = 0 // без «−0»
	}
	s := strconv.FormatFloat(v, 'f', digits, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	s = strings.Replace(s, ".", ",", 1)
	if rest, ok := strings.CutPrefix(s, "-"); ok {
		s = "−" + rest
	}
	return s
}

// Size — размер для людей: «512 Б», «12 КБ», «3,4 МБ», «1,2 ГБ».
func Size(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d Б", n)
	case n < k*k:
		return fmt.Sprintf("%d КБ", (n+k/2)/k)
	case n < k*k*k:
		return sizeUnit(float64(n)/(k*k), "МБ")
	}
	return sizeUnit(float64(n)/(k*k*k), "ГБ")
}

func sizeUnit(v float64, unit string) string {
	if v >= 100 {
		return fmt.Sprintf("%.0f %s", v, unit)
	}
	return Decimal(v, 1) + " " + unit
}
