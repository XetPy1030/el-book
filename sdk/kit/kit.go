// Package kit — готовые экраны и помощники для приложений поверх sdk/app и sdk/ui: то, что
// иначе повторялось бы в каждом приложении. Загрузка в фоне с результатом в цикле интерфейса,
// настройки со значениями по умолчанию, экран ввода текста, экран-список.
package kit

import (
	"context"

	"el-book/sdk/app"
	"el-book/sdk/storage"
)

// Load выполняет fetch в фоне и передаёт результат done в горутине интерфейса. Паника в
// fetch закрывает приложение, а не процесс; закрыли приложение — done не вызывается.
func Load[T any](ctx *app.Context, fetch func(context.Context) (T, error), done func(T, error)) {
	ctx.Go(func(bg context.Context) {
		v, err := fetch(bg)
		ctx.Post(func() { done(v, err) })
	})
}

// Prefs — настройки приложения со значениями по умолчанию. Поля, которых нет в сохранённом
// файле (их добавили в новой версии), остаются по умолчанию — миграция для простых случаев
// не нужна.
type Prefs[T any] struct {
	Value   T
	st      *storage.App
	version int
}

// LoadPrefs читает настройки приложения поверх defaults. Нет файла или он испорчен целиком —
// остаются значения по умолчанию.
func LoadPrefs[T any](st *storage.App, version int, defaults T) *Prefs[T] {
	p := &Prefs[T]{Value: defaults, st: st, version: version}
	_, _ = st.LoadSettings(&p.Value)
	return p
}

// Save записывает настройки.
func (p *Prefs[T]) Save() error { return p.st.SaveSettings(p.version, p.Value) }
