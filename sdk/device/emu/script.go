package emu

import (
	"bufio"
	"fmt"
	"image"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"el-book/sdk/device"
)

// RunScript выполняет скрипт: по команде в строке, «#» — комментарий.
//
//	wait 500                      пауза, мс
//	tap 720 960                   тап
//	longpress 720 960 [800]       долгое нажатие, мс
//	swipe 1200 960 200 960 [250]  свайп за указанное время, мс
//	drag x1 y1 x2 y2 ...          ведение пальцем по ломаной: штрих в рисовалке
//	hold мс x1 y1 x2 y2 ...       то же, но в конце палец замирает на мс: фигура в рисовалке
//	key power|pgfwd|pgback|home   нажать и отпустить кнопку
//	cover close|open              обложка
//	usb computer|charger on|off   кабель
//	battery 15 [charging]         заряд
//	orient 0..3                   книгу повернули (событие датчика)
//	net offline|online            Wi-Fi не находит знакомых сетей / снова находит
//	shot имя                      снимок панели; shot получает имя и картинку
//
// Перед снимком и после каждого действия скрипт ждёт, пока приложение затихнет.
func (e *Emulator) RunScript(r io.Reader, shot func(name string, img *image.Gray) error) error {
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if err := e.runCommand(f, shot); err != nil {
			return fmt.Errorf("строка %d %q: %w", n, line, err)
		}
	}
	return sc.Err()
}

func (e *Emulator) runCommand(f []string, shot func(string, *image.Gray) error) error {
	nums := func(from int) ([]int, error) {
		var out []int
		for _, s := range f[from:] {
			v, err := strconv.Atoi(s)
			if err != nil {
				return nil, fmt.Errorf("ожидалось число, получено %q", s)
			}
			out = append(out, v)
		}
		return out, nil
	}
	switch f[0] {
	case "wait":
		v, err := nums(1)
		if err != nil || len(v) != 1 {
			return fmt.Errorf("wait <мс>")
		}
		time.Sleep(time.Duration(v[0]) * time.Millisecond)
		return nil
	case "tap":
		v, err := nums(1)
		if err != nil || len(v) != 2 {
			return fmt.Errorf("tap <x> <y>")
		}
		e.Touch(device.TouchDown, v[0], v[1])
		time.Sleep(40 * time.Millisecond)
		e.Touch(device.TouchUp, v[0], v[1])
	case "longpress":
		v, err := nums(1)
		if err != nil || len(v) < 2 {
			return fmt.Errorf("longpress <x> <y> [мс]")
		}
		d := 800
		if len(v) > 2 {
			d = v[2]
		}
		e.Touch(device.TouchDown, v[0], v[1])
		time.Sleep(time.Duration(d) * time.Millisecond)
		e.Touch(device.TouchUp, v[0], v[1])
	case "swipe":
		v, err := nums(1)
		if err != nil || len(v) < 4 {
			return fmt.Errorf("swipe <x1> <y1> <x2> <y2> [мс]")
		}
		d := 250
		if len(v) > 4 {
			d = v[4]
		}
		e.stroke([]image.Point{{v[0], v[1]}, {v[2], v[3]}}, time.Duration(d)*time.Millisecond, 0)
	case "drag":
		v, err := nums(1)
		if err != nil || len(v) < 4 || len(v)%2 != 0 {
			return fmt.Errorf("drag x1 y1 x2 y2 ...")
		}
		var pts []image.Point
		for i := 0; i < len(v); i += 2 {
			pts = append(pts, image.Pt(v[i], v[i+1]))
		}
		e.stroke(pts, 0, 0)
	case "hold":
		v, err := nums(1)
		if err != nil || len(v) < 5 || len(v)%2 != 1 {
			return fmt.Errorf("hold <мс> x1 y1 x2 y2 ...")
		}
		var pts []image.Point
		for i := 1; i < len(v); i += 2 {
			pts = append(pts, image.Pt(v[i], v[i+1]))
		}
		e.stroke(pts, 0, time.Duration(v[0])*time.Millisecond)
	case "key":
		k, ok := keyNames[arg(f, 1)]
		if !ok {
			return fmt.Errorf("key power|pgfwd|pgback|home")
		}
		e.Key(k, true)
		time.Sleep(30 * time.Millisecond)
		e.Key(k, false)
	case "cover":
		switch arg(f, 1) {
		case "close":
			e.Key(device.KeySleepCover, true)
		case "open":
			e.Key(device.KeySleepCover, false)
		default:
			return fmt.Errorf("cover close|open")
		}
	case "usb":
		src := device.SourceComputer
		if arg(f, 1) == "charger" {
			src = device.SourceCharger
		}
		e.PowerSource(src, arg(f, 2) != "off")
	case "battery":
		level, err := strconv.Atoi(arg(f, 1))
		if err != nil {
			return fmt.Errorf("battery <уровень> [charging]")
		}
		e.SetBattery(level, arg(f, 2) == "charging")
	case "orient":
		v, err := nums(1)
		if err != nil || len(v) != 1 {
			return fmt.Errorf("orient 0..3")
		}
		e.Orientation(device.Rotation(v[0]))
	case "net":
		switch arg(f, 1) {
		case "offline":
			e.SetOffline(true)
		case "online":
			e.SetOffline(false)
		default:
			return fmt.Errorf("net offline|online")
		}
	case "shot":
		if len(f) != 2 {
			return fmt.Errorf("shot <имя>")
		}
		e.WaitIdle(150*time.Millisecond, 10*time.Second)
		return shot(f[1], e.Panel())
	default:
		return fmt.Errorf("неизвестная команда %q", f[0])
	}
	e.WaitIdle(120*time.Millisecond, 10*time.Second)
	return nil
}

func arg(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

// stroke ведёт палец по ломаной: точка через каждые несколько пикселей, как у настоящего тача.
// hold — сколько палец стоит в конце, прежде чем подняться.
func (e *Emulator) stroke(pts []image.Point, total, hold time.Duration) {
	var path []image.Point
	path = append(path, pts[0])
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		steps := int(math.Ceil(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)) / 12))
		for s := 1; s <= steps; s++ {
			t := float64(s) / float64(steps)
			path = append(path, image.Pt(a.X+int(math.Round(float64(b.X-a.X)*t)), a.Y+int(math.Round(float64(b.Y-a.Y)*t))))
		}
	}
	step := 8 * time.Millisecond
	if total > 0 && len(path) > 1 {
		step = total / time.Duration(len(path)-1)
	}
	// Время касаний — точное расписание, а не момент, когда проснулся Sleep: скорость пальца
	// (от неё зависит толщина пера) одинакова в каждом прогоне, и снимки не плывут.
	start := time.Now()
	e.touchAt(device.TouchDown, path[0].X, path[0].Y, start)
	at := start
	for _, p := range path[1:] {
		at = at.Add(step)
		time.Sleep(time.Until(at))
		e.touchAt(device.TouchMove, p.X, p.Y, at)
	}
	last := path[len(path)-1]
	if hold > 0 {
		at = at.Add(hold)
		time.Sleep(time.Until(at))
	}
	e.touchAt(device.TouchUp, last.X, last.Y, at)
}

// WaitIdle ждёт, пока приложение затихнет: события разобраны, обновлений нет quiet подряд.
func (e *Emulator) WaitIdle(quiet, limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if e.Idle(quiet) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
