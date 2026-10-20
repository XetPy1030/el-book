// Package eink решает, как обновлять панель: приложение сообщает, какая область и почему
// изменилась (класс изменения), а Policy выбирает waveform и режим, Scheduler отправляет
// обновления и ждёт только тех предыдущих, что мешают новому.
package eink

import (
	"image"
	"sort"
	"time"

	"el-book/sdk/device"
)

// Class — почему изменилась область. Приложения выбирают класс, а не waveform.
type Class int

const (
	Feedback Class = iota // отклик на нажатие: инверсия кнопки
	UI                    // текст и элементы управления
	Content               // картинка, полностраничное содержимое
	Stroke                // штрих пера, перетаскивание
	Clean                 // очистка всего экрана с миганием
)

func (c Class) String() string {
	switch c {
	case Feedback:
		return "отклик"
	case UI:
		return "интерфейс"
	case Content:
		return "контент"
	case Stroke:
		return "штрих"
	case Clean:
		return "очистка"
	}
	return "класс?"
}

// Damage — изменившаяся область и причина.
type Damage struct {
	Rect  image.Rectangle
	Class Class
}

// Policy переводит повреждения в обновления панели.
type Policy struct {
	// FlashEvery — после стольких крупных частичных обновлений (больше половины экрана)
	// следующее крупное делается полным, чтобы убрать накопившиеся следы. 0 — никогда.
	FlashEvery int
	// StrokeWaveform — режим для штрихов. A2 вдвое быстрее DU, но показывает только чёрное
	// и белое: цикл событий отправляет такие области двухцветной копией с дизерингом, а
	// оттенки возвращает чистовая перерисовка приложения.
	StrokeWaveform device.Waveform
	// StrokeInterval — штрихи уходят на панель не чаще раза в этот срок, области за срок
	// объединяются. Каждое движение пальца отдельным обновлением перегружает контроллер
	// e-ink, и след отстаёт. 0 — без задержки.
	StrokeInterval time.Duration

	majors int
}

// NewPolicy возвращает политику с настройками по умолчанию.
func NewPolicy() *Policy {
	return &Policy{FlashEvery: 8, StrokeWaveform: device.WaveformA2, StrokeInterval: 40 * time.Millisecond}
}

// Plan превращает повреждения экрана screen в обновления панели. Порядок результата —
// порядок отправки: сначала быстрые (отклик, штрих), потом интерфейс и контент.
func (p *Policy) Plan(screen image.Rectangle, damage []Damage) []device.Update {
	byClass := map[Class][]image.Rectangle{}
	for _, d := range damage {
		r := d.Rect.Intersect(screen)
		if r.Empty() {
			continue
		}
		if d.Class == Clean {
			p.majors = 0
			return []device.Update{{Rect: screen, Waveform: device.WaveformGC16, Full: true}}
		}
		byClass[d.Class] = append(byClass[d.Class], r)
	}
	ui := Merge(byClass[UI])
	content := Merge(byClass[Content])
	// Отклик и штрих внутри области, которая всё равно перерисуется полностью, не нужны:
	// быстрое обновление только задержало бы её, ведь перекрытия разных режимов ждут друг друга.
	covered := func(r image.Rectangle) bool { return coveredBy(r, ui) || coveredBy(r, content) }

	var out []device.Update
	for _, r := range Merge(byClass[Feedback]) {
		if !covered(r) {
			out = append(out, device.Update{Rect: r, Waveform: device.WaveformDU})
		}
	}
	for _, r := range Merge(byClass[Stroke]) {
		if !covered(r) {
			out = append(out, device.Update{Rect: r, Waveform: p.StrokeWaveform})
		}
	}
	half := screen.Dx() * screen.Dy() / 2
	add := func(r image.Rectangle, wf device.Waveform) {
		u := device.Update{Rect: r, Waveform: wf}
		if r.Dx()*r.Dy() >= half {
			p.majors++
			if p.FlashEvery > 0 && p.majors >= p.FlashEvery {
				p.majors = 0
				u = device.Update{Rect: r, Waveform: device.WaveformGC16, Full: true}
			}
		}
		out = append(out, u)
	}
	for _, r := range ui {
		add(r, device.WaveformGL16)
	}
	for _, r := range content {
		add(r, device.WaveformGC16)
	}
	return out
}

func coveredBy(r image.Rectangle, rs []image.Rectangle) bool {
	for _, o := range rs {
		if r.In(o) {
			return true
		}
	}
	return false
}

