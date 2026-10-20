package kobo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"el-book/sdk/device"
)

func TestStructSizes(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"fb_fix_screeninfo", unsafe.Sizeof(fixScreenInfo{}), 68},
		{"fb_var_screeninfo", unsafe.Sizeof(varScreenInfo{}), 160},
		{"mxcfb_update_data v2", unsafe.Sizeof(mxcfbUpdateDataV2{}), sizeofUpdateDataV2},
		{"mxcfb_update_marker_data", unsafe.Sizeof(mxcfbUpdateMarkerData{}), sizeofUpdateMarker},
		{"input_absinfo", unsafe.Sizeof(inputAbsInfo{}), sizeofInputAbsInfo},
		{"input_event (arm)", unsafe.Sizeof(rawEvent{}), rawEventSize},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: %d байт, ядро ждёт %d", c.name, c.got, c.want)
		}
	}
}

func TestIoctlNumbers(t *testing.T) {
	cases := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"MXCFB_SEND_UPDATE_V2", mxcfbSendUpdateV2, 0x4048462E},
		{"MXCFB_WAIT_FOR_UPDATE_COMPLETE_V3", mxcfbWaitUpdateV3, 0xC008462F},
		{"EVIOCGRAB", evIOCGrab, 0x40044590},
		{"EVIOCGNAME(256)", evIOCGName, 0x81004506},
		{"EVIOCGABS(ABS_MT_POSITION_X)", evIOCGAbsBase + absMTPositionX, 0x80184575},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %#x, нужно %#x", c.name, c.got, c.want)
		}
	}
}

func TestBuildUpdate(t *testing.T) {
	r := mxcfbRect{Top: 10, Left: 20, Width: 30, Height: 40}
	cases := []struct {
		u        device.Update
		waveform uint32
		mode     uint32
		flags    uint32
	}{
		{device.Update{Waveform: device.WaveformDU}, ntxWaveformDU, updateModePartial, 0},
		{device.Update{Waveform: device.WaveformA2}, ntxWaveformA2, updateModePartial, flagForceMonochrome},
		{device.Update{Waveform: device.WaveformGL16}, ntxWaveformGL16, updateModePartial, 0},
		{device.Update{Waveform: device.WaveformGC16, Full: true}, ntxWaveformGC16, updateModeFull, 0},
		{device.Update{Waveform: device.WaveformREAGL, Full: true}, ntxWaveformGLR16, updateModePartial, 0},
		{device.Update{Waveform: device.WaveformAuto}, waveformModeAuto, updateModePartial, 0},
	}
	for _, c := range cases {
		got := buildUpdate(r, c.u, 7)
		if got.WaveformMode != c.waveform || got.UpdateMode != c.mode || got.Flags != c.flags {
			t.Errorf("%v full=%v: waveform %d mode %d flags %#x, нужно %d %d %#x",
				c.u.Waveform, c.u.Full, got.WaveformMode, got.UpdateMode, got.Flags, c.waveform, c.mode, c.flags)
		}
		if got.UpdateRegion != r || got.UpdateMarker != 7 || got.Temp != tempUseAmbient || got.DitherMode != ditherPassthrough {
			t.Errorf("%v: поля запроса %+v", c.u.Waveform, got)
		}
	}
}

func TestDetect(t *testing.T) {
	if m, err := Detect("frost", ""); err != nil || m.Name != "Kobo Forma" {
		t.Fatalf("frost: %v %v", m, err)
	}
	if m, err := Detect("", "380"); err != nil || m.Codename != "frost" {
		t.Fatalf("380: %v %v", m, err)
	}
	if _, err := Detect("nova", "376"); err == nil {
		t.Fatal("Clara HD не поддерживается, а Detect её нашёл")
	}
}

func TestParseVersion(t *testing.T) {
	n, fw := parseVersion("N782000000000,4.1.15,4.38.23171,4.1.15,4.1.15,00000000-0000-0000-0000-000000000377\n")
	if n != "377" || fw != "4.38.23171" {
		t.Fatalf("номер %q, прошивка %q", n, fw)
	}
	if n, fw := parseVersion(""); n != "" || fw != "" {
		t.Fatalf("пустой файл: %q %q", n, fw)
	}
}

