// Package ui — виджеты для e-ink: тема с размерами в миллиметрах, распознавание жестов,
// страница с откликом на нажатие, кнопки, плитки, ползунки, сетка со страницами, панели и диалоги.
// Размеры и правила — из docs/rules/Правила интерфейса.md.
package ui

import (
	"image"

	"el-book/sdk/eink"
	"el-book/sdk/gfx"
)

// Host — то, что виджетам нужно от окружения. Его реализует app.Context.
type Host interface {
	Invalidate(r image.Rectangle, c eink.Class)
	Theme() *Theme
}

// Theme — шрифты и размеры интерфейса, посчитанные по плотности экрана.
type Theme struct {
	DPI   int
	Fonts *gfx.Fonts
}

// NewTheme создаёт тему для экрана с плотностью dpi.
func NewTheme(dpi int, fonts *gfx.Fonts) *Theme {
	return &Theme{DPI: dpi, Fonts: fonts}
}

// MM переводит миллиметры в пиксели.
func (t *Theme) MM(mm float64) int { return gfx.PxFromMM(mm, t.DPI) }

// PT переводит пункты в пиксели.
func (t *Theme) PT(pt float64) int { return gfx.PxFromPt(pt, t.DPI) }

// Face — шрифт начертания w кеглем pt пунктов.
func (t *Theme) Face(w gfx.Weight, pt float64) *gfx.Face { return t.Fonts.Face(w, t.PT(pt)) }

// Готовые шрифты: подписи не мельче 9 pt, основной текст — 11 pt, заголовки — от 14 pt.
func (t *Theme) Caption() *gfx.Face { return t.Face(gfx.Regular, 9) }
func (t *Theme) Body() *gfx.Face    { return t.Face(gfx.Regular, 11) }
func (t *Theme) Strong() *gfx.Face  { return t.Face(gfx.Bold, 11) }
func (t *Theme) Title() *gfx.Face   { return t.Face(gfx.Bold, 14) }
func (t *Theme) Header() *gfx.Face  { return t.Face(gfx.Bold, 20) }

// Touch — высота основной цели касания (9 мм).
func (t *Theme) Touch() int { return t.MM(9) }

// TouchMin — минимальная цель касания (7 мм).
func (t *Theme) TouchMin() int { return t.MM(7) }

// Gap — промежуток между целями (2 мм).
func (t *Theme) Gap() int { return t.MM(2) }

// Pad — отступ от края экрана (3 мм).
func (t *Theme) Pad() int { return t.MM(3) }

// Radius — скругление кнопок и плашек.
func (t *Theme) Radius() int { return t.MM(1.5) }

// Line — толщина линий рамок.
func (t *Theme) Line() int { return max(t.MM(0.35), 2) }

// IconSize — размер иконки на кнопке.
func (t *Theme) IconSize() int { return t.MM(5.5) }
