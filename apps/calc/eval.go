package calc

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// Ошибки вычисления, понятные пользователю.
var (
	errSyntax  = errors.New("Ошибка в выражении")
	errDivZero = errors.New("Деление на ноль")
	errRange   = errors.New("Слишком большое число")
)

// eval вычисляет выражение: + − × ÷, скобки, унарный минус, проценты, десятичная запятая.
// «200 + 10%» — 220, «50 × 10%» — 5: процент от левого операнда, как в обычных калькуляторах.
func eval(expr string) (float64, error) {
	p := parser{src: []rune(expr)}
	v, err := p.expr()
	if err != nil {
		return 0, err
	}
	p.skip()
	if p.pos < len(p.src) {
		return 0, errSyntax
	}
	if math.IsInf(v, 0) || math.IsNaN(v) || math.Abs(v) > 1e15 {
		return 0, errRange
	}
	return v, nil
}

type parser struct {
	src []rune
	pos int
}

func (p *parser) skip() {
	for p.pos < len(p.src) && unicode.IsSpace(p.src[p.pos]) {
		p.pos++
	}
}

func (p *parser) peek() rune {
	p.skip()
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *parser) expr() (float64, error) {
	v, err := p.term()
	if err != nil {
		return 0, err
	}
	for {
		switch op := p.peek(); op {
		case '+', '-', '−':
			p.pos++
			rhs, pct, err := p.termPct()
			if err != nil {
				return 0, err
			}
			if pct {
				rhs = v * rhs / 100
			}
			if op == '+' {
				v += rhs
			} else {
				v -= rhs
			}
		default:
			return v, nil
		}
	}
}

// termPct — слагаемое; pct — это число с процентом вида «10%», которое считается от левой части.
func (p *parser) termPct() (float64, bool, error) {
	start := p.pos
	v, err := p.factor()
	if err != nil {
		return 0, false, err
	}
	if p.peek() == '%' {
		p.pos++
		if next := p.peek(); next == 0 || next == ')' || next == '+' || next == '-' || next == '−' {
			return v, true, nil
		}
		v /= 100
	}
	p.pos = start
	v, err = p.term()
	return v, false, err
}

func (p *parser) term() (float64, error) {
	v, err := p.factor()
	if err != nil {
		return 0, err
	}
	for {
		switch op := p.peek(); op {
		case '%':
			p.pos++
			v /= 100
		case '*', '×', '/', '÷':
			p.pos++
			rhs, err := p.factor()
			if err != nil {
				return 0, err
			}
			if p.peek() == '%' {
				p.pos++
				rhs /= 100
			}
			if op == '*' || op == '×' {
				v *= rhs
			} else {
				if rhs == 0 {
					return 0, errDivZero
				}
				v /= rhs
			}
		default:
			return v, nil
		}
	}
}

func (p *parser) factor() (float64, error) {
	switch c := p.peek(); {
	case c == '-' || c == '−':
		p.pos++
		v, err := p.factor()
		return -v, err
	case c == '+':
		p.pos++
		return p.factor()
	case c == '(':
		p.pos++
		v, err := p.expr()
		if err != nil {
			return 0, err
		}
		if p.peek() == ')' {
			p.pos++
		}
		// Незакрытая скобка в конце — как будто закрыта: «2 × (3 + 4» — 14.
		return v, nil
	case c == ',' || c == '.' || unicode.IsDigit(c):
		start := p.pos
		for p.pos < len(p.src) && (unicode.IsDigit(p.src[p.pos]) || p.src[p.pos] == ',' || p.src[p.pos] == '.') {
			p.pos++
		}
		num := strings.ReplaceAll(string(p.src[start:p.pos]), ",", ".")
		v, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, errSyntax
		}
		return v, nil
	}
	return 0, errSyntax
}

// format — число для экрана: до 12 значащих цифр, запятая, пробелы между тысячами, «−».
func format(v float64) string {
	if v == 0 {
		return "0"
	}
	s := strconv.FormatFloat(v, 'g', 12, 64)
	if strings.ContainsAny(s, "e") {
		s = strconv.FormatFloat(v, 'e', 6, 64)
		return strings.NewReplacer(".", ",", "-", "−", "e+", "·10^", "e", "·10^").Replace(s)
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteRune('\u202f') // узкий неразрывный пробел между тысячами
		}
		b.WriteRune(r)
	}
	out := b.String()
	if frac != "" {
		out += "," + frac
	}
	if neg {
		out = "−" + out
	}
	return out
}
