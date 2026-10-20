// Package emu — эмулятор книги для разработки на компьютере. Панель меняется только по
// запросам обновления, как настоящая: видно, какие области и какими режимами обновляет
// приложение. Управляется из браузера (Serve) или скриптом (RunScript).
package emu

import (
	"errors"
	"image"
	"image/draw"
	"sync"
	"time"

	"el-book/sdk/device"
	"el-book/sdk/device/kobo"
)

// Options — параметры эмулятора.
type Options struct {
	Width, Height int // портретный размер; по умолчанию — как у kobo.DefaultModel
	DPI           int // по умолчанию — как у kobo.DefaultModel
	// SimulateTiming заставляет Wait ждать примерное время обновления, как на панели.
	SimulateTiming bool
	// AutoWake: Suspend возвращается сразу (для скриптов и тестов).
	AutoWake bool
}

// UpdateRecord — одно обновление панели, для журнала и браузера.
type UpdateRecord struct {
	Seq      int             `json:"seq"`
	Rect     image.Rectangle `json:"-"`
	X        int             `json:"x"`
	Y        int             `json:"y"`
	W        int             `json:"w"`
	H        int             `json:"h"`
	Waveform string          `json:"waveform"`
	Full     bool            `json:"full"`
	Time     time.Time       `json:"-"`
	pixels   *image.Gray
}

// Emulator — эмулированная книга.
type Emulator struct {
	opts   Options
	events chan device.Event

	mu         sync.Mutex
	rotation   device.Rotation
	panel      *image.Gray // что показывает панель
	seq        int
	history    []UpdateRecord // последние обновления, с пикселями
	listeners  map[chan UpdateRecord]bool
	brightness int
	warmth     int
	battery    int
	charging   bool
	sleeping   bool
	wake       chan struct{}
	closed     bool
	lastInput  time.Time
	lastUpdate time.Time
	waitUntil  map[device.Marker]time.Time
	net        *network
}

// New создаёт эмулятор.
func New(opts Options) *Emulator {
	m := kobo.DefaultModel()
	if opts.Width == 0 || opts.Height == 0 {
		opts.Width, opts.Height = m.Width, m.Height
	}
	if opts.DPI == 0 {
		opts.DPI = m.DPI
	}
	e := &Emulator{
		opts:       opts,
		events:     make(chan device.Event, 4096),
		listeners:  map[chan UpdateRecord]bool{},
		brightness: 20,
		warmth:     30,
		battery:    87,
		waitUntil:  map[device.Marker]time.Time{},
	}
	e.panel = image.NewGray(e.bounds())
	for i := range e.panel.Pix {
		e.panel.Pix[i] = 255
	}
	e.net = newNetwork(e)
	return e
}

func (e *Emulator) bounds() image.Rectangle {
	if e.rotation.Landscape() {
		return image.Rect(0, 0, e.opts.Height, e.opts.Width)
	}
	return image.Rect(0, 0, e.opts.Width, e.opts.Height)
}

func (e *Emulator) Info() device.Info {
	return device.Info{Model: "Эмулятор Kobo Forma", Codename: "emu", Firmware: ""}
}

// Describe — сведения для диагностики.
func (e *Emulator) Describe() []device.Fact {
	timing := "мгновенно"
	if e.opts.SimulateTiming {
		timing = "как на книге"
	}
	return []device.Fact{
		{Name: "Устройство", Value: "эмулятор"},
		{Name: "Обновление панели", Value: timing},
	}
}

func (e *Emulator) Display() device.Display     { return (*display)(e) }
func (e *Emulator) Events() <-chan device.Event { return e.events }
func (e *Emulator) Light() device.Light         { return (*light)(e) }
func (e *Emulator) Battery() device.Battery     { return (*battery)(e) }
func (e *Emulator) Power() device.Power         { return (*power)(e) }

// Close закрывает поток событий.
func (e *Emulator) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.closed = true
		close(e.events)
	}
	return nil
}

// Panel возвращает копию того, что сейчас показывает панель.
func (e *Emulator) Panel() *image.Gray {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := image.NewGray(e.panel.Bounds())
	copy(cp.Pix, e.panel.Pix)
	return cp
}

// History возвращает последние обновления без пикселей.
func (e *Emulator) History() []UpdateRecord {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]UpdateRecord, len(e.history))
	for i, r := range e.history {
		r.pixels = nil
		out[i] = r
	}
	return out
}

// send кладёт событие в поток; после Close события молча теряются.
func (e *Emulator) send(ev device.Event) {
	e.mu.Lock()
	closed := e.closed
	e.lastInput = time.Now()
	e.mu.Unlock()
	if !closed {
		e.events <- ev
	}
}