func TestFormaRotation(t *testing.T) {
	m := &forma
	// Прочитанные значения — канонические повороты FBInk: UR→3, CCW→0, UD→1, CW→2;
	// записывать нечётные нужно с инверсией.
	write := map[device.Rotation]uint32{
		device.Portrait: 1, device.LandscapeCW: 0, device.PortraitFlipped: 3, device.LandscapeCCW: 2,
	}
	for r, w := range write {
		if got := m.WriteRotation(r); got != w {
			t.Errorf("%v: пишем %d, нужно %d", r, got, w)
		}
		back, ok := m.CanonicalFromRead(m.RotationRead[r])
		if !ok || back != r {
			t.Errorf("%v: обратное преобразование дало %v", r, back)
		}
	}
}

func TestTouchToPortrait(t *testing.T) {
	m := &forma
	cases := []struct{ rx, ry, x, y int }{
		{0, 0, 1439, 0},       // начало сырых осей — правый верхний угол портрета
		{1919, 1439, 0, 1919}, // противоположный угол
		{100, 200, 1239, 100}, // x = 1439 − rawY, y = rawX
	}
	for _, c := range cases {
		if x, y := m.touchToPortrait(c.rx, c.ry, 0, 0); x != c.x || y != c.y {
			t.Errorf("сырые (%d,%d) → (%d,%d), нужно (%d,%d)", c.rx, c.ry, x, y, c.x, c.y)
		}
	}
	// Диапазоны осей из EVIOCGABS масштабируют координаты.
	if x, y := m.touchToPortrait(960, 720, 1920, 1440); x != 719 || y != 960 {
		t.Errorf("середина с масштабом → (%d,%d)", x, y)
	}
}

func TestPortraitToLogical(t *testing.T) {
	m := &forma
	// Точка у левого верхнего угла портрета, (10, 20).
	cases := []struct {
		r    device.Rotation
		x, y int
	}{
		{device.Portrait, 10, 20},
		{device.LandscapeCW, 20, 1429},       // содержимое по часовой: левый верх портрета — левый низ
		{device.PortraitFlipped, 1429, 1899}, // перевёрнуто
		{device.LandscapeCCW, 1899, 10},      // против часовой: левый верх портрета — правый верх
	}
	for _, c := range cases {
		if x, y := m.portraitToLogical(10, 20, c.r); x != c.x || y != c.y {
			t.Errorf("%v: (%d,%d), нужно (%d,%d)", c.r, x, y, c.x, c.y)
		}
	}
}

func ev(typ, code uint16, value int32) rawEvent {
	return rawEvent{Sec: 1, Type: typ, Code: code, Value: value}
}

func syn() rawEvent { return ev(evSyn, synReport, 0) }

type touchLog []device.TouchEvent

func runTouch(snow bool, events ...rawEvent) (touchLog, *touchParser) {
	var out touchLog
	p := newTouchParser(snow, func(x, y int) (int, int) { return x, y }, func(e device.Event) {
		out = append(out, e.(device.TouchEvent))
	})
	for _, e := range events {
		p.feed(e)
	}
	return out, p
}

func (l touchLog) phases() []string {
	var s []string
	for _, e := range l {
		s = append(s, e.Phase.String())
	}
	return s
}

