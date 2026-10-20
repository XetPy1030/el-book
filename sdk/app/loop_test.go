package app

import (
	"image"
	"io"
	"log/slog"
	"testing"
	"time"

	"el-book/sdk/device"
	"el-book/sdk/device/emu"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// testScreen считает события жизненного цикла и рисует заливку fill.
type testScreen struct {
	fill   uint8
	events map[Lifecycle]int
}

func newScreen(fill uint8) *testScreen { return &testScreen{fill: fill, events: map[Lifecycle]int{}} }

func (s *testScreen) Layout(*Context, image.Rectangle) {}
func (s *testScreen) Draw(c *gfx.Canvas, _ *ui.Theme)  { c.Fill(c.Bounds(), s.fill) }
func (s *testScreen) Handle(_ *Context, ev Event) {
	if l, ok := ev.(Lifecycle); ok {
		s.events[l]++
	}
}

// testApp — приложение с одним экраном; считает запуски и остановки.
type testApp struct {
	id      string
	screen  *testScreen
	ctx     *Context
	starts  int
	stopped int
}

func (a *testApp) Manifest() Manifest { return Manifest{ID: a.id, Title: a.id, Icon: icons.Plus} }
func (a *testApp) Start(ctx *Context) Screen {
	a.starts++
	a.ctx = ctx
	a.screen = newScreen(gfx.Gray16(8))
	return a.screen
}
func (a *testApp) Stop(*Context) { a.stopped++ }

// testSystem — оболочка из одного домашнего экрана.
type testSystem struct {
	home *testScreen
	ctx  *Context
}

func (s *testSystem) Home(ctx *Context) Screen {
	s.ctx = ctx
	s.home = newScreen(gfx.White)
	return s.home
}
func (s *testSystem) Intercept(*Context, device.Event) bool   { return false }
func (s *testSystem) Crashed(*Context, Manifest, any, []byte) {}

func newTestLoop(t *testing.T, maxApps int) (*Loop, *testSystem, *emu.Emulator) {
	t.Helper()
	dev := emu.New(emu.Options{AutoWake: true})
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := gfx.DefaultFonts()
	if err != nil {
		t.Fatal(err)
	}
	sys := &testSystem{}
	l := newLoop(Config{Device: dev, System: sys, Store: store, Fonts: fonts, MaxApps: maxApps,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	l.push(nil, sys.Home(l.sysCtx))
	l.frame()
	return l, sys, dev
}

func TestBackgroundAppKeepsState(t *testing.T) {
	l, sys, _ := newTestLoop(t, 0)
	a := &testApp{id: "a"}
	l.launch(a)
	if l.active == nil || a.screen.events[Shown] != 1 || sys.home.events[Hidden] != 1 {
		t.Fatalf("запуск: active=%v, события %v, дом %v", l.active, a.screen.events, sys.home.events)
	}
	l.goHome()
	if l.active != nil || a.screen.events[Hidden] != 1 || a.stopped != 0 {
		t.Fatalf("домой: приложение должно уйти в фон, а не закрыться: %v, stop %d", a.screen.events, a.stopped)
	}
	if got := sys.ctx.Sys().Recent(); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("недавние: %v", got)
	}
	first := a.screen
	l.launch(a)
	if a.starts != 1 || a.screen != first || first.events[Shown] != 2 {
		t.Fatalf("повторный запуск должен вернуть тот же экран: запусков %d, события %v", a.starts, first.events)
	}
}

func TestPopLastScreenClosesApp(t *testing.T) {
	l, sys, _ := newTestLoop(t, 0)
	a := &testApp{id: "a"}
	l.launch(a)
	a.ctx.Push(newScreen(gfx.Black))
	a.ctx.Pop()
	if a.stopped != 0 || l.active == nil {
		t.Fatal("закрытие второго экрана не должно закрывать приложение")
	}
	a.ctx.Pop()
	if a.stopped != 1 || l.active != nil || len(sys.ctx.Sys().Recent()) != 0 {
		t.Fatalf("последний экран закрыт — приложение остановлено: stop %d, active %v", a.stopped, l.active)
	}
	if sys.home.events[Shown] != 2 {
		t.Fatalf("домашний экран должен показаться снова: %v", sys.home.events)
	}
}

func TestEvictsOldestApp(t *testing.T) {
	l, sys, _ := newTestLoop(t, 2)
	a, b, c := &testApp{id: "a"}, &testApp{id: "b"}, &testApp{id: "c"}
	for _, x := range []*testApp{a, b, c} {
		l.launch(x)
		l.goHome()
	}
	if a.stopped != 1 || b.stopped != 0 || c.stopped != 0 {
		t.Fatalf("закрыться должно самое давнее: a=%d b=%d c=%d", a.stopped, b.stopped, c.stopped)
	}
	recent := sys.ctx.Sys().Recent()
	if len(recent) != 2 || recent[0].ID != "c" || recent[1].ID != "b" {
		t.Fatalf("недавние: %v", recent)
	}
}

func TestBackgroundAppDoesNotPaint(t *testing.T) {
	l, _, _ := newTestLoop(t, 0)
	a := &testApp{id: "a"}
	l.launch(a)
	l.goHome()
	l.frame()
	a.ctx.Invalidate(image.Rect(0, 0, 10, 10), eink.UI)
	a.ctx.ShowOverlay(ui.NewToast("из фона"))
	if len(l.damage) != 0 {
		t.Fatalf("приложение в фоне не должно перерисовывать экран: %v", l.damage)
	}
	for _, ov := range l.overlays {
		if l.shown(ov) {
			t.Fatal("оверлей фонового приложения не должен быть виден")
		}
	}
	l.launch(a)
	shown := false
	for _, ov := range l.overlays {
		shown = shown || l.shown(ov)
	}
	if !shown {
		t.Fatal("оверлей должен вернуться вместе с приложением")
	}
}

func TestShellScreenSendsAppToBackground(t *testing.T) {
	l, sys, _ := newTestLoop(t, 0)
	a := &testApp{id: "a"}
	l.launch(a)
	settings := newScreen(gfx.Gray16(12))
	sys.ctx.Push(settings)
	if l.active != nil || a.screen.events[Hidden] != 1 || a.stopped != 0 {
		t.Fatal("экран оболочки должен увести приложение в фон, не закрывая")
	}
	sys.ctx.Pop()
	if top := l.top(); top == nil || top.screen != sys.home {
		t.Fatal("после закрытия настроек — домашний экран")
	}
}

func TestNightModeInvertsPanel(t *testing.T) {
	l, sys, dev := newTestLoop(t, 0)
	sys.ctx.Sys().SetNight(true)
	l.frame()
	if v := dev.Panel().GrayAt(5, 5).Y; v != 0 {
		t.Fatalf("в ночном режиме белый экран должен стать чёрным, пиксель %d", v)
	}
	sys.ctx.Sys().SetNight(false)
	l.frame()
	if v := dev.Panel().GrayAt(5, 5).Y; v != 255 {
		t.Fatalf("после выключения ночного режима пиксель %d", v)
	}
}

func TestToastReplacesPrevious(t *testing.T) {
	l, sys, _ := newTestLoop(t, 0)
	sys.ctx.Toast("Заряд 9 % — пора на зарядку")
	sys.ctx.Toast("Заряжается")
	var toasts []string
	for _, ov := range l.overlays {
		if tt, ok := ov.o.(*ui.Toast); ok {
			toasts = append(toasts, tt.Text)
		}
	}
	if len(toasts) != 1 || toasts[0] != "Заряжается" {
		t.Fatalf("на экране должно остаться одно, последнее сообщение: %q", toasts)
	}
}

func TestStrokesAreCoalesced(t *testing.T) {
	l, sys, dev := newTestLoop(t, 0)
	l.policy.StrokeInterval = time.Hour
	start := len(dev.History())
	strokes := func() (n int, last image.Rectangle) {
		for _, r := range dev.History()[start:] {
			if r.Waveform == device.WaveformA2.String() {
				n++
				last = r.Rect
			}
		}
		return n, last
	}
	for i := range 10 {
		sys.ctx.Invalidate(image.Rect(100+10*i, 100, 110+10*i, 110), eink.Stroke)
		l.frame()
	}
	if n, _ := strokes(); n != 1 {
		t.Fatalf("первый штрих уходит сразу, следующие ждут срока: обновлений A2 — %d", n)
	}

	sys.ctx.Invalidate(image.Rect(0, 1000, 100, 1100), eink.UI)
	l.frame()
	if n, last := strokes(); n != 2 || last != image.Rect(110, 100, 200, 110) {
		t.Fatalf("отложенные штрихи уходят одним обновлением вместе с другим изменением: %d, %v", n, last)
	}

	// Без других изменений отложенный штрих отправляет таймер.
	l.policy.StrokeInterval = 200 * time.Millisecond
	l.strokeSent = time.Now()
	sys.ctx.Invalidate(image.Rect(300, 300, 310, 310), eink.Stroke)
	l.frame()
	if n, _ := strokes(); n != 2 || l.strokeCancel == nil {
		t.Fatalf("штрих раньше срока должен ждать таймера: обновлений A2 — %d", n)
	}
	select {
	case f := <-l.posts:
		f()
	case <-time.After(5 * time.Second):
		t.Fatal("таймер не отправил отложенный штрих")
	}
	l.frame()
	if n, last := strokes(); n != 3 || last != image.Rect(300, 300, 310, 310) {
		t.Fatalf("по таймеру штрих должен уйти на панель: %d, %v", n, last)
	}
}

func TestA2GetsDitheredCopy(t *testing.T) {
	l, sys, dev := newTestLoop(t, 0)
	sys.home.fill = gfx.Gray16(8)
	sys.ctx.Invalidate(image.Rect(0, 0, 64, 64), eink.Stroke)
	l.frame()
	black, white := 0, 0
	for y := range 64 {
		for x := range 64 {
			switch dev.Panel().GrayAt(x, y).Y {
			case gfx.Black:
				black++
			case gfx.White:
				white++
			}
		}
	}
	if black == 0 || white == 0 || black+white != 64*64 {
		t.Fatalf("серая заливка в режиме A2 должна стать узором из чёрного и белого: чёрных %d, белых %d", black, white)
	}
}
