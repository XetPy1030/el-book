package app

import (
	"context"
	"image"
	"log/slog"
	"runtime/debug"
	"slices"
	"time"

	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
	"el-book/sdk/web"
)

// Context — то, через что приложение и оболочка работают с платформой. Методы вызываются
// только из горутины интерфейса, кроме Post.
type Context struct {
	l     *Loop
	owner *running // nil — оболочка
}

// Theme — шрифты и размеры.
func (c *Context) Theme() *ui.Theme { return c.l.theme }

// Bounds — размер экрана.
func (c *Context) Bounds() image.Rectangle { return c.l.disp.Bounds() }

// Rotation — текущая ориентация экрана.
func (c *Context) Rotation() device.Rotation { return c.l.disp.Rotation() }

// Now — текущее время (в тестах — подменённое).
func (c *Context) Now() time.Time { return c.l.clock() }

// Log — журнал с пометкой приложения.
func (c *Context) Log() *slog.Logger {
	if c.owner == nil {
		return c.l.log.With("app", "shell")
	}
	return c.l.log.With("app", c.owner.man.ID)
}

// Storage — хранилище приложения; у оболочки — «shell».
func (c *Context) Storage() *storage.App {
	if c.owner == nil {
		return c.l.cfg.Store.App("shell")
	}
	return c.owner.store
}

// Web — HTTP-клиент приложения: сам включает Wi-Fi, кэширует ответы в кэше приложения.
// Его методы блокируются — зовите их из фоновой работы (Go, kit.Load).
func (c *Context) Web() *web.Client {
	if c.owner == nil {
		if c.l.shellWeb == nil {
			c.l.shellWeb = c.l.newWeb(c.Storage())
		}
		return c.l.shellWeb
	}
	if c.owner.web == nil {
		c.owner.web = c.l.newWeb(c.owner.store)
	}
	return c.owner.web
}

// Visible — экраны владельца сейчас на экране: приложение не ушло в фон.
func (c *Context) Visible() bool { return c.owner == c.l.active }

// Invalidate просит перерисовать область r; класс выбирает способ обновления панели.
// Приложение в фоне ничего не перерисовывает: его экран покажут целиком, когда вернутся.
func (c *Context) Invalidate(r image.Rectangle, cl eink.Class) {
	if c.owner != nil && c.owner != c.l.active {
		return
	}
	c.l.damage = append(c.l.damage, eink.Damage{Rect: r, Class: cl})
}

// InvalidateAll просит перерисовать весь экран.
func (c *Context) InvalidateAll(cl eink.Class) {
	if c.owner != nil && c.owner != c.l.active {
		return
	}
	c.l.invalidateAll(cl)
}

// Post выполняет f в горутине интерфейса. Можно вызывать из любой горутины.
func (c *Context) Post(f func()) {
	owner := c.owner
	l := c.l
	l.posts <- func() {
		if owner != nil && owner.stopped {
			return
		}
		l.safe(owner, f)
	}
}

// After выполняет f в горутине интерфейса через d. Возвращает отмену.
func (c *Context) After(d time.Duration, f func()) (cancel func()) {
	t := time.AfterFunc(d, func() { c.Post(f) })
	return func() { t.Stop() }
}

// Go запускает фоновую работу. Паника в ней закрывает приложение, а не процесс;
// контекст отменяется, когда приложение закрыто.
func (c *Context) Go(f func(ctx context.Context)) {
	owner := c.owner
	bg := c.l.root
	if owner != nil {
		bg = owner.bg
	}
	l := c.l
	go func() {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				l.posts <- func() { l.crash(owner, r, stack) }
			}
		}()
		f(bg)
	}()
}

// Push открывает экран поверх текущего.
func (c *Context) Push(s Screen) { c.l.push(c.owner, s) }

// Pop закрывает верхний экран владельца. Закрыт последний экран приложения — приложение
// останавливается и открывается домашний экран.
func (c *Context) Pop() { c.l.pop(c.owner) }

