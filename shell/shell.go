// Package shell — оболочка платформы: домашний экран, быстрые настройки, жесты от краёв,
// подсветка, Wi-Fi, сон, USB, настройки. Владеет устройством; приложения запускает через app.Sys.
package shell

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ui"
)

// Options — параметры оболочки.
type Options struct {
	Apps    []app.App
	Version string
	// WifiDefault — включён ли Wi-Fi, пока пользователь не выбрал сам. На книге выключен,
	// пока спайк включения Wi-Fi не пройден; в эмуляторе включён.
	WifiDefault bool
}

// settingsVersion — версия схемы настроек оболочки. Версия 2 добавила закреплённые
// приложения, Wi-Fi, ночной режим, жесты от краёв и экран сна, а время запусков (recent)
// убрала: порядок на домашнем экране не прыгает. Новые поля при чтении старого файла
// остаются по умолчанию.
const settingsVersion = 2

// Settings — настройки оболочки, сохраняются между запусками.
type Settings struct {
	Brightness  int      `json:"brightness"`
	Warmth      int      `json:"warmth"`
	AutoSleep   int      `json:"auto_sleep_min"` // 0 — не засыпать само
	FlashEvery  int      `json:"flash_every"`
	Pinned      []string `json:"pinned"`            // закреплённые приложения: первыми на домашнем экране
	Wifi        bool     `json:"wifi"`              // приложениям можно включать Wi-Fi
	WifiAutoOff int      `json:"wifi_auto_off_min"` // гасить Wi-Fi без дела через, мин; 0 — не гасить
	Night       bool     `json:"night"`             // ночной режим: светлое на тёмном
	Edges       bool     `json:"edge_gestures"`     // жесты от верхнего и нижнего края
	SleepScreen string   `json:"sleep_screen"`      // "" — сообщение; id приложения — его картинка
}

// Shell реализует app.System.
type Shell struct {
	opts      Options
	settings  Settings
	loaded    bool
	resumedAt time.Time
	home      *Home
	sleepTick func()
	usb       *ui.Panel                               // окно «Книга подключена к компьютеру»
	hw        func(ctx *app.Context, ev device.Event) // проверка касаний показывает кнопки и датчик
	edges     *edgeSwipe
	edgeTimer func()
	quick     *quickPanel
	wifi      *wifiScreen // открытый экран Wi-Fi: обновляется по событиям сети
	netOnAt   time.Time   // когда Wi-Fi последний раз включили из оболочки
	lowWarned int         // о каком заряде уже предупредили: 0, lowBattery или criticalBattery
}

// Пороги предупреждений о заряде, %.
const (
	lowBattery      = 10
	criticalBattery = 5
)

// New создаёт оболочку.
func New(opts Options) *Shell {
	return &Shell{opts: opts}
}

func (s *Shell) load(ctx *app.Context) {
	if s.loaded {
		return
	}
	s.loaded = true
	s.settings = Settings{AutoSleep: 10, FlashEvery: 8, Wifi: s.opts.WifiDefault, WifiAutoOff: 5, Edges: true}
	if light := ctx.Sys().Device().Light(); light != nil {
		s.settings.Brightness, s.settings.Warmth = light.Brightness(), light.Warmth()
	}
	if _, err := ctx.Storage().LoadSettings(&s.settings); err == nil {
		s.applyLight(ctx)
	}
	sys := ctx.Sys()
	sys.Policy().FlashEvery = s.settings.FlashEvery
	sys.AllowNetwork(s.settings.Wifi)
	if s.settings.Night {
		sys.SetNight(true)
	}
	t := ctx.Theme()
	s.edges = &edgeSwipe{zone: t.MM(3.5), distance: t.MM(10), maxTime: 450 * time.Millisecond}
}

func (s *Shell) save(ctx *app.Context) {
	if err := ctx.Storage().SaveSettings(settingsVersion, s.settings); err != nil {
		ctx.Fail("Не удалось сохранить настройки", err)
	}
}

func (s *Shell) applyLight(ctx *app.Context) {
	light := ctx.Sys().Device().Light()
	if light == nil {
		return
	}
	if err := light.SetBrightness(s.settings.Brightness); err != nil {
		ctx.Log().Error("яркость", "err", err)
	}
	if light.HasWarmth() {
		if err := light.SetWarmth(s.settings.Warmth); err != nil {
			ctx.Log().Error("теплота", "err", err)
		}
	}
}

// Home — домашний экран.
func (s *Shell) Home(ctx *app.Context) app.Screen {
	s.load(ctx)
	s.home = newHome(s)
	s.scheduleTick(ctx)
	return s.home
}