func TestTouchSnowProtocol(t *testing.T) {
	// Forma: TRACKING_ID вместо слота, кадр движения без него, подъём только BTN_TOUCH 0.
	got, p := runTouch(true,
		ev(evAbs, absMTTrackingID, 0), ev(evAbs, absMTPositionX, 100), ev(evAbs, absMTPositionY, 200), syn(),
		ev(evAbs, absMTPositionX, 110), syn(),
		ev(evKey, btnTouch, 0), syn(),
	)
	want := []string{"down", "move", "up"}
	if len(got) != 3 || got.phases()[0] != want[0] || got.phases()[1] != want[1] || got.phases()[2] != want[2] {
		t.Fatalf("фазы %v, нужно %v", got.phases(), want)
	}
	if got[1].X != 110 || got[1].Y != 200 || got[2].X != 110 {
		t.Fatalf("координаты %+v", got)
	}
	if got[0].Slot != 0 || !p.snow {
		t.Fatalf("слот %d, snow %v", got[0].Slot, p.snow)
	}
	// Второе касание после подъёма — снова слот 0.
	again, _ := runTouch(true,
		ev(evAbs, absMTTrackingID, 0), ev(evAbs, absMTPositionX, 1), ev(evAbs, absMTPositionY, 2), syn(),
		ev(evKey, btnTouch, 0), syn(),
		ev(evAbs, absMTTrackingID, 0), ev(evAbs, absMTPositionX, 5), ev(evAbs, absMTPositionY, 6), syn(),
	)
	if len(again) != 3 || again[2].Phase != device.TouchDown || again[2].Slot != 0 || again[2].X != 5 {
		t.Fatalf("повторное касание: %+v", again)
	}
}

func TestTouchSnowTwoFingers(t *testing.T) {
	got, _ := runTouch(true,
		ev(evAbs, absMTTrackingID, 0), ev(evAbs, absMTPositionX, 10), ev(evAbs, absMTPositionY, 10),
		ev(evAbs, absMTTrackingID, 1), ev(evAbs, absMTPositionX, 50), ev(evAbs, absMTPositionY, 50), syn(),
		ev(evKey, btnTouch, 0), syn(),
	)
	downs, ups := map[int]bool{}, map[int]bool{}
	for _, e := range got {
		switch e.Phase {
		case device.TouchDown:
			downs[e.Slot] = true
		case device.TouchUp:
			ups[e.Slot] = true
		}
	}
	if len(downs) != 2 || len(ups) != 2 || !downs[0] || !downs[1] {
		t.Fatalf("два пальца: %+v", got)
	}
}

func TestTouchQuickTapInOneFrame(t *testing.T) {
	got, _ := runTouch(true,
		ev(evAbs, absMTTrackingID, 3), ev(evAbs, absMTPositionX, 7), ev(evAbs, absMTPositionY, 8),
		ev(evKey, btnTouch, 0), syn(),
	)
	if len(got) != 2 || got[0].Phase != device.TouchDown || got[1].Phase != device.TouchUp || got[0].Slot != 0 {
		t.Fatalf("касание в одном кадре: %+v", got)
	}
}

func TestTouchStandardProtocolDisablesSnow(t *testing.T) {
	// Новая ревизия панели шлёт обычный протокол B: TRACKING_ID −1 при подъёме.
	got, p := runTouch(true,
		ev(evAbs, absMTTrackingID, 41), ev(evAbs, absMTPositionX, 10), ev(evAbs, absMTPositionY, 20), syn(),
		ev(evAbs, absMTTrackingID, -1), syn(),
		ev(evAbs, absMTSlot, 0), ev(evAbs, absMTTrackingID, 42), ev(evAbs, absMTPositionX, 30), ev(evAbs, absMTPositionY, 40), syn(),
		ev(evAbs, absMTSlot, 1), ev(evAbs, absMTTrackingID, 43), ev(evAbs, absMTPositionX, 300), ev(evAbs, absMTPositionY, 400), syn(),
		ev(evAbs, absMTSlot, 0), ev(evAbs, absMTTrackingID, -1), syn(),
		ev(evAbs, absMTSlot, 1), ev(evAbs, absMTPositionX, 310), syn(),
		ev(evAbs, absMTTrackingID, -1), syn(),
	)
	if p.snow {
		t.Fatal("квирк snow не выключился после TRACKING_ID −1")
	}
	want := []struct {
		phase device.TouchPhase
		slot  int
		x     int
	}{
		{device.TouchDown, 0, 10}, {device.TouchUp, 0, 10},
		{device.TouchDown, 0, 30},
		{device.TouchDown, 1, 300},
		{device.TouchUp, 0, 30},
		{device.TouchMove, 1, 310},
		{device.TouchUp, 1, 310},
	}
	if len(got) != len(want) {
		t.Fatalf("события %+v", got)
	}
	for i, w := range want {
		if got[i].Phase != w.phase || got[i].Slot != w.slot || got[i].X != w.x {
			t.Errorf("событие %d: %+v, нужно %+v", i, got[i], w)
		}
	}
}

