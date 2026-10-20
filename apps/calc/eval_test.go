package calc

import (
	"errors"
	"testing"
)

func TestEval(t *testing.T) {
	cases := map[string]float64{
		"2+3×4": 14, "(2+3)×4": 20, "−5+2": -3, "10÷4": 2.5, "200+10%": 220, "200−10%": 180,
		"50×10%": 5, "10%": 0.1, "2×(3+4": 14, "3,5+1,5": 5, "−(−3)": 3, "1,5×2+0,25": 3.25,
		"(100+10%)×2": 220, "7−2−1": 4, "8÷2÷2": 2,
	}
	for expr, want := range cases {
		got, err := eval(expr)
		if err != nil || got != want {
			t.Errorf("%s = %v, %v; ждали %v", expr, got, err, want)
		}
	}
	for expr, want := range map[string]error{"1÷0": errDivZero, "2++": errSyntax, "×3": errSyntax, "": errSyntax, "9999999×9999999×99999": errRange} {
		if _, err := eval(expr); !errors.Is(err, want) {
			t.Errorf("%s: ошибка %v, ждали %v", expr, err, want)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := map[float64]string{
		1234567.5: "1\u202f234\u202f567,5", -0.25: "−0,25", 0: "0", 0.1 + 0.2: "0,3", 1e20: "1,000000·10^20", 100: "100",
	}
	for v, want := range cases {
		if got := format(v); got != want {
			t.Errorf("format(%v) = %q, ждали %q", v, got, want)
		}
	}
}