// Intercept обрабатывает кнопку питания, обложку, кабель, датчик положения, жесты от краёв
// и сеть. События железа пишутся в журнал: по нему проверяют коды кнопок и датчиков на книге.
func (s *Shell) Intercept(ctx *app.Context, ev device.Event) bool {
	switch e := ev.(type) {
	case device.TouchEvent:
		return s.edgeTouch(ctx, e)
	case device.KeyEvent:
		ctx.Log().Info("кнопка", "key", e.Key, "pressed", e.Pressed)
		s.showHardware(ctx, ev)
		// После пробуждения приходят события той же кнопки, которой будили: их пропускаем.
		if time.Since(s.resumedAt) < 1500*time.Millisecond && (e.Key == device.KeyPower || e.Key == device.KeySleepCover) {
			return true
		}
		switch {
		case e.Key == device.KeyPower && e.Pressed:
			s.Sleep(ctx)
			return true
		case e.Key == device.KeyPower:
			return true
		case e.Key == device.KeySleepCover:
			if e.Pressed {
				s.Sleep(ctx)
			}
			return true
		case e.Key == device.KeyHome && e.Pressed:
			s.hw = nil
			s.closeQuick(ctx)
			ctx.Home()
			return true
		}
	case device.PowerSourceEvent:
		ctx.Log().Info("кабель", "source", e.Source, "connected", e.Connected)
		s.refreshStatus(ctx)
		s.batteryCheck(ctx)
		switch {
		case e.Source == device.SourceComputer && e.Connected:
			s.askUSB(ctx)
		case e.Source == device.SourceComputer:
			s.closeUSB(ctx) // кабель вынули, не ответив: вопрос больше не нужен
		case e.Connected:
			ctx.Toast("Заряжается")
		}
		return true
	case device.OrientationEvent:
		ctx.Log().Info("датчик положения", "rotation", e.Rotation)
		s.showHardware(ctx, ev)
		return true // поворот по датчику пока не используется: экран остаётся в выбранной ориентации
	case device.NetworkEvent:
		s.onNetwork(ctx, e.Status)
		return false // приложения тоже узнают, что сеть появилась
	}
	return false
}

// edgeTouch пропускает касание через жесты от краёв. Пока открыта проверка касаний,
// жестов нет: там рисуют у самых краёв.
func (s *Shell) edgeTouch(ctx *app.Context, e device.TouchEvent) bool {
	if s.edges == nil || !s.settings.Edges || s.hw != nil {
		return false
	}
	wasHolding := s.edges.holding()
	consumed, replay, fired := s.edges.feed(e, ctx.Bounds())
	for _, ev := range replay {
		ctx.Sys().Dispatch(ev)
	}
	switch {
	case fired == edgeTop:
		s.openQuick(ctx)
	case fired == edgeBottom:
		s.closeQuick(ctx)
		ctx.Home()
	case !wasHolding && s.edges.holding():
		// Палец стоит у края: через maxTime касание уходит приложению, чтобы долгое
		// нажатие у края работало.
		if s.edgeTimer != nil {
			s.edgeTimer()
		}
		s.edgeTimer = ctx.After(s.edges.maxTime, func() {
			for _, ev := range s.edges.expire() {
				ctx.Sys().Dispatch(ev)
			}
		})
	}
	return consumed
}

// Sleep усыпляет книгу. Wi-Fi перед сном гасится: со включённым модулем сон ненадёжен.
func (s *Shell) Sleep(ctx *app.Context) {
	if s.edges != nil {
		s.edges.reset()
	}
	if n := ctx.Sys().Network(); n != nil && n.Status().State != device.NetOff {
		if err := n.Down(); err != nil {
			ctx.Log().Error("Wi-Fi перед сном", "err", err)
		}
	}
	screen := paintSleep
	if p := s.sleepPainter(ctx); p != nil {
		screen = p
	}
	switch err := ctx.Sys().Suspend(screen); {
	case errors.Is(err, device.ErrSuspendCancelled):
		ctx.Log().Info("сон отменён", "err", err)
	case err != nil:
		ctx.Fail("Книга не смогла уснуть", err)
	}
	s.resumedAt = time.Now()
	s.refreshStatus(ctx)
}

// sleepPainter — экран сна от приложения, выбранного в настройках; nil — обычный.
func (s *Shell) sleepPainter(ctx *app.Context) func(c *gfx.Canvas, t *ui.Theme) {
	a := s.appByID(s.settings.SleepScreen)
	if a == nil {
		return nil
	}
	sp, ok := a.(app.SleepPainter)
	if !ok {
		return nil
	}
	st := ctx.Sys().Storage(a.Manifest().ID)
	return func(c *gfx.Canvas, t *ui.Theme) {
		if !sp.PaintSleep(st, c, t) {
			paintSleep(c, t)
		}
	}
}