func TestDecodeEvents(t *testing.T) {
	b := make([]byte, 2*rawEventSize+3) // хвост неполного события отбрасывается
	binary.LittleEndian.PutUint32(b[0:], 100)
	binary.LittleEndian.PutUint32(b[4:], 250000)
	binary.LittleEndian.PutUint16(b[8:], evKey)
	binary.LittleEndian.PutUint16(b[10:], 116)
	binary.LittleEndian.PutUint32(b[12:], 1)
	binary.LittleEndian.PutUint16(b[16+8:], evAbs)
	binary.LittleEndian.PutUint16(b[16+10:], absMTTrackingID)
	binary.LittleEndian.PutUint32(b[16+12:], 0xFFFFFFFF)
	got := decodeEvents(b)
	if len(got) != 2 {
		t.Fatalf("событий %d", len(got))
	}
	if got[0].Type != evKey || got[0].Code != 116 || got[0].Value != 1 || !got[0].time().Equal(time.Unix(100, 250000000)) {
		t.Errorf("первое событие %+v", got[0])
	}
	if got[1].Value != -1 {
		t.Errorf("TRACKING_ID −1 прочитан как %d", got[1].Value)
	}
}

func TestKeysAndOrientation(t *testing.T) {
	e, ok := keyEvent(forma.Keys, ev(evKey, 116, 1))
	if k := e.(device.KeyEvent); !ok || k.Key != device.KeyPower || !k.Pressed {
		t.Fatalf("кнопка питания: %+v", e)
	}
	if _, ok := keyEvent(forma.Keys, ev(evKey, 194, 2)); ok {
		t.Fatal("автоповтор не должен давать событие")
	}
	if e, ok := keyEvent(forma.Keys, ev(evKey, 59, 0)); !ok || e.(device.KeyEvent).Key != device.KeySleepCover || e.(device.KeyEvent).Pressed {
		t.Fatalf("обложка открыта: %+v", e)
	}
	o, ok := orientationEvent(ev(evMsc, mscRaw, gsensorLandscapeRight))
	if !ok || o.(device.OrientationEvent).Rotation != device.LandscapeCCW {
		t.Fatalf("книга по часовой: %+v", o)
	}
	if _, ok := orientationEvent(ev(evMsc, mscRaw, 0x1b)); ok {
		t.Fatal("«экраном вниз» не меняет ориентацию")
	}
}

func TestHardwareStatus(t *testing.T) {
	e, ok := parseHardwareStatus("usb plug add")
	if p := e.(device.PowerSourceEvent); !ok || p.Source != device.SourceComputer || !p.Connected {
		t.Fatalf("usb plug add: %+v", e)
	}
	e, ok = parseHardwareStatus("usb ac remove")
	if p := e.(device.PowerSourceEvent); !ok || p.Source != device.SourceCharger || p.Connected {
		t.Fatalf("usb ac remove: %+v", e)
	}
	if _, ok := parseHardwareStatus("network bound eth0"); ok {
		t.Fatal("сеть не источник питания")
	}
}

func writeFile(t *testing.T, root, p, content string) {
	t.Helper()
	full := filepath.Join(root, p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, p))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLight(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, forma.BrightnessPath, "")
	writeFile(t, root, filepath.Join(filepath.Dir(forma.BrightnessPath), "actual_brightness"), "37\n")
	writeFile(t, root, forma.WarmthPath, "")
	l := newLight(sysfs{root: root}, &forma)
	if l.Brightness() != 37 {
		t.Fatalf("яркость при старте %d", l.Brightness())
	}
	if err := l.SetBrightness(140); err != nil || readFile(t, root, forma.BrightnessPath) != "100" {
		t.Fatalf("яркость выше 100: %v %q", err, readFile(t, root, forma.BrightnessPath))
	}
	for _, c := range []struct {
		percent int
		want    string
	}{{100, "0"}, {0, "10"}, {35, "6"}} {
		writeFile(t, root, forma.WarmthPath, "")
		if err := l.SetWarmth(c.percent); err != nil {
			t.Fatal(err)
		}
		if got := readFile(t, root, forma.WarmthPath); got != c.want {
			t.Errorf("теплота %d%% → %q, нужно %q (шкала 0…10 инвертирована)", c.percent, got, c.want)
		}
	}
	if l.Warmth() != 35 || !l.HasWarmth() {
		t.Fatalf("теплота %d", l.Warmth())
	}
}