// Touch эмулирует касание первым пальцем.
func (e *Emulator) Touch(phase device.TouchPhase, x, y int) {
	e.touchAt(phase, x, y, time.Now())
}

// touchAt — касание с заданным временем события: скрипты ведут палец по точному расписанию.
func (e *Emulator) touchAt(phase device.TouchPhase, x, y int, at time.Time) {
	e.send(device.TouchEvent{Time: at, Slot: 0, Phase: phase, X: x, Y: y})
}

// Key эмулирует нажатие или отпускание кнопки. Кнопка питания будит спящую книгу.
func (e *Emulator) Key(k device.Key, pressed bool) {
	e.mu.Lock()
	sleeping := e.sleeping
	e.mu.Unlock()
	if sleeping {
		if (k == device.KeyPower && pressed) || (k == device.KeySleepCover && !pressed) {
			e.Wake()
			// Как на книге: кнопка, которой разбудили, приходит событием уже после пробуждения.
			e.send(device.KeyEvent{Time: time.Now(), Key: k, Pressed: pressed})
		}
		return
	}
	e.send(device.KeyEvent{Time: time.Now(), Key: k, Pressed: pressed})
}

// PowerSource эмулирует подключение кабеля.
func (e *Emulator) PowerSource(src device.PowerSource, connected bool) {
	e.mu.Lock()
	e.charging = connected
	e.mu.Unlock()
	e.send(device.PowerSourceEvent{Time: time.Now(), Source: src, Connected: connected})
	e.broadcastState()
}

// Orientation эмулирует поворот книги в руках.
func (e *Emulator) Orientation(r device.Rotation) {
	e.send(device.OrientationEvent{Time: time.Now(), Rotation: r})
}

// SetBattery задаёт заряд.
func (e *Emulator) SetBattery(level int, charging bool) {
	e.mu.Lock()
	e.battery, e.charging = level, charging
	e.mu.Unlock()
	e.broadcastState()
}

// Wake будит книгу из сна.
func (e *Emulator) Wake() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sleeping && e.wake != nil {
		close(e.wake)
		e.wake = nil
	}
}

// Sleeping — спит ли книга.
func (e *Emulator) Sleeping() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sleeping
}

// Idle — первый кадр уже на панели, с последнего ввода и последнего обновления прошло
// не меньше d и очередь пуста. Без первого кадра книга не «затихла», а ещё не начала:
// иначе снимок сразу после запуска выходит пустым.
func (e *Emulator) Idle(d time.Duration) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	return e.seq > 0 && len(e.events) == 0 && now.Sub(e.lastInput) >= d && now.Sub(e.lastUpdate) >= d
}

// Subscribe подписывает на обновления панели; отписка — Unsubscribe.
func (e *Emulator) Subscribe() chan UpdateRecord {
	ch := make(chan UpdateRecord, 512)
	e.mu.Lock()
	e.listeners[ch] = true
	e.mu.Unlock()
	return ch
}

func (e *Emulator) Unsubscribe(ch chan UpdateRecord) {
	e.mu.Lock()
	delete(e.listeners, ch)
	e.mu.Unlock()
}

// stateRecord — особая запись: изменилось состояние (сон, заряд, поворот), а не пиксели.
func (e *Emulator) broadcastState() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ch := range e.listeners {
		select {
		case ch <- UpdateRecord{Seq: -1}:
		default:
		}
	}
}

// RectPixels возвращает пиксели обновления seq, если оно ещё в истории.
func (e *Emulator) RectPixels(seq int) (*image.Gray, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.history {
		if r.Seq == seq && r.pixels != nil {
			return r.pixels, true
		}
	}
	return nil, false
}

// State — состояние для браузера.
type State struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Rotation   int    `json:"rotation"`
	Sleeping   bool   `json:"sleeping"`
	Battery    int    `json:"battery"`
	Charging   bool   `json:"charging"`
	Brightness int    `json:"brightness"`
	Warmth     int    `json:"warmth"`
	Seq        int    `json:"seq"`
	Model      string `json:"model"`
	Wifi       string `json:"wifi"`
	Offline    bool   `json:"offline"`
}

func (e *Emulator) State() State {
	ns := e.net.Status()
	e.net.mu.Lock()
	offline := e.net.offline
	e.net.mu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	b := e.bounds()
	wifi := ns.State.String()
	if ns.SSID != "" {
		wifi += ", " + ns.SSID
	}
	return State{
		Width: b.Dx(), Height: b.Dy(), Rotation: int(e.rotation), Sleeping: e.sleeping,
		Battery: e.battery, Charging: e.charging, Brightness: e.brightness, Warmth: e.warmth,
		Seq: e.seq, Model: "Kobo Forma", Wifi: wifi, Offline: offline,
	}
}

type display Emulator

