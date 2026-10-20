package app

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync/atomic"
	"time"

	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
	"el-book/sdk/web"
)

// Config — всё, что нужно циклу.
type Config struct {
	Device    device.Device
	System    System
	Store     *storage.Store
	Log       *slog.Logger
	Fonts     *gfx.Fonts
	Policy    *eink.Policy   // по умолчанию eink.NewPolicy()
	Durations eink.Durations // по умолчанию eink.DefaultDurations()
	Clock     func() time.Time
	// HTTP — транспорт сетевых запросов приложений; nil — настоящая сеть. UI-тесты
	// подставляют ответы из файлов.
	HTTP http.RoundTripper
	// UserAgent — подпись сетевых запросов: «el-book/<версия>».
	UserAgent string
	// MaxApps — сколько приложений держать запущенными; 0 — DefaultMaxApps.
	MaxApps int
}

// DefaultMaxApps — сколько приложений живут в фоне, прежде чем самое давнее закроется.
const DefaultMaxApps = 5

// running — запущенное приложение. Его экраны лежат в собственном стеке: уход домой
// приложение не закрывает, а повторный запуск возвращает его в том же состоянии.
type running struct {
	app     App
	man     Manifest
	ctx     *Context
	store   *storage.App
	bg      context.Context
	cancel  context.CancelFunc
	stopped bool
	stack   []frame
	web     *web.Client
}

type frame struct {
	screen Screen
	owner  *running // nil — экран оболочки
	laid   image.Rectangle
}

type overlay struct {
	o     ui.Overlay
	owner *running
}

// Loop — цикл событий. Создаётся Run; снаружи доступен через Context.
type Loop struct {
	cfg    Config
	dev    device.Device
	disp   device.Display
	net    device.Network // nil — у устройства нет Wi-Fi
	sys    System
	log    *slog.Logger
	theme  *ui.Theme
	policy *eink.Policy
	sched  *eink.Scheduler
	clock  func() time.Time

	buf      *image.Gray
	out      *image.Gray // ночной режим: инвертированный buf, его и видит панель
	mono     *image.Gray // двухцветная копия областей для режима A2
	night    bool
	shell    []frame    // экраны оболочки; в основании — домашний
	apps     []*running // запущенные приложения; последнее — недавнее
	active   *running   // приложение на экране; nil — экран оболочки
	overlays []overlay
	damage   []eink.Damage
	posts    chan func()
	recog    *ui.Recognizer
	gestures []ui.Gesture
	feeding  bool // идёт разбор касания: жесты копятся и доставляются после сырого события
	exitCode *int
	root     context.Context
	cancel   context.CancelFunc
	sysCtx   *Context
	shellWeb *web.Client
	lastUser time.Time
	awoke    time.Time    // касания раньше этого времени накопились, пока книга засыпала
	netUsed  atomic.Int64 // когда приложения последний раз ходили в сеть, Unix-наносекунды
	netOff   atomic.Bool  // оболочка запретила включать Wi-Fi

	// Штрихи копятся и уходят на панель не чаще раза в Policy.StrokeInterval.
	strokeLater  []image.Rectangle
	strokeSent   time.Time
	strokeCancel func()
}

// Run запускает цикл и возвращает код выхода процесса.
func Run(cfg Config) int {
	l := newLoop(cfg)
	return l.run()
}