// Home открывает домашний экран. Приложение не закрывается: оно ждёт в фоне и откроется
// в том же месте.
func (c *Context) Home() { c.l.goHome() }

// Close закрывает приложение целиком; у оболочки — то же, что Home.
func (c *Context) Close() {
	if c.owner == nil {
		c.l.goHome()
		return
	}
	c.l.removeApp(c.owner, true)
}

// ShowOverlay показывает оверлей: панель, диалог, тост.
func (c *Context) ShowOverlay(o ui.Overlay) {
	l := c.l
	o.SetCloser(func() { c.CloseOverlay(o) })
	o.Layout(l.theme, l.disp.Bounds())
	l.overlays = append(l.overlays, overlay{o: o, owner: c.owner})
	c.Invalidate(o.Bounds(), eink.UI)
}

// HasOverlay сообщает, показан ли оверлей: его могли закрыть кнопкой или возвратом домой.
func (c *Context) HasOverlay(o ui.Overlay) bool {
	for _, ov := range c.l.overlays {
		if ov.o == o {
			return true
		}
	}
	return false
}

// CloseOverlay убирает оверлей.
func (c *Context) CloseOverlay(o ui.Overlay) {
	l := c.l
	for i, ov := range l.overlays {
		if ov.o == o {
			l.overlays = append(l.overlays[:i], l.overlays[i+1:]...)
			c.Invalidate(o.Bounds(), eink.UI)
			return
		}
	}
}

// Toast показывает короткое сообщение. Новое заменяет прежнее: два сообщения на одном
// месте наложились бы друг на друга.
func (c *Context) Toast(text string) {
	for _, ov := range slices.Clone(c.l.overlays) {
		if old, ok := ov.o.(*ui.Toast); ok {
			c.CloseOverlay(old)
		}
	}
	t := ui.NewToast(text)
	c.ShowOverlay(t)
	c.After(t.Duration, func() { c.CloseOverlay(t) })
}

// Fail сообщает об ошибке действия пользователя: пишет её в журнал с attrs и показывает
// тост text. Текст — понятный пользователю: «Не удалось сохранить рисунок».
func (c *Context) Fail(text string, err error, attrs ...any) {
	c.Log().Error(text, append([]any{"err", err}, attrs...)...)
	c.Toast(text)
}

// Confirm спрашивает подтверждение необратимого действия.
func (c *Context) Confirm(title, message, yes string, onYes func()) {
	c.ShowOverlay(ui.NewDialog(c.l.theme, title, message,
		ui.DialogButton{Text: "Отмена"},
		ui.DialogButton{Text: yes, Primary: true, OnTap: onYes},
	))
}

// Menu показывает меню действий.
func (c *Context) Menu(title string, items ...ui.MenuItem) {
	c.ShowOverlay(ui.NewMenu(c.l.theme, title, items...))
}

// Sys — системные возможности; есть только у оболочки, у приложения — nil.
func (c *Context) Sys() *Sys {
	if c.owner != nil {
		return nil
	}
	return &Sys{l: c.l}
}

// Sys — то, что разрешено только оболочке: запуск приложений, устройство, сон, выход.
type Sys struct {
	l *Loop
}

// Device — устройство: подсветка, батарея, сведения о модели.
func (s *Sys) Device() device.Device { return s.l.dev }

// Network — Wi-Fi; nil, если его нет.
func (s *Sys) Network() device.Network { return s.l.net }

// AllowNetwork разрешает или запрещает приложениям включать Wi-Fi: выключенный
// в настройках Wi-Fi запросы не включают, они сразу получают device.ErrWifiDisabled.
func (s *Sys) AllowNetwork(on bool) { s.l.netOff.Store(!on) }