// scheduleTick раз в полминуты проверяет автосон и гасит Wi-Fi без дела.
func (s *Shell) scheduleTick(ctx *app.Context) {
	if s.sleepTick != nil {
		s.sleepTick()
	}
	s.sleepTick = ctx.After(30*time.Second, func() {
		s.wifiIdleCheck(ctx)
		s.batteryCheck(ctx)
		if m := s.settings.AutoSleep; m > 0 && ctx.Now().Sub(ctx.Sys().LastActivity()) >= time.Duration(m)*time.Minute {
			s.Sleep(ctx)
		}
		s.scheduleTick(ctx)
	})
}

// batteryCheck предупреждает о низком заряде: при 10 % — подсказкой, при 5 % — окном.
// Каждое предупреждение — один раз; после зарядки — заново.
func (s *Shell) batteryCheck(ctx *app.Context) {
	bat := ctx.Sys().Device().Battery()
	level, err := bat.Level()
	if err != nil {
		return
	}
	if charging, _ := bat.Charging(); charging || level > lowBattery {
		s.lowWarned = 0
		return
	}
	switch {
	case level <= criticalBattery && s.lowWarned != criticalBattery:
		s.lowWarned = criticalBattery
		ctx.Log().Warn("заряд почти кончился", "level", level)
		ctx.ShowOverlay(ui.NewDialog(ctx.Theme(), "Заряд почти кончился",
			fmt.Sprintf("Осталось %d %%. Подключите зарядку, иначе книга скоро выключится.", level),
			ui.DialogButton{Text: "Понятно", Primary: true}))
	case level > criticalBattery && s.lowWarned == 0:
		s.lowWarned = lowBattery
		ctx.Toast(fmt.Sprintf("Заряд %d %% — пора на зарядку", level))
	}
}

// wifiIdleCheck гасит Wi-Fi, если им давно никто не пользовался: ни приложения, ни экран
// Wi-Fi. Включится снова при следующем запросе приложения.
func (s *Shell) wifiIdleCheck(ctx *app.Context) {
	n := ctx.Sys().Network()
	m := s.settings.WifiAutoOff
	if n == nil || m <= 0 || s.wifi != nil || n.Status().State == device.NetOff {
		return
	}
	last := ctx.Sys().NetworkUsed()
	if s.netOnAt.After(last) {
		last = s.netOnAt
	}
	if ctx.Now().Sub(last) < time.Duration(m)*time.Minute {
		return
	}
	ctx.Log().Info("Wi-Fi без дела, выключаем", "минут", m)
	ctx.Go(func(context.Context) {
		if err := n.Down(); err != nil {
			ctx.Log().Error("выключение Wi-Fi", "err", err)
		}
	})
}

// setWifi включает или выключает Wi-Fi целиком: разрешение приложениям и сам модуль.
func (s *Shell) setWifi(ctx *app.Context, on bool) {
	s.settings.Wifi = on
	s.save(ctx)
	sys := ctx.Sys()
	sys.AllowNetwork(on)
	n := sys.Network()
	if n == nil {
		if on {
			ctx.Toast("На этой книге Wi-Fi недоступен")
		}
		return
	}
	if on {
		s.netOnAt = ctx.Now()
		ctx.Go(func(bg context.Context) {
			bg, cancel := context.WithTimeout(bg, time.Minute)
			defer cancel()
			if err := n.Up(bg); err != nil {
				ctx.Log().Info("Wi-Fi не подключился", "err", err)
			}
		})
	} else {
		ctx.Go(func(context.Context) {
			if err := n.Down(); err != nil {
				ctx.Log().Error("выключение Wi-Fi", "err", err)
			}
		})
	}
	s.refreshStatus(ctx)
}

// onNetwork — состояние Wi-Fi изменилось.
func (s *Shell) onNetwork(ctx *app.Context, st device.NetStatus) {
	ctx.Log().Info("сеть", "состояние", st.State.String(), "сеть", st.SSID)
	s.refreshStatus(ctx)
	if s.wifi != nil {
		s.wifi.update(ctx)
	}
	if s.quick != nil {
		s.quick.update(ctx)
	}
}

func (s *Shell) refreshStatus(ctx *app.Context) {
	if s.home != nil {
		s.home.refreshStatus(ctx)
	}
}

// showHardware передаёт событие кнопки или датчика проверке касаний, если она открыта.
func (s *Shell) showHardware(ctx *app.Context, ev device.Event) {
	if s.hw != nil {
		s.hw(ctx, ev)
	}
}

