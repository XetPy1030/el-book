package kobo

import (
	"encoding/binary"
	"strings"
	"time"

	"el-book/sdk/device"
)

// Типы и коды evdev (linux/input-event-codes.h).
const (
	evSyn = 0x00
	evKey = 0x01
	evAbs = 0x03
	evMsc = 0x04

	synReport  = 0x00
	synDropped = 0x03
	btnTouch   = 0x14a

	absMTSlot       = 0x2f
	absMTPositionX  = 0x35
	absMTPositionY  = 0x36
	absMTTrackingID = 0x39

	mscRaw = 0x03
)

// Значения MSC_RAW акселерометра (drivers/hwmon/mma8x5x.c, по KOReader).
const (
	gsensorPortraitDown   = 0x17
	gsensorPortraitUp     = 0x18
	gsensorLandscapeRight = 0x19
	gsensorLandscapeLeft  = 0x1a
)

// rawEvent — struct input_event на 32-битном ARM: timeval из двух 32-битных полей.
type rawEvent struct {
	Sec, Usec int32
	Type      uint16
	Code      uint16
	Value     int32
}

const rawEventSize = 16

func (e rawEvent) time() time.Time { return time.Unix(int64(e.Sec), int64(e.Usec)*1000) }

// decodeEvents разбирает буфер прочитанных событий.
func decodeEvents(b []byte) []rawEvent {
	out := make([]rawEvent, 0, len(b)/rawEventSize)
	for len(b) >= rawEventSize {
		out = append(out, rawEvent{
			Sec:   int32(binary.LittleEndian.Uint32(b[0:])),
			Usec:  int32(binary.LittleEndian.Uint32(b[4:])),
			Type:  binary.LittleEndian.Uint16(b[8:]),
			Code:  binary.LittleEndian.Uint16(b[10:]),
			Value: int32(binary.LittleEndian.Uint32(b[12:])),
		})
		b = b[rawEventSize:]
	}
	return out
}

// contact — палец в слоте протокола B.
type contact struct {
	active bool // палец на экране
	isNew  bool // опустился в этом кадре
	moved  bool // координаты изменились в этом кадре
	lifted bool // поднялся в этом кадре
	x, y   int
	slot   int // номер для device.TouchEvent: первый палец — 0
}

// touchParser собирает события тача в касания. Понимает обычный multitouch protocol B и
// «snow»-вариант Kobo: без ABS_MT_SLOT, с TRACKING_ID вместо слота и подъёмом по BTN_TOUCH = 0.
// Если пришёл TRACKING_ID = -1, панель на самом деле обычная — квирк выключается.
type touchParser struct {
	snow     bool
	dropping bool // ядро потеряло события: ждём следующего SYN_REPORT
	cur      int
	slots    map[int]*contact
	toPoint  func(rawX, rawY int) (int, int) // сырые координаты → логические
	emit     func(device.Event)
}

func newTouchParser(snow bool, toPoint func(int, int) (int, int), emit func(device.Event)) *touchParser {
	return &touchParser{snow: snow, slots: map[int]*contact{}, toPoint: toPoint, emit: emit}
}

func (p *touchParser) slot(key int) *contact {
	c, ok := p.slots[key]
	if !ok {
		c = &contact{slot: -1}
		p.slots[key] = c
	}
	return c
}

// freeSlot — наименьший номер, не занятый активным пальцем.
func (p *touchParser) freeSlot() int {
	used := map[int]bool{}
	for _, c := range p.slots {
		if c.active && c.slot >= 0 {
			used[c.slot] = true
		}
	}
	for n := 0; ; n++ {
		if !used[n] {
			return n
		}
	}
}

func (p *touchParser) press(c *contact) {
	if !c.active {
		c.active, c.isNew, c.lifted = true, true, false
		c.slot = p.freeSlot()
	}
}

func (p *touchParser) feed(e rawEvent) {
	if p.dropping {
		if e.Type == evSyn && e.Code == synReport {
			p.dropping = false
		}
		return
	}
	switch e.Type {
	case evAbs:
		switch e.Code {
		case absMTSlot:
			p.cur = int(e.Value)
		case absMTTrackingID:
			if e.Value == -1 {
				p.snow = false
				if c := p.slot(p.cur); c.active {
					c.lifted = true
				}
				return
			}
			if p.snow {
				p.cur = int(e.Value)
			}
			p.press(p.slot(p.cur))
		case absMTPositionX:
			c := p.slot(p.cur)
			c.x, c.moved = int(e.Value), true
			if p.snow {
				p.press(c)
			}
		case absMTPositionY:
			c := p.slot(p.cur)
			c.y, c.moved = int(e.Value), true
			if p.snow {
				p.press(c)
			}
		}
	case evKey:
		if e.Code == btnTouch && e.Value == 0 {
			for _, c := range p.slots {
				if c.active {
					c.lifted = true
				}
			}
		}
	case evSyn:
		switch e.Code {
		case synReport:
			p.report(e.time())
		case synDropped:
			p.dropping = true
		}
	}
}

