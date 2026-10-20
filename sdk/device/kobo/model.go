// Package kobo — реализация устройства для книг Kobo на i.MX (Mark 7): фреймбуфер e-ink
// через ioctl MXCFB, ввод через evdev, подсветка и батарея через sysfs, сон через /sys/power.
// Системные вызовы — в файлах *_linux_arm.go; модели, разбор событий и структуры ioctl
// здесь, в обычных файлах, и проверяются тестами на компьютере.
//
// Факты о железе взяты из FBInk, KOReader и Plato (ссылки — в
// docs/reference/Устройство Kobo Forma — снимок 19.10.2026.md); код написан заново.
package kobo

import (
	"fmt"
	"strings"
	"time"

	"el-book/sdk/device"
)

// Model — особенности модели.
type Model struct {
	Name     string
	Codename string
	Numbers  []string // номера моделей из /mnt/onboard/.kobo/version
	DPI      int
	Width    int // портретный размер экрана
	Height   int

	// Тач: сырые координаты панели → портрет. У большинства Kobo оси переставлены и X отражён.
	TouchSwitchXY bool
	TouchMirrorX  bool
	TouchMirrorY  bool
	// SnowProtocol — тач без ABS_MT_SLOT: ABS_MT_TRACKING_ID работает как номер слота,
	// а подъём пальца сообщается только BTN_TOUCH = 0.
	SnowProtocol bool

	// RotationRead — какое значение rotate фреймбуфер показывает, когда содержимое повёрнуто
	// на device.Rotation (портрет, по часовой, перевёрнуто, против часовой). Содержимое по
	// часовой — это книга, повёрнутая против часовой, то есть каноническое FB_ROTATE_CCW
	// в терминах FBInk и KOReader.
	RotationRead [4]uint32
	// OddWriteInverted — при записи нечётного поворота ядро его инвертирует: писать надо rotate ^ 2.
	OddWriteInverted bool

	BrightnessPath string
	WarmthPath     string
	WarmthMax      int
	WarmthInverted bool

	// SuspendSettle — сколько ждать после последнего обновления экрана, прежде чем просить
	// ядро уснуть: драйвер EPDC не засыпает, пока панель не обесточена и не остыла.
	// 0 — defaultSuspendSettle.
	SuspendSettle time.Duration

	// Keys — коды EV_KEY кнопок.
	Keys map[uint16]device.Key
}

// defaultSuspendSettle — пауза перед сном для модели без своей: самая долгая из известных.
const defaultSuspendSettle = 10 * time.Second

// forma — Kobo Forma (frost), Mark 7.
var forma = Model{
	Name: "Kobo Forma", Codename: "frost", Numbers: []string{"377", "380"},
	DPI: 300, Width: 1440, Height: 1920,
	TouchSwitchXY: true, TouchMirrorX: true, SnowProtocol: true,
	// Канонические повороты FBInk для Forma: UR→3, CW→2, UD→1, CCW→0.
	RotationRead:     [4]uint32{3, 0, 1, 2},
	OddWriteInverted: true,
	BrightnessPath:   "/sys/class/backlight/mxc_msp430.0/brightness",
	WarmthPath:       "/sys/class/backlight/tlc5947_bl/color",
	WarmthMax:        10,
	WarmthInverted:   true,
	// Ядро Forma (mxc_epdc_v2_fb.c): после выключения панели 1920×1440 засыпать можно через
	// 900 тиков, при HZ=100 это 9 с; ещё полсекунды — на само обновление и запас.
	SuspendSettle: 10 * time.Second,
	Keys: map[uint16]device.Key{
		116: device.KeyPower,
		59:  device.KeySleepCover,
		193: device.KeyPageBack,
		194: device.KeyPageForward,
		102: device.KeyHome,
	},
}

// Models — поддерживаемые модели.
var Models = []*Model{&forma}

// DefaultModel — модель, под которую платформа делается в первую очередь (Kobo Forma);
// по ней эмулятор берёт размер экрана и DPI.
func DefaultModel() Model { return forma }

// Detect находит модель по кодовому имени (PRODUCT) или номеру модели.
func Detect(codename, number string) (*Model, error) {
	for _, m := range Models {
		if codename != "" && m.Codename == codename {
			return m, nil
		}
		for _, n := range m.Numbers {
			if n == number {
				return m, nil
			}
		}
	}
	return nil, fmt.Errorf("модель Kobo не поддерживается: кодовое имя %q, номер %q", codename, number)
}

// parseVersion разбирает /mnt/onboard/.kobo/version: «серийный,ядро,прошивка,…,идентификатор»;
// номер модели — последние три цифры последнего поля.
func parseVersion(s string) (number, firmware string) {
	f := strings.Split(strings.TrimSpace(s), ",")
	if len(f) >= 3 {
		firmware = f[2]
	}
	if len(f) >= 6 {
		id := f[len(f)-1]
		if len(id) >= 3 {
			number = id[len(id)-3:]
		}
	}
	return number, firmware
}

// WriteRotation — значение, которое нужно записать во фреймбуфер для ориентации r.
func (m *Model) WriteRotation(r device.Rotation) uint32 {
	v := m.RotationRead[r&3]
	if m.OddWriteInverted && v&1 == 1 {
		v ^= 2
	}
	return v
}

// CanonicalFromRead переводит прочитанный поворот фреймбуфера в каноническую ориентацию.
func (m *Model) CanonicalFromRead(read uint32) (device.Rotation, bool) {
	for i, v := range m.RotationRead {
		if v == read&3 {
			return device.Rotation(i), true
		}
	}
	return device.Portrait, false
}

// touchToPortrait переводит сырые координаты тача в координаты портрета.
// rawW и rawH — диапазоны сырых осей (максимум + 1); 0 — оси уже в пикселях панели.
func (m *Model) touchToPortrait(rawX, rawY, rawW, rawH int) (int, int) {
	x, y := rawX, rawY
	w, h := m.Width, m.Height // размер в системе после перестановки осей
	if m.TouchSwitchXY {
		x, y = rawY, rawX
		if rawW > 0 && rawH > 0 {
			x, y = rawY*w/rawH, rawX*h/rawW
		}
	} else if rawW > 0 && rawH > 0 {
		x, y = rawX*w/rawW, rawY*h/rawH
	}
	if m.TouchMirrorX {
		x = w - 1 - x
	}
	if m.TouchMirrorY {
		y = h - 1 - y
	}
	return clampInt(x, 0, w-1), clampInt(y, 0, h-1)
}

// portraitToLogical переводит точку портрета в логические координаты при ориентации r.
func (m *Model) portraitToLogical(px, py int, r device.Rotation) (int, int) {
	w, h := m.Width, m.Height
	switch r & 3 {
	case device.LandscapeCW:
		return py, w - 1 - px
	case device.PortraitFlipped:
		return w - 1 - px, h - 1 - py
	case device.LandscapeCCW:
		return h - 1 - py, px
	}
	return px, py
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
