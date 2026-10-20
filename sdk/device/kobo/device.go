package kobo

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"el-book/sdk/device"
)

// Device — книга Kobo. Создаётся через Open (только на книге).
type Device struct {
	model   *Model
	info    device.Info
	display *display
	light   *light
	battery battery
	power   power
	wifi    *wifi // nil — скрипт запуска не передал модуль Wi-Fi

	inputs   []InputInfo
	warnings []string

	events  chan device.Event
	wake    chan struct{} // законные пробуждения для power
	done    chan struct{}
	wg      sync.WaitGroup
	mu      sync.Mutex
	closers []io.Closer
	closed  bool
	once    sync.Once
}

var (
	_ device.Device    = (*Device)(nil)
	_ device.Describer = (*Device)(nil)
	_ device.Networker = (*Device)(nil)
)

// hardwareStatusFIFO — канал, в который udev пишет строки о USB и сети.
const hardwareStatusFIFO = "/tmp/nickel-hardware-status"

// newDevice собирает устройство; log получает подробности сна (nil — без журнала).
func newDevice(m *Model, info device.Info, disp *display, fs sysfs, log *slog.Logger) *Device {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	d := &Device{
		model:   m,
		info:    info,
		display: disp,
		battery: findBattery(fs),
		events:  make(chan device.Event, 64),
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	d.power = power{
		fs: fs, sync: func() { syscall.Sync() }, now: time.Now, shown: disp.lastUpdate,
		wake: d.wake, log: log, t: defaultTiming(m.SuspendSettle),
	}
	if m.BrightnessPath != "" && fs.exists(m.BrightnessPath) {
		d.light = newLight(fs, m)
	}
	return d
}

func (d *Device) Info() device.Info           { return d.info }
func (d *Device) Display() device.Display     { return d.display }
func (d *Device) Events() <-chan device.Event { return d.events }
func (d *Device) Battery() device.Battery     { return d.battery }
func (d *Device) Power() device.Power         { return d.power }

// Model — особенности модели: нужны диагностике и спайкам.
func (d *Device) Model() *Model { return d.model }

// Inputs — найденные устройства ввода.
func (d *Device) Inputs() []InputInfo { return d.inputs }

// Warnings — что не удалось включить при открытии, но без чего книга работает.
func (d *Device) Warnings() []string { return d.warnings }

func (d *Device) addCloser(c io.Closer) {
	d.mu.Lock()
	d.closers = append(d.closers, c)
	d.mu.Unlock()
}

// touchPoint переводит сырые координаты тача в логические с учётом текущего поворота.
func (d *Device) touchPoint(rx, ry, rawW, rawH int) (int, int) {
	px, py := d.model.touchToPortrait(rx, ry, rawW, rawH)
	return d.model.portraitToLogical(px, py, d.display.Rotation())
}

// Describe — подробные сведения для диагностики.
func (d *Device) Describe() []device.Fact {
	m := d.model
	facts := []device.Fact{
		{Name: "Модель", Value: fmt.Sprintf("%s (%s, номера %s)", m.Name, m.Codename, strings.Join(m.Numbers, ", "))},
		{Name: "Прошивка", Value: d.info.Firmware},
		{Name: "Фреймбуфер", Value: d.display.describe()},
	}
	for _, in := range d.inputs {
		v := in.Name + " — " + in.Kind
		if in.Kind == inputTouch.String() {
			v += fmt.Sprintf(", x %d…%d, y %d…%d", in.AbsX[0], in.AbsX[1], in.AbsY[0], in.AbsY[1])
		}
		facts = append(facts, device.Fact{Name: "Ввод " + in.Path, Value: v})
	}
	facts = append(facts, device.Fact{Name: "Батарея", Value: d.battery.dir})
	if d.light != nil {
		facts = append(facts, device.Fact{Name: "Подсветка", Value: m.BrightnessPath + "; теплота " + m.WarmthPath})
	}
	if d.wifi != nil {
		st := d.wifi.Status()
		facts = append(facts, device.Fact{Name: "Wi-Fi", Value: d.wifi.iface + ", " + st.State.String()})
	}
	for _, w := range d.warnings {
		facts = append(facts, device.Fact{Name: "Предупреждение", Value: w})
	}
	return facts
}

// Network — Wi-Fi; nil, если скрипт запуска не передал модуль Wi-Fi.
func (d *Device) Network() device.Network {
	if d.wifi == nil {
		return nil
	}
	return d.wifi
}

// Light возвращает nil, если подсветки нет.
func (d *Device) Light() device.Light {
	if d.light == nil {
		return nil
	}
	return d.light
}

// send отдаёт событие потребителю; после Close события отбрасываются.
// Кнопка питания, открытая обложка и кабель компьютера заодно подтверждают пробуждение.
func (d *Device) send(e device.Event) {
	if isWakeEvent(e) {
		select {
		case d.wake <- struct{}{}:
		default:
		}
	}
	select {
	case d.events <- e:
	case <-d.done:
	}
}

// trySend отдаёт событие, если в очереди есть место, и не ждёт потребителя: так шлёт
// события Wi-Fi, которому нельзя застревать под своей блокировкой. Пропущенное состояние
// сети можно перечитать через Status.
func (d *Device) trySend(e device.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	select {
	case d.events <- e:
	default:
	}
}

// isWakeEvent — событие, которым пользователь будит книгу.
func isWakeEvent(e device.Event) bool {
	switch e := e.(type) {
	case device.KeyEvent:
		return (e.Key == device.KeyPower && e.Pressed) || (e.Key == device.KeySleepCover && !e.Pressed)
	case device.PowerSourceEvent:
		return e.Source == device.SourceComputer && e.Connected
	}
	return false
}

// run запускает источник событий; Close дожидается его завершения.
func (d *Device) run(f func()) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		f()
	}()
}

