package app

import (
	"context"
	"fmt"
	"image"
	"slices"

	"el-book/sdk/device"
	"el-book/sdk/eink"
)

// Стеки экранов. У оболочки свой стек (в основании — домашний экран), у каждого запущенного
// приложения — свой. На экране — стек активного приложения или, если его нет, стек оболочки.
// Уход домой приложение не закрывает: оно получает Hidden, сохраняется и ждёт в фоне; повторный
// запуск показывает его в том же состоянии. Сверх Config.MaxApps самое давнее закрывается.

// visible — стек, который сейчас на экране.
func (l *Loop) visible() *[]frame {
	if l.active != nil {
		return &l.active.stack
	}
	return &l.shell
}

func (l *Loop) top() *frame {
	s := *l.visible()
	if len(s) == 0 {
		return nil
	}
	return &s[len(s)-1]
}

// hideTop сообщает верхнему экрану, что его закрывает другой.
func (l *Loop) hideTop() {
	if f := l.top(); f != nil {
		l.lifecycle(f, Hidden)
	}
}

// push кладёт экран в стек владельца. Экран оболочки уводит с экрана приложение (оно
// остаётся в фоне); экран фонового приложения ждёт, пока приложение снова откроют.
func (l *Loop) push(owner *running, s Screen) {
	if owner == nil {
		l.hideTop()
		l.active = nil
		l.shell = append(l.shell, frame{screen: s})
		l.showTop()
		return
	}
	if owner.stopped {
		return
	}
	if owner != l.active {
		owner.stack = append(owner.stack, frame{screen: s, owner: owner})
		return
	}
	l.hideTop()
	owner.stack = append(owner.stack, frame{screen: s, owner: owner})
	l.showTop()
}

// pop закрывает верхний экран владельца. Закрыт последний экран приложения — приложение
// останавливается и открывается экран оболочки.
func (l *Loop) pop(owner *running) {
	if owner == nil {
		if l.active != nil || len(l.shell) <= 1 {
			return
		}
		l.shell = l.shell[:len(l.shell)-1]
		l.showTop()
		return
	}
	if owner.stopped || len(owner.stack) == 0 {
		return
	}
	owner.stack = owner.stack[:len(owner.stack)-1]
	if len(owner.stack) == 0 {
		l.removeApp(owner, true)
		return
	}
	if owner == l.active {
		l.showTop()
	}
}

// goHome показывает домашний экран; запущенные приложения остаются в фоне, панели
// оболочки закрываются.
func (l *Loop) goHome() {
	if l.active != nil || len(l.shell) > 1 {
		l.hideTop()
	}
	l.active = nil
	l.shell = l.shell[:min(len(l.shell), 1)]
	l.overlays = slices.DeleteFunc(l.overlays, func(o overlay) bool { return o.owner == nil })
	l.showTop()
}

// activate выводит приложение r на экран (nil — оболочку).
func (l *Loop) activate(r *running) {
	if r == l.active {
		l.showTop()
		return
	}
	l.hideTop()
	l.shell = l.shell[:min(len(l.shell), 1)]
	l.overlays = slices.DeleteFunc(l.overlays, func(o overlay) bool { return o.owner == nil && o.o.Modal() })
	l.active = r
	if r != nil {
		l.apps = slices.DeleteFunc(l.apps, func(x *running) bool { return x == r })
		l.apps = append(l.apps, r)
		if !r.man.Landscape && l.disp.Rotation().Landscape() {
			l.setRotation(device.Portrait)
			l.invalidateAll(eink.Clean)
		}
	}
	l.showTop()
}

// launch открывает приложение: запущенное — возвращает на экран, новое — запускает.
func (l *Loop) launch(a App) {
	man := a.Manifest()
	for _, r := range l.apps {
		if r.man.ID == man.ID {
			l.activate(r)
			return
		}
	}
	r := &running{app: a, man: man, store: l.cfg.Store.App(man.ID)}
	r.bg, r.cancel = context.WithCancel(l.root)
	r.ctx = &Context{l: l, owner: r}
	var scr Screen
	if !l.safe(r, func() { scr = a.Start(r.ctx) }) {
		return
	}
	if scr == nil {
		l.crash(r, fmt.Sprintf("приложение %s не вернуло экран", man.ID), nil)
		return
	}
	r.stack = append([]frame{{screen: scr, owner: r}}, r.stack...)
	l.apps = append(l.apps, r)
	l.activate(r)
	l.evict()
}

// evict закрывает самые давние приложения сверх MaxApps.
func (l *Loop) evict() {
	for len(l.apps) > l.cfg.MaxApps {
		victim := l.apps[0]
		if victim == l.active {
			return
		}
		l.log.Info("приложение закрыто: много открытых", "app", victim.man.ID)
		l.removeApp(victim, true)
	}
}

// removeApp убирает экраны и оверлеи приложения и останавливает его.
func (l *Loop) removeApp(r *running, callStop bool) {
	if r.stopped {
		return
	}
	wasActive := r == l.active
	if wasActive && callStop {
		l.hideTop()
	}
	l.overlays = slices.DeleteFunc(l.overlays, func(o overlay) bool { return o.owner == r })
	l.apps = slices.DeleteFunc(l.apps, func(x *running) bool { return x == r })
	if callStop {
		if s, ok := r.app.(Stopper); ok {
			l.safe(r, func() { s.Stop(r.ctx) })
		}
	}
	r.stopped = true
	r.cancel()
	r.stack = nil
	if wasActive {
		l.active = nil
		l.showTop()
	}
}

// showTop раскладывает верхний экран, если нужно, и сообщает, что он показан.
func (l *Loop) showTop() {
	f := l.top()
	if f == nil {
		return
	}
	b := l.disp.Bounds()
	if f.laid != b {
		owner, scr := f.owner, f.screen
		ctx := l.ctxFor(owner)
		if l.safe(owner, func() { scr.Layout(ctx, b) }) {
			if f2 := l.top(); f2 != nil && f2.screen == scr {
				f2.laid = b
			}
		}
	}
	if f := l.top(); f != nil {
		l.lifecycle(f, Shown)
	}
	l.invalidateAll(eink.UI)
}

func (l *Loop) lifecycle(f *frame, ev Lifecycle) {
	owner, scr := f.owner, f.screen
	ctx := l.ctxFor(owner)
	l.safe(owner, func() { scr.Handle(ctx, ev) })
}

// setRotation поворачивает экран; все экраны разложатся заново, когда их покажут.
func (l *Loop) setRotation(r device.Rotation) bool {
	if r == l.disp.Rotation() {
		return false
	}
	if err := l.disp.SetRotation(r); err != nil {
		l.log.Error("поворот экрана", "rotation", r, "err", err)
		return false
	}
	l.allocBuffer()
	reset := func(fs []frame) {
		for i := range fs {
			fs[i].laid = image.Rectangle{}
		}
	}
	reset(l.shell)
	for _, a := range l.apps {
		reset(a.stack)
	}
	for _, ov := range l.overlays {
		ov.o.Layout(l.theme, l.disp.Bounds())
	}
	return true
}