func TestBattery(t *testing.T) {
	root := t.TempDir()
	dir := "/sys/class/power_supply/mc13892_bat"
	writeFile(t, root, dir+"/capacity", "73\n")
	writeFile(t, root, dir+"/status", "Discharging\n")
	b := findBattery(sysfs{root: root})
	if lvl, err := b.Level(); err != nil || lvl != 73 {
		t.Fatalf("заряд %d %v", lvl, err)
	}
	if ch, err := b.Charging(); err != nil || ch {
		t.Fatalf("Discharging: заряжается=%v %v", ch, err)
	}
	writeFile(t, root, dir+"/status", "Full\n")
	if ch, _ := b.Charging(); !ch {
		t.Fatal("полная батарея на кабеле считается заряжающейся, как у KOReader")
	}
}

// fakeKernel — файлы сна: запоминает записи и отказывает очередным записям mem.
type fakeKernel struct {
	writes []string // «файл=значение» по порядку
	refuse []error  // ответы ядра на очередные записи mem; кончились — успех
	mems   int
	onMem  func() // вызывается при записи mem, до ответа
}

func (k *fakeKernel) write(p, v string) error {
	k.writes = append(k.writes, filepath.Base(p)+"="+v)
	if p != statePath {
		return nil
	}
	k.mems++
	if k.onMem != nil {
		k.onMem()
	}
	if len(k.refuse) == 0 {
		return nil
	}
	err := k.refuse[0]
	k.refuse = k.refuse[1:]
	return err
}

// kernelRefusal — отказ ядра в том виде, в каком его возвращает sysfs.write.
func kernelRefusal(e syscall.Errno) error {
	return fmt.Errorf("%s: %w", statePath, &os.PathError{Op: "write", Path: statePath, Err: e})
}

// testPower — сон без пауз: экран обновлялся давно, ждать панель не нужно.
func testPower(k *fakeKernel, wake chan struct{}) power {
	long := time.Now().Add(-time.Hour)
	return power{
		fs: k, sync: func() {}, now: time.Now, shown: func() time.Time { return long },
		wake: wake, log: slog.New(slog.DiscardHandler),
		t: powerTiming{retry: time.Millisecond, attempts: 3, wakeWait: 20 * time.Millisecond},
	}
}

func TestSuspendSequence(t *testing.T) {
	k := &fakeKernel{}
	syncs := 0
	p := testPower(k, nil)
	p.sync = func() { syncs++ }
	if err := p.Suspend(); err != nil {
		t.Fatal(err)
	}
	want := "state-extended=1 state=mem state-extended=0"
	if got := strings.Join(k.writes, " "); got != want || syncs != 1 {
		t.Fatalf("записи %q, sync %d; нужно %q и один sync", got, syncs, want)
	}
}

func TestSuspendWaitsForPanel(t *testing.T) {
	k := &fakeKernel{}
	p := testPower(k, nil)
	start := time.Now()
	p.shown = func() time.Time { return start } // экран только что обновился
	p.t.settle, p.t.lead = 80*time.Millisecond, 20*time.Millisecond
	var memAt time.Duration
	k.onMem = func() { memAt = time.Since(start) }
	if err := p.Suspend(); err != nil {
		t.Fatal(err)
	}
	if memAt < p.t.settle {
		t.Fatalf("mem записан через %v после обновления экрана, нужно не раньше %v", memAt, p.t.settle)
	}
}