func (d *display) e() *Emulator { return (*Emulator)(d) }

func (d *display) Bounds() image.Rectangle {
	e := d.e()
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.bounds()
}

func (d *display) DPI() int { return d.e().opts.DPI }

func (d *display) Rotation() device.Rotation {
	e := d.e()
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rotation
}

func (d *display) SetRotation(r device.Rotation) error {
	if r < 0 || r > 3 {
		return errors.New("недопустимый поворот")
	}
	e := d.e()
	e.mu.Lock()
	e.rotation = r
	e.panel = image.NewGray(e.bounds())
	for i := range e.panel.Pix {
		e.panel.Pix[i] = 255
	}
	e.mu.Unlock()
	e.broadcastState()
	return nil
}

var durations = map[device.Waveform]time.Duration{
	device.WaveformDU: 260 * time.Millisecond, device.WaveformA2: 120 * time.Millisecond,
	device.WaveformGL16: 450 * time.Millisecond, device.WaveformREAGL: 450 * time.Millisecond,
	device.WaveformGC16: 600 * time.Millisecond, device.WaveformAuto: 600 * time.Millisecond,
}

func (d *display) Present(src *image.Gray, u device.Update) (device.Marker, error) {
	e := d.e()
	e.mu.Lock()
	r := u.Rect.Intersect(e.panel.Bounds()).Intersect(src.Bounds())
	if r.Empty() {
		e.mu.Unlock()
		return 0, nil
	}
	pix := src
	if u.Waveform == device.WaveformDU || u.Waveform == device.WaveformA2 {
		pix = twoLevel(src, r)
	}
	draw.Draw(e.panel, r, pix, r.Min, draw.Src)
	e.seq++
	crop := image.NewGray(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(crop, crop.Bounds(), e.panel, r.Min, draw.Src)
	rec := UpdateRecord{
		Seq: e.seq, Rect: r, X: r.Min.X, Y: r.Min.Y, W: r.Dx(), H: r.Dy(),
		Waveform: u.Waveform.String(), Full: u.Full, Time: time.Now(), pixels: crop,
	}
	e.history = append(e.history, rec)
	if len(e.history) > 256 {
		e.history = e.history[len(e.history)-256:]
	}
	e.lastUpdate = time.Now()
	marker := device.Marker(e.seq)
	dur := durations[u.Waveform]
	if u.Full {
		dur += dur / 4
	}
	e.waitUntil[marker] = time.Now().Add(dur)
	for ch := range e.listeners {
		select {
		case ch <- rec:
		default:
		}
	}
	e.mu.Unlock()
	return marker, nil
}

// twoLevel показывает, что сделает двухцветный режим: серое станет чёрным или белым.
func twoLevel(src *image.Gray, r image.Rectangle) *image.Gray {
	out := image.NewGray(src.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			v := src.Pix[src.PixOffset(x, y)]
			if v < 128 {
				v = 0
			} else {
				v = 255
			}
			out.Pix[out.PixOffset(x, y)] = v
		}
	}
	return out
}

func (d *display) Wait(m device.Marker) error {
	e := d.e()
	if !e.opts.SimulateTiming {
		return nil
	}
	e.mu.Lock()
	until, ok := e.waitUntil[m]
	delete(e.waitUntil, m)
	e.mu.Unlock()
	if ok {
		time.Sleep(time.Until(until))
	}
	return nil
}

type light Emulator

func (l *light) Brightness() int {
	e := (*Emulator)(l)
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.brightness
}

func (l *light) HasWarmth() bool { return true }

func (l *light) Warmth() int {
	e := (*Emulator)(l)
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.warmth
}

func (l *light) SetBrightness(p int) error {
	e := (*Emulator)(l)
	e.mu.Lock()
	e.brightness = clamp(p, 0, 100)
	e.mu.Unlock()
	e.broadcastState()
	return nil
}

func (l *light) SetWarmth(p int) error {
	e := (*Emulator)(l)
	e.mu.Lock()
	e.warmth = clamp(p, 0, 100)
	e.mu.Unlock()
	e.broadcastState()
	return nil
}

type battery Emulator

func (b *battery) Level() (int, error) {
	e := (*Emulator)(b)
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.battery, nil
}

func (b *battery) Charging() (bool, error) {
	e := (*Emulator)(b)
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.charging, nil
}

type power Emulator

func (p *power) Suspend() error {
	e := (*Emulator)(p)
	if e.opts.AutoWake {
		return nil
	}
	e.mu.Lock()
	e.sleeping = true
	wake := make(chan struct{})
	e.wake = wake
	e.mu.Unlock()
	e.broadcastState()
	<-wake
	e.mu.Lock()
	e.sleeping = false
	e.mu.Unlock()
	e.broadcastState()
	return nil
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
