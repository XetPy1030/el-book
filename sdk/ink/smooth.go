package ink

import (
	"math"
	"time"
)

// Smoother готовит точки штриха из касаний пальца: гасит дрожание, прореживает слишком
// близкие точки и для пера считает толщину по скорости. Один Smoother — один штрих.
type Smoother struct {
	// Alpha — доля нового положения пальца в сглаженном, 0…1: меньше — плавнее, но линия
	// отстаёт от пальца. По умолчанию 0,6.
	Alpha float64
	// MinStep — точки ближе этого к предыдущей пропускаются, пиксели. По умолчанию 2,5.
	MinStep float64
	// Speed — толщина по скорости: P у точек от 0,35 (быстро) до 1,3 (медленно).
	Speed bool
	// SlowSpeed и FastSpeed — скорости в пикселях за миллисекунду, при которых толщина
	// наибольшая и наименьшая. По умолчанию 0,2 и 3.
	SlowSpeed, FastSpeed float64

	x, y    float64 // сглаженное положение
	lx, ly  float64 // последняя выданная точка
	lt      time.Time
	p       float64 // текущая толщина
	started bool
}

func (s *Smoother) defaults() {
	if s.Alpha <= 0 || s.Alpha > 1 {
		s.Alpha = 0.6
	}
	if s.MinStep <= 0 {
		s.MinStep = 2.5
	}
	if s.SlowSpeed <= 0 {
		s.SlowSpeed = 0.2
	}
	if s.FastSpeed <= s.SlowSpeed {
		s.FastSpeed = 3
	}
}

// Begin — палец коснулся: первая точка штриха.
func (s *Smoother) Begin(x, y float64, t time.Time) Point {
	s.defaults()
	s.x, s.y, s.lx, s.ly, s.lt = x, y, x, y, t
	s.p = 1
	s.started = true
	return s.point(x, y)
}

// Move — палец сдвинулся. ok = false — точка слишком близко к прежней, её нет.
func (s *Smoother) Move(x, y float64, t time.Time) (Point, bool) {
	if !s.started {
		return s.Begin(x, y, t), true
	}
	s.x += s.Alpha * (x - s.x)
	s.y += s.Alpha * (y - s.y)
	return s.emit(s.x, s.y, t)
}

// End — палец поднялся в точке (x, y): штрих дотягивается до неё без сглаживания.
func (s *Smoother) End(x, y float64, t time.Time) (Point, bool) {
	if !s.started {
		return Point{}, false
	}
	s.started = false
	return s.emit(x, y, t)
}

func (s *Smoother) emit(x, y float64, t time.Time) (Point, bool) {
	d := math.Hypot(x-s.lx, y-s.ly)
	if d < s.MinStep {
		return Point{}, false
	}
	if s.Speed {
		dt := float64(t.Sub(s.lt)) / float64(time.Millisecond)
		if dt <= 0 {
			dt = 8 // время не пришло: считаем как один кадр касаний
		}
		v := d / dt
		k := (v - s.SlowSpeed) / (s.FastSpeed - s.SlowSpeed)
		target := 1.3 - 0.95*math.Max(0, math.Min(1, k))
		// Толщина меняется плавно: скачок скорости не рвёт линию.
		s.p += 0.35 * (target - s.p)
	}
	s.lx, s.ly, s.lt = x, y, t
	return s.point(x, y), true
}

func (s *Smoother) point(x, y float64) Point {
	return Point{X: float32(x), Y: float32(y), P: float32(s.p)}
}