func TestSuspendRetriesKernelRefusal(t *testing.T) {
	k := &fakeKernel{refuse: []error{kernelRefusal(syscall.ENOENT), kernelRefusal(syscall.EPERM)}}
	if err := testPower(k, nil).Suspend(); err != nil {
		t.Fatal(err)
	}
	want := "state-extended=1 state=mem state-extended=0 state-extended=1 state=mem state-extended=0 state-extended=1 state=mem state-extended=0"
	if got := strings.Join(k.writes, " "); got != want {
		t.Fatalf("записи %q, нужно %q", got, want)
	}
}

func TestSuspendGivesUp(t *testing.T) {
	busy := kernelRefusal(syscall.EBUSY)
	k := &fakeKernel{refuse: []error{busy, busy, busy, busy}}
	err := testPower(k, nil).Suspend()
	if !errors.Is(err, syscall.EBUSY) || errors.Is(err, device.ErrSuspendCancelled) || k.mems != 3 {
		t.Fatalf("ошибка %v после %d попыток: нужен отказ EBUSY после трёх", err, k.mems)
	}
	k = &fakeKernel{refuse: []error{kernelRefusal(syscall.EINVAL)}}
	if err := testPower(k, nil).Suspend(); !errors.Is(err, syscall.EINVAL) || k.mems != 1 {
		t.Fatalf("ошибка %v после %d попыток: EINVAL не повторяется", err, k.mems)
	}
}

func TestSuspendCancelledByWake(t *testing.T) {
	// Разбудили, пока книга ждёт панель: ядро не трогается вовсе.
	k := &fakeKernel{}
	wake := make(chan struct{}, 1)
	p := testPower(k, wake)
	p.shown = time.Now
	p.t.settle = time.Minute
	go func() {
		time.Sleep(20 * time.Millisecond)
		wake <- struct{}{}
	}()
	if err := p.Suspend(); !errors.Is(err, device.ErrSuspendCancelled) || len(k.writes) != 0 {
		t.Fatalf("ошибка %v, записи %v: нужна отмена без записей", err, k.writes)
	}

	// Разбудили за две секунды до mem: периферия просыпается, mem не пишется.
	k = &fakeKernel{}
	p = testPower(k, wake)
	p.t.lead = time.Minute
	go func() {
		time.Sleep(20 * time.Millisecond)
		wake <- struct{}{}
	}()
	err := p.Suspend()
	if got := strings.Join(k.writes, " "); !errors.Is(err, device.ErrSuspendCancelled) || got != "state-extended=1 state-extended=0" {
		t.Fatalf("ошибка %v, записи %q", err, got)
	}
}

func TestSuspendSpuriousWake(t *testing.T) {
	k := &fakeKernel{}
	wake := make(chan struct{}, 1)
	wake <- struct{}{} // нажатие, которым усыпили, не отменяет сон
	p := testPower(k, wake)
	k.onMem = func() {
		if k.mems == 2 { // во втором сне пользователь нажимает кнопку
			wake <- struct{}{}
		}
	}
	if err := p.Suspend(); err != nil {
		t.Fatal(err)
	}
	if k.mems != 2 {
		t.Fatalf("засыпаний %d: после ложного пробуждения книга должна уснуть снова", k.mems)
	}
	// Ввод сломан: события не приходят никогда — Suspend всё равно возвращается.
	k = &fakeKernel{}
	p = testPower(k, wake)
	p.t.wakeWait = time.Millisecond
	if err := p.Suspend(); err != nil || k.mems != maxSpurious+1 {
		t.Fatalf("ошибка %v, засыпаний %d", err, k.mems)
	}
}

func TestKernelBusy(t *testing.T) {
	for e, busy := range map[syscall.Errno]bool{
		syscall.EPERM: true, syscall.ENOENT: true, syscall.ESRCH: true, syscall.EBUSY: true,
		syscall.EAGAIN: true, syscall.EINVAL: false, syscall.EIO: false,
	} {
		if kernelBusy(kernelRefusal(e)) != busy {
			t.Errorf("%v: повторять %v", e, busy)
		}
	}
}