// NetworkUsed — когда приложения последний раз ходили в сеть: по нему оболочка гасит
// Wi-Fi без дела.
func (s *Sys) NetworkUsed() time.Time {
	n := s.l.netUsed.Load()
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// Storage — хранилище приложения id: для строк домашнего экрана и очистки данных.
func (s *Sys) Storage(id string) *storage.App { return s.l.cfg.Store.App(id) }

// Launch открывает приложение: запущенное — в том же состоянии, новое — запускает.
func (s *Sys) Launch(a App) { s.l.launch(a) }

// Running — манифест приложения на экране; ok = false, если открыта оболочка.
func (s *Sys) Running() (Manifest, bool) {
	if s.l.active == nil {
		return Manifest{}, false
	}
	return s.l.active.man, true
}

// Recent — запущенные приложения, недавние первыми.
func (s *Sys) Recent() []Manifest {
	out := make([]Manifest, 0, len(s.l.apps))
	for i := len(s.l.apps) - 1; i >= 0; i-- {
		out = append(out, s.l.apps[i].man)
	}
	return out
}

// CloseApp закрывает запущенное приложение id; приложение сохраняется в Stop.
func (s *Sys) CloseApp(id string) {
	for _, r := range s.l.apps {
		if r.man.ID == id {
			s.l.removeApp(r, true)
			return
		}
	}
}

// Dispatch отдаёт событие экранам в обход Intercept: оболочка придержала касание у края
// экрана, а жеста не вышло — касание отдаётся приложению как было.
func (s *Sys) Dispatch(ev device.Event) { s.l.dispatch(ev) }

// LastActivity — когда пользователь последний раз касался экрана или нажимал кнопку.
func (s *Sys) LastActivity() time.Time { return s.l.lastUser }

// Suspend усыпляет книгу: приложения сохраняются, рисуется экран сна, подсветка гаснет
// до пробуждения, после пробуждения экран перерисовывается полностью. Касания, накопившиеся
// за это время, отбрасываются.
func (s *Sys) Suspend(sleepScreen func(c *gfx.Canvas, t *ui.Theme)) error {
	l := s.l
	l.pauseAll()
	l.drawFullScreen(sleepScreen)
	restore := l.lightOff()
	err := l.dev.Power().Suspend()
	restore()
	l.recog.Reset()
	l.lastUser = l.clock()
	l.awoke = l.lastUser
	if f := l.top(); f != nil {
		l.lifecycle(f, Resumed)
	}
	l.invalidateAll(eink.Clean)
	return err
}

// Exit завершает оболочку с кодом для скрипта запуска; перед этим закрывает приложения
// (они сохраняются) и рисует прощальный экран.
func (s *Sys) Exit(code int, farewell func(c *gfx.Canvas, t *ui.Theme)) {
	l := s.l
	l.pauseAll()
	for len(l.apps) > 0 {
		l.removeApp(l.apps[len(l.apps)-1], true)
	}
	l.goHome()
	if farewell != nil {
		l.drawFullScreen(farewell)
	}
	l.exitCode = &code
}

// SetRotation поворачивает экран и перераскладывает экраны.
func (s *Sys) SetRotation(r device.Rotation) {
	if s.l.setRotation(r) {
		s.l.showTop()
		s.l.invalidateAll(eink.Clean)
	}
}

// SetNight включает ночной режим: белое на чёрном. Панель перерисовывается полностью.
func (s *Sys) SetNight(on bool) {
	if s.l.night == on {
		return
	}
	s.l.night = on
	s.l.invalidateAll(eink.Clean)
}

// Night — включён ли ночной режим.
func (s *Sys) Night() bool { return s.l.night }

// Refresh полностью перерисовывает экран с миганием: убрать следы прежних картинок.
func (s *Sys) Refresh() { s.l.invalidateAll(eink.Clean) }

// Policy — политика обновлений панели: оболочка меняет частоту полного обновления.
func (s *Sys) Policy() *eink.Policy { return s.l.policy }

// Stats — счётчики обновлений панели: для журнала и отладки.
func (s *Sys) Stats() (updates, waits int) {
	return s.l.sched.Stats.Updates, s.l.sched.Stats.Waits
}