// askUSB спрашивает, что делать с подключённым компьютером. Повторное подключение не
// открывает второе окно поверх первого.
func (s *Shell) askUSB(ctx *app.Context) {
	if s.usb != nil && ctx.HasOverlay(s.usb) {
		return
	}
	s.usb = ui.NewDialog(ctx.Theme(), "Книга подключена к компьютеру",
		"Чтобы перенести файлы, нужен Nickel: он покажет книгу компьютеру как диск. Вернуться в Nickel?",
		ui.DialogButton{Text: "Только зарядка"},
		ui.DialogButton{Text: "В Nickel", Primary: true, OnTap: func() { s.ExitToNickel(ctx) }},
	)
	ctx.ShowOverlay(s.usb)
}

func (s *Shell) closeUSB(ctx *app.Context) {
	if s.usb != nil {
		ctx.CloseOverlay(s.usb)
		s.usb = nil
	}
}

// confirmNickel спрашивает, вернуться ли в Nickel.
func (s *Shell) confirmNickel(ctx *app.Context) {
	ctx.Confirm("Вернуться в Nickel?", "Оболочка закроется, откроется штатная программа книги.", "В Nickel",
		func() { s.ExitToNickel(ctx) })
}

// ExitToNickel завершает оболочку; скрипт запуска вернёт Nickel.
func (s *Shell) ExitToNickel(ctx *app.Context) {
	s.save(ctx)
	ctx.Sys().Exit(app.ExitNickel, func(c *gfx.Canvas, t *ui.Theme) {
		paintMessage(c, t, icons.Exit, "Возвращаемся в Nickel", "Это займёт несколько секунд")
	})
}

// Crashed показывает, что приложение упало.
func (s *Shell) Crashed(ctx *app.Context, m app.Manifest, err any, stack []byte) {
	msg := strings.SplitN(fmt.Sprint(err), "\n", 2)[0]
	ctx.ShowOverlay(ui.NewDialog(ctx.Theme(), fmt.Sprintf("«%s» закрылось с ошибкой", m.Title),
		"Данные, сохранённые до ошибки, на месте. Подробности — в журнале.\n"+msg,
		ui.DialogButton{Text: "Понятно", Primary: true},
	))
}

// launch запускает приложение.
func (s *Shell) launch(ctx *app.Context, a app.App) {
	s.closeQuick(ctx)
	ctx.Sys().Launch(a)
}

// appByID — приложение по id; nil — нет такого.
func (s *Shell) appByID(id string) app.App {
	if id == "" {
		return nil
	}
	for _, a := range s.opts.Apps {
		if a.Manifest().ID == id {
			return a
		}
	}
	return nil
}

func (s *Shell) pinned(id string) bool { return slices.Contains(s.settings.Pinned, id) }

// togglePin закрепляет приложение первым на домашнем экране или открепляет его.
func (s *Shell) togglePin(ctx *app.Context, id string) {
	if i := slices.Index(s.settings.Pinned, id); i >= 0 {
		s.settings.Pinned = slices.Delete(s.settings.Pinned, i, i+1)
	} else {
		s.settings.Pinned = append(s.settings.Pinned, id)
	}
	s.save(ctx)
}

func (s *Shell) running(ctx *app.Context, id string) bool {
	for _, m := range ctx.Sys().Recent() {
		if m.ID == id {
			return true
		}
	}
	return false
}

func paintSleep(c *gfx.Canvas, t *ui.Theme) {
	paintMessage(c, t, icons.Moon, "Сон", "Нажмите кнопку питания или откройте обложку")
}

// paintMessage рисует экран с крупной иконкой, заголовком и пояснением.
func paintMessage(c *gfx.Canvas, t *ui.Theme, icon icons.Drawer, title, hint string) {
	b := c.Bounds()
	size := t.MM(28)
	cy := b.Min.Y + b.Dy()*2/5
	icon(c, rectAround(b.Min.X+b.Dx()/2, cy-size/2, size), gfx.Black)
	header := t.Header()
	y := cy + size/2 + t.MM(8)
	header.DrawIn(c, title, rect(b.Min.X, y, b.Max.X, y+header.Height), gfx.AlignCenter, gfx.Black)
	y += header.Height + t.MM(3)
	body := t.Body()
	for _, line := range body.Wrap(hint, b.Dx()-2*t.MM(12)) {
		body.DrawIn(c, line, rect(b.Min.X, y, b.Max.X, y+body.Height), gfx.AlignCenter, gfx.Gray16(5))
		y += body.Height
	}
	caption := t.Caption()
	caption.DrawIn(c, "el-book", rect(b.Min.X, b.Max.Y-t.MM(12), b.Max.X, b.Max.Y-t.MM(6)), gfx.AlignCenter, gfx.Gray16(7))
}