func TestWakeEvents(t *testing.T) {
	cases := []struct {
		e    device.Event
		wake bool
	}{
		{device.KeyEvent{Key: device.KeyPower, Pressed: true}, true},
		{device.KeyEvent{Key: device.KeyPower, Pressed: false}, false},
		{device.KeyEvent{Key: device.KeySleepCover, Pressed: false}, true},
		{device.KeyEvent{Key: device.KeySleepCover, Pressed: true}, false},
		{device.PowerSourceEvent{Source: device.SourceComputer, Connected: true}, true},
		{device.PowerSourceEvent{Source: device.SourceCharger, Connected: true}, false},
		{device.TouchEvent{}, false},
	}
	for _, c := range cases {
		if isWakeEvent(c.e) != c.wake {
			t.Errorf("%+v: пробуждение %v", c.e, !c.wake)
		}
	}
}

// fakeFB ведёт себя как ядро Forma: нечётный rotate при записи инвертируется (quirk),
// при нечётном прочитанном значении экран портретный.
type fakeFB struct {
	quirk   bool
	v       varScreenInfo
	mem     []byte
	maps    int
	sent    []mxcfbUpdateDataV2
	waitErr error
}

func newFakeFB(quirk bool, read uint32) *fakeFB {
	f := &fakeFB{quirk: quirk}
	f.v.BitsPerPixel = 32
	f.setRotate(read)
	return f
}

func (f *fakeFB) setRotate(read uint32) {
	f.v.Rotate = read
	if read%2 == 1 {
		f.v.XRes, f.v.YRes = 1440, 1920
	} else {
		f.v.XRes, f.v.YRes = 1920, 1440
	}
	f.v.XResVirtual, f.v.YResVirtual = f.v.XRes, f.v.YRes
}

func (f *fakeFB) VarInfo() (varScreenInfo, error) { return f.v, nil }

func (f *fakeFB) PutVarInfo(v *varScreenInfo) error {
	r := v.Rotate & 3
	if f.quirk && r%2 == 1 {
		r ^= 2
	}
	f.setRotate(r)
	*v = f.v
	return nil
}

func (f *fakeFB) FixInfo() (fixScreenInfo, error) {
	return fixScreenInfo{LineLength: f.v.XResVirtual * f.v.BitsPerPixel / 8, SmemLen: 1440 * 1920 * 4}, nil
}

func (f *fakeFB) Map(size int) ([]byte, error) {
	f.maps++
	f.mem = make([]byte, size)
	return f.mem, nil
}

func (f *fakeFB) SendUpdate(u *mxcfbUpdateDataV2) error {
	f.sent = append(f.sent, *u)
	return nil
}

func (f *fakeFB) WaitUpdate(*mxcfbUpdateMarkerData) error { return f.waitErr }

func (f *fakeFB) Close() error { return nil }

func TestDisplayRotation(t *testing.T) {
	fb := newFakeFB(true, 3)
	d, err := newDisplay(fb, &forma)
	if err != nil {
		t.Fatal(err)
	}
	if d.Rotation() != device.Portrait || d.Bounds() != image.Rect(0, 0, 1440, 1920) {
		t.Fatalf("при старте %v %v", d.Rotation(), d.Bounds())
	}
	for _, r := range []device.Rotation{device.LandscapeCW, device.PortraitFlipped, device.LandscapeCCW, device.Portrait} {
		if err := d.SetRotation(r); err != nil {
			t.Fatalf("%v: %v", r, err)
		}
		if fb.v.Rotate != forma.RotationRead[r] || d.Rotation() != r {
			t.Fatalf("%v: ядро держит %d", r, fb.v.Rotate)
		}
		if r.Landscape() != (d.Bounds().Dx() > d.Bounds().Dy()) {
			t.Fatalf("%v: размер %v", r, d.Bounds())
		}
	}
	if fb.maps != 5 {
		t.Errorf("отображений памяти %d: после каждого поворота нужно новое", fb.maps)
	}
}

func TestDisplayCloseRestoresRotation(t *testing.T) {
	fb := newFakeFB(true, 0) // Nickel оставил экран в альбомной ориентации
	d, err := newDisplay(fb, &forma)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetRotation(device.Portrait); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if fb.v.Rotate != 0 {
		t.Fatalf("после закрытия ядро держит %d, Nickel оставлял 0", fb.v.Rotate)
	}
}

