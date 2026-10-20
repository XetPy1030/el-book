// Package draw — рисовалка: тетради со страницами, кисти (ручка, перо, маркер, карандаш,
// набросок, паутина, мех), ластик по площади и штрихами, лассо, фигуры по удержанию пальца,
// оттенки серого, разметка листа, отмена и повтор, автосохранение, выгрузка в PNG, SVG и PDF,
// экран сна с последним рисунком. Штрихи рисует sdk/ink.
package draw

import (
	"el-book/sdk/app"
	"el-book/sdk/gfx"
	"el-book/sdk/icons"
	"el-book/sdk/ink"
	"el-book/sdk/kit"
	"el-book/sdk/storage"
	"el-book/sdk/ui"
)

// App — приложение «Рисовалка».
type App struct {
	hinted bool // подсказка о кнопках листания уже показана в этом запуске
	prefs  *kit.Prefs[prefs]
}

// New создаёт приложение.
func New() *App { return &App{} }

func (a *App) Manifest() app.Manifest {
	return app.Manifest{ID: "draw", Title: "Рисовалка", Icon: icons.Pencil}
}

func (a *App) Start(ctx *app.Context) app.Screen {
	a.prefs = kit.LoadPrefs(ctx.Storage(), prefsVersion, defaultPrefs())
	a.prefs.Value.fill()
	return newGallery(a)
}

// prefsVersion — версия схемы настроек рисовалки.
const prefsVersion = 1

// eraseMode — как стирает ластик.
type eraseMode string

const (
	eraseArea   eraseMode = "area"   // по площади, как резинка
	eraseStroke eraseMode = "stroke" // штрихами целиком
)

// prefs — инструменты, которые рисовалка помнит между запусками.
type prefs struct {
	Brush    ink.Brush             `json:"brush"`
	Widths   map[ink.Brush]float64 `json:"widths_mm"` // толщина кисти, мм
	Tones    map[ink.Brush]uint8   `json:"tones"`     // цвет кисти: 0 — чёрный
	Eraser   eraseMode             `json:"eraser"`
	EraserMM float64               `json:"eraser_mm"`
	Shapes   bool                  `json:"shapes"`   // узнавать фигуры в каждом штрихе
	Template Template              `json:"template"` // разметка новых тетрадей
}

func defaultPrefs() prefs {
	return prefs{Brush: ink.Pen, Eraser: eraseArea, EraserMM: 5, Template: Plain}
}

// defaultWidth — толщина кисти по умолчанию, мм.
func defaultWidth(b ink.Brush) float64 {
	switch b {
	case ink.Marker:
		return 5
	case ink.Pencil:
		return 0.6
	case ink.Fountain:
		return 1
	case ink.Sketch, ink.Web, ink.Fur:
		return 0.3
	}
	return 0.6
}

// defaultTone — цвет кисти по умолчанию.
func defaultTone(b ink.Brush) uint8 {
	switch b {
	case ink.Marker:
		return tones[2]
	case ink.Pencil:
		return tones[1]
	}
	return tones[0]
}

// fill дописывает значения, которых нет в сохранённых настройках.
func (p *prefs) fill() {
	if p.Widths == nil {
		p.Widths = map[ink.Brush]float64{}
	}
	if p.Tones == nil {
		p.Tones = map[ink.Brush]uint8{}
	}
	for _, b := range ink.Brushes {
		if _, ok := p.Widths[b]; !ok {
			p.Widths[b] = defaultWidth(b)
		}
		if _, ok := p.Tones[b]; !ok {
			p.Tones[b] = defaultTone(b)
		}
	}
	if p.Eraser != eraseStroke {
		p.Eraser = eraseArea
	}
	if p.EraserMM <= 0 {
		p.EraserMM = 5
	}
	if p.Template == "" {
		p.Template = Plain
	}
}

func (a *App) save(ctx *app.Context) {
	if err := a.prefs.Save(); err != nil {
		ctx.Log().Error("настройки рисовалки", "err", err)
	}
}

// widths — толщины на выбор, мм.
var widths = []float64{0.3, 0.6, 1, 1.8, 3, 5, 8}

// tones — цвета на выбор: от чёрного до белого.
var tones = []uint8{0, 85, 150, 205, 255}

var toneNames = []string{"Чёрный", "Тёмный", "Серый", "Светлый", "Белый"}

// PaintSleep рисует экран сна: открытую страницу последней тетради. Оболочка зовёт его,
// если в настройках сна выбрана «Рисовалка».
func (a *App) PaintSleep(st *storage.App, c *gfx.Canvas, t *ui.Theme) bool {
	return paintSleep(st, c, t)
}