// report выдаёт касания кадра: опускания, движения, подъёмы.
func (p *touchParser) report(t time.Time) {
	for _, c := range p.slots {
		if !c.active {
			continue
		}
		x, y := p.toPoint(c.x, c.y)
		switch {
		case c.isNew:
			p.emit(device.TouchEvent{Time: t, Slot: c.slot, Phase: device.TouchDown, X: x, Y: y})
			if c.lifted {
				p.emit(device.TouchEvent{Time: t, Slot: c.slot, Phase: device.TouchUp, X: x, Y: y})
			}
		case c.lifted:
			p.emit(device.TouchEvent{Time: t, Slot: c.slot, Phase: device.TouchUp, X: x, Y: y})
		case c.moved:
			p.emit(device.TouchEvent{Time: t, Slot: c.slot, Phase: device.TouchMove, X: x, Y: y})
		}
		if c.lifted {
			c.active = false
			c.slot = -1
		}
		c.isNew, c.moved, c.lifted = false, false, false
	}
}

// keyEvent переводит EV_KEY в событие кнопки; автоповтор (значение 2) пропускается.
func keyEvent(keys map[uint16]device.Key, e rawEvent) (device.Event, bool) {
	if e.Type != evKey || e.Value == 2 {
		return nil, false
	}
	k, ok := keys[e.Code]
	if !ok {
		return nil, false
	}
	return device.KeyEvent{Time: e.time(), Key: k, Pressed: e.Value == 1}, true
}

// orientationEvent переводит сообщение акселерометра в ориентацию содержимого, при которой
// оно стоит прямо. LANDSCAPE_RIGHT — книга повёрнута по часовой (так у KOReader), значит
// содержимое нужно повернуть против часовой.
func orientationEvent(e rawEvent) (device.Event, bool) {
	if e.Type != evMsc || e.Code != mscRaw {
		return nil, false
	}
	var r device.Rotation
	switch e.Value {
	case gsensorPortraitUp:
		r = device.Portrait
	case gsensorLandscapeRight:
		r = device.LandscapeCCW
	case gsensorPortraitDown:
		r = device.PortraitFlipped
	case gsensorLandscapeLeft:
		r = device.LandscapeCW
	default:
		return nil, false
	}
	return device.OrientationEvent{Time: e.time(), Rotation: r}, true
}

// parseHardwareStatus переводит строку FIFO /tmp/nickel-hardware-status, куда пишет udev.
func parseHardwareStatus(line string) (device.Event, bool) {
	now := time.Now()
	switch line {
	case "usb plug add":
		return device.PowerSourceEvent{Time: now, Source: device.SourceComputer, Connected: true}, true
	case "usb plug remove":
		return device.PowerSourceEvent{Time: now, Source: device.SourceComputer, Connected: false}, true
	case "usb ac add":
		return device.PowerSourceEvent{Time: now, Source: device.SourceCharger, Connected: true}, true
	case "usb ac remove":
		return device.PowerSourceEvent{Time: now, Source: device.SourceCharger, Connected: false}, true
	}
	return nil, false
}

// inputKind — что за устройство ввода.
type inputKind int

const (
	inputKeys  inputKind = iota // кнопки, обложка и повороты от акселерометра NTX
	inputTouch                  // тачскрин
	inputSkip                   // сырые данные акселерометра: их не читаем
)

func (k inputKind) String() string {
	switch k {
	case inputKeys:
		return "кнопки"
	case inputTouch:
		return "тач"
	}
	return "пропущено"
}

// classifyInput определяет назначение устройства по имени и наличию осей multitouch.
// Повороты NTX приходят в главное устройство кнопок, а отдельное устройство акселерометра
// шлёт сырые данные непрерывно — его пропускаем, как KOReader.
func classifyInput(name string, hasMT bool) inputKind {
	switch {
	case hasMT:
		return inputTouch
	case strings.Contains(strings.ToLower(name), "accelerometer"):
		return inputSkip
	}
	return inputKeys
}

// touchScale возвращает диапазоны сырых осей для масштабирования или 0, 0, если оси уже
// в пикселях панели (диапазоны совпадают с размером экрана в любом порядке).
func touchScale(m *Model, absX, absY inputAbsInfo) (int, int) {
	w, h := int(absX.Maximum)+1, int(absY.Maximum)+1
	if absX.Maximum <= 0 || absY.Maximum <= 0 ||
		(w == m.Width && h == m.Height) || (w == m.Height && h == m.Width) {
		return 0, 0
	}
	return w, h
}