func TestDisplayRotationRecovery(t *testing.T) {
	// Ядро без квирка: поправка Forma даёт не то значение, и его надо подобрать перебором.
	fb := newFakeFB(false, 0)
	d, err := newDisplay(fb, &forma)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetRotation(device.Portrait); err != nil {
		t.Fatal(err)
	}
	if fb.v.Rotate != 3 || d.Rotation() != device.Portrait {
		t.Fatalf("перебор не нашёл значение: ядро держит %d", fb.v.Rotate)
	}
}

func TestDisplayPresent(t *testing.T) {
	fb := newFakeFB(true, 3)
	fb.v.YResVirtual, fb.v.YOffset = 2*1920, 1920 // двойной буфер, видна вторая страница
	d, err := newDisplay(fb, &forma)
	if err != nil {
		t.Fatal(err)
	}
	src := image.NewGray(d.Bounds())
	src.Pix[7*src.Stride+5] = 0x80
	m, err := d.Present(src, device.Update{Rect: image.Rect(5, 7, 6, 8), Waveform: device.WaveformDU})
	if err != nil || m != 1 {
		t.Fatalf("маркер %d, %v", m, err)
	}
	line := 1440 * 4
	o := (7+1920)*line + 5*4
	if got := fb.mem[o : o+4]; got[0] != 0x80 || got[1] != 0x80 || got[2] != 0x80 || got[3] != 0xFF {
		t.Fatalf("пиксель BGRA %x", got)
	}
	u := fb.sent[0]
	if u.UpdateRegion != (mxcfbRect{Top: 7, Left: 5, Width: 2, Height: 2}) || u.WaveformMode != ntxWaveformDU || u.UpdateMarker != 1 {
		t.Fatalf("запрос %+v: область 1×1 должна стать 2×2", u)
	}
	if _, err := d.Present(image.NewGray(image.Rect(0, 0, 10, 10)), device.Update{}); err == nil {
		t.Fatal("кадр не того размера принят")
	}
	if m, err := d.Present(src, device.Update{Rect: image.Rect(2000, 2000, 2100, 2100)}); err != nil || m != 0 {
		t.Fatalf("область за экраном: %d %v", m, err)
	}
	fb.waitErr = syscall.EINVAL
	if err := d.Wait(1); err != nil {
		t.Fatalf("EINVAL — обновление уже закончилось: %v", err)
	}
	fb.waitErr = syscall.EIO
	if err := d.Wait(1); err == nil {
		t.Fatal("ошибка ожидания потеряна")
	}
}

func TestMinUpdateRect(t *testing.T) {
	b := image.Rect(0, 0, 100, 50)
	if r := minUpdateRect(image.Rect(99, 49, 100, 50), b); r != image.Rect(98, 48, 100, 50) {
		t.Fatalf("угол: %v", r)
	}
	if r := minUpdateRect(image.Rect(10, 10, 30, 20), b); r != image.Rect(10, 10, 30, 20) {
		t.Fatalf("обычная область изменилась: %v", r)
	}
}

func TestBlitFormats(t *testing.T) {
	dst := make([]byte, 8)
	blit16(dst, []byte{0xFF, 0x00, 0x80, 0x10})
	if dst[0] != 0xFF || dst[1] != 0xFF || dst[2] != 0 || dst[3] != 0 {
		t.Fatalf("RGB565 белый/чёрный: %x", dst)
	}
	var lut [256]uint32
	for g := range lut {
		v := uint32(g)
		lut[g] = 0xFF000000 | v<<16 | v<<8 | v
	}
	odd := make([]byte, 9)
	blit32(odd[1:], []byte{0x11, 0x22}, &lut) // невыровненный адрес
	if odd[1] != 0x11 || odd[4] != 0xFF || odd[5] != 0x22 || odd[8] != 0xFF {
		t.Fatalf("BGRA по невыровненному адресу: %x", odd)
	}
}