// Merge объединяет пересекающиеся и соседние прямоугольники, а также те, чьё общее
// охватывающее не намного больше суммы площадей: меньше мелких обновлений.
func Merge(rs []image.Rectangle) []image.Rectangle {
	out := make([]image.Rectangle, 0, len(rs))
	for _, r := range rs {
		if !r.Empty() {
			out = append(out, r)
		}
	}
	for changed := true; changed; {
		changed = false
		for i := 0; i < len(out) && !changed; i++ {
			for j := i + 1; j < len(out); j++ {
				if worthMerging(out[i], out[j]) {
					out[i] = out[i].Union(out[j])
					out = append(out[:j], out[j+1:]...)
					changed = true
					break
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Min.Y != out[j].Min.Y {
			return out[i].Min.Y < out[j].Min.Y
		}
		return out[i].Min.X < out[j].Min.X
	})
	return out
}

func worthMerging(a, b image.Rectangle) bool {
	if a.Overlaps(b) || adjacent(a, b) {
		return true
	}
	u := a.Union(b)
	return area(u) <= (area(a)+area(b))*5/4
}

func adjacent(a, b image.Rectangle) bool {
	return a.Inset(-1).Overlaps(b)
}

func area(r image.Rectangle) int { return r.Dx() * r.Dy() }

// Durations — сколько примерно длится обновление каждым режимом. После этого времени
// обновление считается законченным, и Scheduler его не ждёт.
type Durations map[device.Waveform]time.Duration

// DefaultDurations — длительности на Kobo Forma: самый долгий из четырёх замеров экрана
// «Диагностика» 20.10.2026 (DU 288, A2 149, GL16 375, REAGL 378, GC16 481 мс) с запасом.
func DefaultDurations() Durations {
	return Durations{
		device.WaveformDU:    300 * time.Millisecond,
		device.WaveformA2:    160 * time.Millisecond,
		device.WaveformGL16:  400 * time.Millisecond,
		device.WaveformREAGL: 400 * time.Millisecond,
		device.WaveformGC16:  500 * time.Millisecond,
		device.WaveformAuto:  500 * time.Millisecond,
	}
}

type pending struct {
	marker   device.Marker
	update   device.Update
	deadline time.Time
}

// Scheduler отправляет обновления на дисплей. Перед обновлением он ждёт только те
// незаконченные, что пересекаются с ним и идут другим режимом или полностью:
// обновления разных областей идут параллельно, штрихи одним режимом — без ожидания.
type Scheduler struct {
	d         device.Display
	durations Durations
	now       func() time.Time
	pending   []pending
	// Stats — счётчики для журнала и эмулятора.
	Stats struct {
		Updates int
		Waits   int
	}
}

// NewScheduler создаёт планировщик для дисплея d.
func NewScheduler(d device.Display, durations Durations) *Scheduler {
	if durations == nil {
		durations = DefaultDurations()
	}
	return &Scheduler{d: d, durations: durations, now: time.Now}
}

// Present отправляет одно обновление.
func (s *Scheduler) Present(src *image.Gray, u device.Update) error {
	now := s.now()
	kept := s.pending[:0]
	for _, p := range s.pending {
		if now.Before(p.deadline) {
			kept = append(kept, p)
		}
	}
	s.pending = kept
	kept = s.pending[:0]
	for _, p := range s.pending {
		conflict := p.update.Rect.Overlaps(u.Rect) &&
			(p.update.Waveform != u.Waveform || p.update.Full || u.Full)
		if conflict {
			s.Stats.Waits++
			if err := s.d.Wait(p.marker); err != nil {
				return err
			}
			continue
		}
		kept = append(kept, p)
	}
	s.pending = kept
	m, err := s.d.Present(src, u)
	if err != nil {
		return err
	}
	s.Stats.Updates++
	d := s.durations[u.Waveform]
	if u.Full {
		d += d / 4
	}
	s.pending = append(s.pending, pending{marker: m, update: u, deadline: s.now().Add(d)})
	return nil
}

// WaitAll ждёт окончания всех незаконченных обновлений: перед сном и выходом.
func (s *Scheduler) WaitAll() error {
	for _, p := range s.pending {
		if err := s.d.Wait(p.marker); err != nil {
			return err
		}
	}
	s.pending = s.pending[:0]
	return nil
}