func newLoop(cfg Config) *Loop {
	if cfg.Policy == nil {
		cfg.Policy = eink.NewPolicy()
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.Log == nil {
		cfg.Log = slog.Default()
	}
	if cfg.MaxApps <= 0 {
		cfg.MaxApps = DefaultMaxApps
	}
	l := &Loop{
		cfg:    cfg,
		dev:    cfg.Device,
		disp:   cfg.Device.Display(),
		sys:    cfg.System,
		log:    cfg.Log,
		theme:  ui.NewTheme(cfg.Device.Display().DPI(), cfg.Fonts),
		policy: cfg.Policy,
		clock:  cfg.Clock,
		posts:  make(chan func(), 1024),
	}
	if n, ok := cfg.Device.(device.Networker); ok {
		l.net = n.Network()
	}
	l.sched = eink.NewScheduler(l.disp, cfg.Durations)
	l.root, l.cancel = context.WithCancel(context.Background())
	l.sysCtx = &Context{l: l}
	l.recog = ui.NewRecognizer(l.theme, func(d time.Duration, f func()) func() {
		return l.sysCtx.After(d, f)
	}, l.onGesture)
	l.allocBuffer()
	l.lastUser = l.clock()
	return l
}

func (l *Loop) allocBuffer() {
	l.buf = image.NewGray(l.disp.Bounds())
	l.out = image.NewGray(l.disp.Bounds())
	l.mono = image.NewGray(l.disp.Bounds())
	gfx.NewCanvas(l.buf).Fill(l.buf.Bounds(), gfx.White)
}

func (l *Loop) run() int {
	var home Screen
	if !l.safe(nil, func() { home = l.sys.Home(l.sysCtx) }) || home == nil {
		return ExitCrash
	}
	l.push(nil, home)
	l.invalidateAll(eink.Clean)
	l.frame()
	events := l.dev.Events()
	for l.exitCode == nil {
		select {
		case ev, ok := <-events:
			if !ok {
				l.log.Error("поток событий устройства закрылся")
				return ExitCrash
			}
			l.handleDevice(ev)
		case f := <-l.posts:
			f()
		}
		l.drain(events)
		l.frame()
	}
	l.cancel()
	return *l.exitCode
}

// drain забирает накопившиеся события без ожидания: движения пальца склеиваются в один кадр.
func (l *Loop) drain(events <-chan device.Event) {
	for i := 0; i < 256 && l.exitCode == nil; i++ {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			l.handleDevice(ev)
		case f := <-l.posts:
			f()
		default:
			return
		}
	}
}

func (l *Loop) ctxFor(owner *running) *Context {
	if owner == nil {
		return l.sysCtx
	}
	return owner.ctx
}

func (l *Loop) handleDevice(ev device.Event) {
	if te, ok := ev.(device.TouchEvent); ok {
		if !te.Time.IsZero() && te.Time.Before(l.awoke) {
			return // палец лёг на экран сна: после пробуждения это не нажатие
		}
		l.lastUser = l.clock()
	}
	if ke, ok := ev.(device.KeyEvent); ok && ke.Pressed {
		l.lastUser = l.clock()
	}
	consumed := false
	l.safe(nil, func() { consumed = l.sys.Intercept(l.sysCtx, ev) })
	if consumed || l.exitCode != nil {
		return
	}
	l.dispatch(ev)
}

// dispatch отдаёт событие экранам: касание — через распознаватель жестов.
func (l *Loop) dispatch(ev device.Event) {
	if te, ok := ev.(device.TouchEvent); ok {
		l.gestures = l.gestures[:0]
		l.feeding = true
		l.recog.Feed(te)
		l.feeding = false
		gs := append([]ui.Gesture(nil), l.gestures...)
		l.deliver(te)
		for _, g := range gs {
			l.deliver(g)
		}
		return
	}
	l.deliver(ev)
}

// onGesture получает жесты распознавателя. Долгое нажатие приходит по таймеру, вне разбора
// касания, — такие жесты доставляются сразу.
func (l *Loop) onGesture(g ui.Gesture) {
	if l.feeding {
		l.gestures = append(l.gestures, g)
		return
	}
	l.deliver(g)
}

// shown — виден ли оверлей: свой у оболочки — всегда, у приложения — пока оно на экране.
func (l *Loop) shown(ov overlay) bool { return ov.owner == nil || ov.owner == l.active }

// deliver отдаёт событие верхнему модальному оверлею или верхнему экрану.
func (l *Loop) deliver(ev Event) {
	for i := len(l.overlays) - 1; i >= 0; i-- {
		ov := l.overlays[i]
		if !ov.o.Modal() || !l.shown(ov) {
			continue
		}
		ctx := l.ctxFor(ov.owner)
		l.safe(ov.owner, func() {
			switch e := ev.(type) {
			case device.TouchEvent:
				ov.o.Touch(ctx, e)
			case ui.Gesture:
				ov.o.Gesture(ctx, e)
			}
		})
		return
	}
	if f := l.top(); f != nil {
		owner, scr := f.owner, f.screen
		ctx := l.ctxFor(owner)
		l.safe(owner, func() { scr.Handle(ctx, ev) })
	}
}