// readEvents читает события evdev из r, пока его не закроют.
func (d *Device) readEvents(r io.Reader, handle func(rawEvent)) {
	buf := make([]byte, 64*rawEventSize)
	for {
		n, err := r.Read(buf)
		if err != nil {
			return
		}
		for _, e := range decodeEvents(buf[:n]) {
			handle(e)
		}
	}
}

// keysHandler — кнопки, обложка и повороты от акселерометра.
func (d *Device) keysHandler() func(rawEvent) {
	return func(e rawEvent) {
		if ev, ok := keyEvent(d.model.Keys, e); ok {
			d.send(ev)
		} else if ev, ok := orientationEvent(e); ok {
			d.send(ev)
		}
	}
}

// touchHandler — тачскрин; rawW и rawH — диапазоны осей для масштабирования (0 — не нужно).
func (d *Device) touchHandler(rawW, rawH int) func(rawEvent) {
	p := newTouchParser(d.model.SnowProtocol, func(x, y int) (int, int) {
		return d.touchPoint(x, y, rawW, rawH)
	}, d.send)
	return p.feed
}

// watchHardwareStatus читает строки udev из FIFO. Открытие на чтение и запись не блокируется
// и не даёт конца файла, когда очередной писатель закрывает канал (так делает Plato).
func (d *Device) watchHardwareStatus(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	d.addCloser(f)
	d.run(func() {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if ev, ok := parseHardwareStatus(sc.Text()); ok {
				d.send(ev)
			}
		}
	})
	return nil
}

// Close останавливает чтение событий, закрывает канал событий и экран.
func (d *Device) Close() error {
	var errs []error
	d.once.Do(func() {
		close(d.done)
		d.mu.Lock()
		closers := d.closers
		d.closers = nil
		d.closed = true
		d.mu.Unlock()
		for _, c := range closers {
			if err := c.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				errs = append(errs, err)
			}
		}
		d.wg.Wait()
		close(d.events)
		if d.display != nil {
			if err := d.display.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	})
	return errors.Join(errs...)
}
