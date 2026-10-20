package kit

import (
	"image"
	"strings"

	"el-book/sdk/app"
	"el-book/sdk/device"
	"el-book/sdk/eink"
	"el-book/sdk/gfx"
	"el-book/sdk/ui"
)

// PromptOptions — что спросить у пользователя.
type PromptOptions struct {
	Title       string // заголовок экрана: «Название рисунка»
	Hint        string // пояснение над полем
	Initial     string // текст в поле при открытии
	Placeholder string // серая подсказка в пустом поле
	OK          string // подпись кнопки; по умолчанию «Готово»
	Secret      bool   // пароль: точки вместо букв
	English     bool   // начать с латиницы: адреса, пароли
	NoCaps      bool   // не начинать с заглавной
	Multiline   bool   // несколько строк: клавиша ввода переводит строку
	AllowEmpty  bool   // можно подтвердить пустой текст
	// Error — сообщение под полем при открытии: «Пароль не подошёл» при повторном вводе.
	Error string
	// Check проверяет текст перед done: непустой ответ — что не так. Экран остаётся
	// открытым, ответ показывается под полем.
	Check func(text string) string
}

// Prompt открывает экран ввода текста с клавиатурой. done получает текст после «Готово»;
// «назад» закрывает экран без done.
func Prompt(ctx *app.Context, o PromptOptions, done func(text string)) {
	if o.OK == "" {
		o.OK = "Готово"
	}
	ctx.Push(&promptScreen{o: o, done: done})
}

type promptScreen struct {
	o     PromptOptions
	done  func(string)
	page  ui.Page
	field *ui.TextField
	area  *ui.TextArea
	kb    *ui.Keyboard
	pager *ui.Label
	err   *ui.Label // что не так с текстом: сообщения не закрывают клавиатуру, как тост
}

func (p *promptScreen) target() ui.KeyTarget {
	if p.area != nil {
		return p.area
	}
	return p.field
}

func (p *promptScreen) text() string {
	if p.area != nil {
		return p.area.Text()
	}
	return p.field.Text()
}

func (p *promptScreen) Layout(ctx *app.Context, b image.Rectangle) {
	t := ctx.Theme()
	p.page.Clear()
	if p.field == nil && p.area == nil {
		if p.o.Multiline {
			p.area = ui.NewTextArea(t, 13)
			p.area.Placeholder = p.o.Placeholder
			p.area.SetText(p.o.Initial)
		} else {
			p.field = &ui.TextField{Placeholder: p.o.Placeholder, Secret: p.o.Secret, NoCaps: p.o.NoCaps,
				OnSubmit: func(string) { p.submit(ctx) }}
			p.field.SetText(p.o.Initial)
		}
		p.kb = ui.NewKeyboard(t, p.target())
		p.kb.English = p.o.English
		if p.o.Multiline {
			p.kb.EnterLabel = ""
		} else {
			p.kb.EnterLabel = p.o.OK
		}
		p.err = &ui.Label{Text: p.o.Error, Wrap: true, VAlign: -1, Face: func(t *ui.Theme) *gfx.Face { return t.Strong() }}
		// Текст поправили — сообщение о прежнем больше не о нём.
		edited := func(string) {
			if p.err.Text != "" {
				p.fail(ctx, "")
			}
		}
		if p.area != nil {
			p.area.OnChange = edited
		} else {
			p.field.OnChange = edited
		}
	}
	r := b
	ok := &ui.Button{Text: p.o.OK, Style: ui.Primary, OnTap: func() { p.submit(ctx) }}
	bar := ui.NewTitleBar(t, p.o.Title, func() { ctx.Pop() }, ok)
	bar.SetBounds(ui.CutTop(&r, t.Bar()))
	p.kb.SetBounds(ui.CutBottom(&r, p.kb.Height(t, r.Dx())))
	body := ui.Pad(r, t.Pad(), t.MM(3), t.Pad(), t.MM(2))
	p.page.Add(bar, p.kb)
	if p.o.Hint != "" {
		hint := &ui.Label{Text: p.o.Hint, Wrap: true, VAlign: -1, Color: gfx.Gray16(4)}
		hint.SetBounds(ui.CutTop(&body, hint.Height(t, body.Dx())))
		ui.CutTop(&body, t.MM(2))
		p.page.Add(hint)
	}
	if p.area != nil {
		p.pager = &ui.Label{Align: gfx.AlignRight, Color: gfx.Gray16(5), Face: func(t *ui.Theme) *gfx.Face { return t.Caption() }}
		row := ui.CutBottom(&body, t.Caption().Height)
		p.err.SetBounds(ui.CutLeft(&row, row.Dx()*3/4))
		p.pager.SetBounds(row)
		p.area.OnPage = func(int, int) { p.updatePager(ctx) }
		p.area.SetBounds(body)
		p.updatePager(ctx)
		p.page.Add(p.area, p.pager, p.err)
	} else {
		p.field.SetBounds(ui.CutTop(&body, p.field.Height(t, body.Dx())))
		ui.CutTop(&body, t.MM(2))
		p.err.SetBounds(ui.CutTop(&body, 2*t.Body().Height))
		p.page.Add(p.field, p.err)
	}
	p.kb.SyncCaps(ctx)
}

// fail показывает под полем, что не так с текстом.
func (p *promptScreen) fail(ctx *app.Context, msg string) {
	p.err.Text = msg
	ctx.Invalidate(p.err.R, eink.UI)
}

func (p *promptScreen) updatePager(ctx *app.Context) {
	if p.pager == nil {
		return
	}
	p.pager.Text = p.area.PageText()
	ctx.Invalidate(p.pager.R, eink.UI)
}

func (p *promptScreen) submit(ctx *app.Context) {
	text := p.text()
	if !p.o.AllowEmpty && strings.TrimSpace(text) == "" {
		p.fail(ctx, "Введите текст")
		return
	}
	if p.o.Check != nil {
		if msg := p.o.Check(text); msg != "" {
			p.fail(ctx, msg)
			return
		}
	}
	ctx.Pop()
	if p.done != nil {
		p.done(text)
	}
}

func (p *promptScreen) Draw(c *gfx.Canvas, t *ui.Theme) { p.page.Draw(c, t) }

func (p *promptScreen) Handle(ctx *app.Context, ev app.Event) {
	if k, ok := ev.(device.KeyEvent); ok && p.area != nil {
		if k.Pressed && k.Key == device.KeyPageForward {
			p.area.Next(ctx)
		} else if k.Pressed && k.Key == device.KeyPageBack {
			p.area.Prev(ctx)
		}
		return
	}
	app.RoutePage(ctx, &p.page, ev)
}