// safe выполняет код владельца owner под перехватом паники. Паника приложения закрывает
// приложение; паника оболочки завершает процесс с ExitCrash.
func (l *Loop) safe(owner *running, f func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
			l.crash(owner, r, debug.Stack())
		}
	}()
	f()
	return true
}

func (l *Loop) crash(owner *running, r any, stack []byte) {
	if owner == nil {
		l.log.Error("паника оболочки", "err", fmt.Sprint(r), "stack", string(stack))
		code := ExitCrash
		l.exitCode = &code
		return
	}
	if owner.stopped {
		return
	}
	l.log.Error("паника приложения", "app", owner.man.ID, "err", fmt.Sprint(r), "stack", string(stack))
	l.removeApp(owner, false)
	l.safe(nil, func() { l.sys.Crashed(l.sysCtx, owner.man, r, stack) })
	l.invalidateAll(eink.UI)
}

func (l *Loop) invalidateAll(c eink.Class) {
	l.damage = append(l.damage, eink.Damage{Rect: l.disp.Bounds(), Class: c})
}

// frame перерисовывает повреждённые области и отправляет обновления.
func (l *Loop) frame() {
	if len(l.damage) == 0 && len(l.strokeLater) == 0 || l.exitCode != nil {
		return
	}
	dmg := l.gateStrokes(l.damage)
	l.damage = nil
	if len(dmg) == 0 {
		return
	}
	screen := l.disp.Bounds()
	var rects []image.Rectangle
	for _, d := range dmg {
		if d.Class == eink.Clean {
			rects = []image.Rectangle{screen}
			break
		}
		rects = append(rects, d.Rect.Intersect(screen))
	}
	for _, r := range eink.Merge(rects) {
		l.paint(r)
		if l.night {
			invert(l.out, l.buf, r)
		}
	}
	src := l.buf
	if l.night {
		src = l.out
	}
	for _, u := range l.policy.Plan(screen, dmg) {
		img := src
		if u.Waveform == device.WaveformA2 {
			// A2 показывает только чёрное и белое: оттенки уходят узором, а не порогом,
			// иначе светлая разметка и серые штрихи в области пропали бы до чистовой перерисовки.
			gfx.DitherInto(l.mono, src, u.Rect)
			img = l.mono
		}
		if err := l.sched.Present(img, u); err != nil {
			l.log.Error("обновление панели", "rect", u.Rect, "waveform", u.Waveform, "err", err)
		}
	}
}

// gateStrokes откладывает штрихи, если с прошлой отправки прошло меньше StrokeInterval:
// обновление на каждое движение пальца перегружает контроллер e-ink («working buf busy»,
// «no luts available» в журнале ядра), и след отстаёт.
// Остальные классы идут сразу, и вместе с ними уходят отложенные штрихи: обновления одной
// области не должны обгонять друг друга.
func (l *Loop) gateStrokes(dmg []eink.Damage) []eink.Damage {
	interval := l.policy.StrokeInterval
	if interval <= 0 {
		return dmg
	}
	out := make([]eink.Damage, 0, len(dmg))
	others := false
	for _, d := range dmg {
		if d.Class == eink.Stroke {
			l.strokeLater = append(l.strokeLater, d.Rect)
			continue
		}
		out = append(out, d)
		others = true
	}
	if len(l.strokeLater) == 0 {
		return out
	}
	now := time.Now()
	if wait := interval - now.Sub(l.strokeSent); wait > 0 && !others {
		if l.strokeCancel == nil {
			l.strokeCancel = l.sysCtx.After(wait, func() { l.strokeCancel = nil })
		}
		return out
	}
	for _, r := range eink.Merge(l.strokeLater) {
		out = append(out, eink.Damage{Rect: r, Class: eink.Stroke})
	}
	l.strokeLater = l.strokeLater[:0]
	l.strokeSent = now
	if l.strokeCancel != nil {
		l.strokeCancel()
		l.strokeCancel = nil
	}
	return out
}

