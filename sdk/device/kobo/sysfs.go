package kobo

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// sysfs — файлы ядра относительно корня root: на книге «/», в тестах — временная папка.
type sysfs struct {
	root string
}

func (s sysfs) path(p string) string { return filepath.Join(s.root, p) }

func (s sysfs) exists(p string) bool {
	_, err := os.Stat(s.path(p))
	return err == nil
}

func (s sysfs) readString(p string) (string, error) {
	b, err := os.ReadFile(s.path(p))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (s sysfs) readInt(p string) (int, error) {
	v, err := s.readString(p)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", p, err)
	}
	return n, nil
}

// write пишет значение в существующий файл sysfs: без создания и без усечения.
func (s sysfs) write(p, value string) error {
	f, err := os.OpenFile(s.path(p), os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, werr := f.WriteString(value)
	cerr := f.Close()
	if werr != nil {
		return fmt.Errorf("%s: %w", p, werr)
	}
	return cerr
}

func (s sysfs) writeInt(p string, v int) error { return s.write(p, strconv.Itoa(v)) }

// light — подсветка через sysfs. Файл яркости только для записи, поэтому текущие значения
// хранятся здесь; при старте яркость берётся из actual_brightness, если он есть.
type light struct {
	fs         sysfs
	model      *Model
	mu         sync.Mutex
	brightness int
	warmth     int
}

func newLight(fs sysfs, m *Model) *light {
	l := &light{fs: fs, model: m, warmth: -1}
	if v, err := fs.readInt(filepath.Join(filepath.Dir(m.BrightnessPath), "actual_brightness")); err == nil {
		l.brightness = clampInt(v, 0, 100)
	}
	return l
}

func (l *light) Brightness() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.brightness
}

func (l *light) SetBrightness(percent int) error {
	percent = clampInt(percent, 0, 100)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.fs.writeInt(l.model.BrightnessPath, percent); err != nil {
		return fmt.Errorf("яркость: %w", err)
	}
	l.brightness = percent
	return nil
}

func (l *light) HasWarmth() bool { return l.model.WarmthPath != "" }

func (l *light) Warmth() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.warmth < 0 {
		return 0
	}
	return l.warmth
}

func (l *light) SetWarmth(percent int) error {
	if !l.HasWarmth() {
		return nil
	}
	percent = clampInt(percent, 0, 100)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.fs.writeInt(l.model.WarmthPath, warmthValue(l.model, percent)); err != nil {
		return fmt.Errorf("теплота: %w", err)
	}
	l.warmth = percent
	return nil
}

// warmthValue переводит теплоту 0…100 в шкалу устройства; у Forma 0…10 и инвертирована.
func warmthValue(m *Model, percent int) int {
	v := (percent*m.WarmthMax + 50) / 100
	if m.WarmthInverted {
		v = m.WarmthMax - v
	}
	return v
}

// battery — батарея через /sys/class/power_supply.
type battery struct {
	fs  sysfs
	dir string
}

func (b battery) Level() (int, error) {
	v, err := b.fs.readInt(filepath.Join(b.dir, "capacity"))
	if err != nil {
		return 0, fmt.Errorf("заряд: %w", err)
	}
	return clampInt(v, 0, 100), nil
}

// Charging — как у KOReader: заряжается всё, что не «Discharging», в том числе полная батарея на кабеле.
func (b battery) Charging() (bool, error) {
	v, err := b.fs.readString(filepath.Join(b.dir, "status"))
	if err != nil {
		return false, fmt.Errorf("состояние батареи: %w", err)
	}
	return v != "Discharging", nil
}

// batteryDirs — где искать батарею: у i.MX Kobo — mc13892_bat, у новых — battery.
var batteryDirs = []string{"/sys/class/power_supply/mc13892_bat", "/sys/class/power_supply/battery"}

func findBattery(fs sysfs) battery {
	for _, d := range batteryDirs {
		if fs.exists(filepath.Join(d, "capacity")) {
			return battery{fs: fs, dir: d}
		}
	}
	return battery{fs: fs, dir: batteryDirs[0]}
}