// invert пишет в dst инверсию области r из src: ночной режим, белое на чёрном.
func invert(dst, src *image.Gray, r image.Rectangle) {
	r = r.Intersect(src.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := src.Pix[src.PixOffset(r.Min.X, y):src.PixOffset(r.Max.X, y)]
		d := dst.Pix[dst.PixOffset(r.Min.X, y):dst.PixOffset(r.Max.X, y)]
		for i, v := range s {
			d[i] = ^v
		}
	}
}

func (l *Loop) paint(r image.Rectangle) {
	c := gfx.NewCanvas(l.buf).WithClip(r)
	c.Fill(r, gfx.White)
	if f := l.top(); f != nil {
		owner, scr := f.owner, f.screen
		l.safe(owner, func() { scr.Draw(c, l.theme) })
	}
	for _, ov := range l.overlays {
		if l.shown(ov) && ov.o.Bounds().Overlaps(r) {
			o := ov.o
			l.safe(ov.owner, func() { o.Draw(c, l.theme) })
		}
	}
}

// pauseAll сообщает верхнему экрану и запущенным приложениям, что книга засыпает или выходит.
func (l *Loop) pauseAll() {
	if f := l.top(); f != nil {
		l.lifecycle(f, Paused)
	}
	for _, r := range append([]*running(nil), l.apps...) {
		if p, ok := r.app.(Pauser); ok && !r.stopped {
			l.safe(r, func() { p.Pause(r.ctx) })
		}
	}
}

// lightOff гасит подсветку на время сна и возвращает функцию, которая её восстановит.
func (l *Loop) lightOff() (restore func()) {
	light := l.dev.Light()
	if light == nil || light.Brightness() == 0 {
		return func() {}
	}
	level := light.Brightness()
	if err := light.SetBrightness(0); err != nil {
		l.log.Error("погасить подсветку перед сном", "err", err)
		return func() {}
	}
	return func() {
		if err := light.SetBrightness(level); err != nil {
			l.log.Error("вернуть подсветку после сна", "err", err)
		}
	}
}

// drawFullScreen рисует картинку на весь экран и дожидается конца полного обновления.
func (l *Loop) drawFullScreen(paint func(c *gfx.Canvas, t *ui.Theme)) {
	c := gfx.NewCanvas(l.buf)
	c.Fill(l.buf.Bounds(), gfx.White)
	l.safe(nil, func() { paint(c, l.theme) })
	src := l.buf
	if l.night {
		invert(l.out, l.buf, l.buf.Bounds())
		src = l.out
	}
	u := device.Update{Rect: l.disp.Bounds(), Waveform: device.WaveformGC16, Full: true}
	if err := l.sched.Present(src, u); err != nil {
		l.log.Error("полное обновление", "err", err)
	}
	if err := l.sched.WaitAll(); err != nil {
		l.log.Error("ожидание обновлений", "err", err)
	}
}

// connect включает Wi-Fi для сетевого запроса приложения и отмечает, что сеть нужна.
// Устройство без управления сетью (компьютер, тесты) пропускает запросы как есть.
func (l *Loop) connect(ctx context.Context) error {
	l.netUsed.Store(time.Now().UnixNano())
	if l.net == nil {
		if _, ok := l.dev.(device.Networker); ok {
			return device.ErrNoWifi
		}
		return nil
	}
	if l.netOff.Load() {
		return device.ErrWifiDisabled
	}
	err := l.net.Up(ctx)
	l.netUsed.Store(time.Now().UnixNano())
	return err
}

func (l *Loop) newWeb(st *storage.App) *web.Client {
	return web.New(web.Options{Connect: l.connect, Transport: l.cfg.HTTP, Cache: st, UserAgent: l.cfg.UserAgent, Now: l.clock})
}
