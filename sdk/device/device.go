// Package device описывает устройство, на котором работает платформа: экран e-ink, ввод,
// подсветку, батарею и сон. Реализаций две: sdk/device/kobo для книги и sdk/device/emu
// для разработки на компьютере. Остальной код видит только эти интерфейсы.
package device

import (
	"errors"
	"image"
	"time"
)

// Device — всё устройство целиком.
type Device interface {
	Info() Info
	Display() Display
	// Events — единый поток событий: касания, кнопки, источник питания, ориентация.
	// Канал закрывается при Close.
	Events() <-chan Event
	// Light возвращает nil, если подсветки нет.
	Light() Light
	Battery() Battery
	Power() Power
	Close() error
}

// Describer — устройство, которое умеет рассказать о себе подробнее: для диагностики.
type Describer interface {
	Describe() []Fact
}

// Fact — строка сведений об устройстве.
type Fact struct {
	Name, Value string
}

// Info — что известно о модели.
type Info struct {
	Model    string // «Kobo Forma»
	Codename string // «frost»
	Firmware string // «4.38.23171»; пусто, если неизвестна
}

// Rotation — ориентация содержимого относительно портрета (книга стоит логотипом вниз),
// в четвертях оборота по часовой стрелке.
type Rotation int

const (
	Portrait        Rotation = 0
	LandscapeCW     Rotation = 1
	PortraitFlipped Rotation = 2
	LandscapeCCW    Rotation = 3
)

// Landscape — альбомная ли ориентация.
func (r Rotation) Landscape() bool { return r%2 == 1 }

func (r Rotation) String() string {
	switch r {
	case Portrait:
		return "портрет"
	case LandscapeCW:
		return "альбом по часовой"
	case PortraitFlipped:
		return "портрет перевёрнутый"
	case LandscapeCCW:
		return "альбом против часовой"
	}
	return "поворот?"
}

// Waveform — способ, которым контроллер e-ink перерисовывает пиксели.
type Waveform int

const (
	WaveformAuto  Waveform = iota // выбирает контроллер
	WaveformDU                    // 2 оттенка, быстро: отклик на нажатие
	WaveformA2                    // 2 оттенка, быстрее всех, больше следов
	WaveformGL16                  // 16 оттенков, только изменившиеся пиксели: интерфейс
	WaveformREAGL                 // GL16 с подавлением следов: смена страниц
	WaveformGC16                  // 16 оттенков, все пиксели: картинки и очистка
)

func (w Waveform) String() string {
	switch w {
	case WaveformAuto:
		return "AUTO"
	case WaveformDU:
		return "DU"
	case WaveformA2:
		return "A2"
	case WaveformGL16:
		return "GL16"
	case WaveformREAGL:
		return "REAGL"
	case WaveformGC16:
		return "GC16"
	}
	return "WF?"
}

// Update — запрос на обновление области панели.
type Update struct {
	Rect     image.Rectangle // в логических координатах экрана
	Waveform Waveform
	Full     bool // полное обновление: перегнать все пиксели, с миганием
}

// Marker — номер запущенного обновления; по нему ждут окончания.
type Marker uint32

// Display — экран e-ink.
type Display interface {
	// Bounds — логический размер экрана с учётом поворота; начало всегда (0, 0).
	Bounds() image.Rectangle
	DPI() int
	Rotation() Rotation
	// SetRotation поворачивает экран; Bounds после этого может поменяться местами сторон.
	SetRotation(Rotation) error
	// Present копирует область u.Rect из src (размером Bounds) на панель и запускает обновление.
	// Возвращается сразу, не дожидаясь конца обновления.
	Present(src *image.Gray, u Update) (Marker, error)
	// Wait ждёт окончания обновления с маркером m.
	Wait(m Marker) error
}

// Light — подсветка. Значения — проценты 0…100.
type Light interface {
	Brightness() int
	SetBrightness(percent int) error
	// HasWarmth сообщает, умеет ли подсветка менять теплоту.
	HasWarmth() bool
	// Warmth: 0 — холодный свет, 100 — самый тёплый.
	Warmth() int
	SetWarmth(percent int) error
}

// Battery — батарея.
type Battery interface {
	// Level — заряд в процентах.
	Level() (int, error)
	Charging() (bool, error)
}

// Power — управление питанием.
type Power interface {
	// Suspend усыпляет устройство и возвращается, когда его разбудил пользователь:
	// кнопкой питания, открытой обложкой или кабелем от компьютера. Ложные пробуждения
	// реализация гасит сама. Перед вызовом экран сна уже должен быть показан и дорисован.
	Suspend() error
}

// ErrSuspendCancelled — сон не состоялся: пока устройство готовилось, пришло событие
// пробуждения. Это не сбой: интерфейс просто продолжает работу.
var ErrSuspendCancelled = errors.New("сон отменён событием пробуждения")

// Event — событие устройства: TouchEvent, KeyEvent, PowerSourceEvent или OrientationEvent.
type Event interface {
	EventTime() time.Time
}

// TouchPhase — фаза касания.
type TouchPhase int

const (
	TouchDown TouchPhase = iota
	TouchMove
	TouchUp
)

func (p TouchPhase) String() string {
	switch p {
	case TouchDown:
		return "down"
	case TouchMove:
		return "move"
	case TouchUp:
		return "up"
	}
	return "phase?"
}

// TouchEvent — палец коснулся, сдвинулся или поднялся. Координаты — логические, уже с учётом поворота.
type TouchEvent struct {
	Time  time.Time
	Slot  int // номер пальца; первый палец — 0
	Phase TouchPhase
	X, Y  int
}

func (e TouchEvent) EventTime() time.Time { return e.Time }

// Point возвращает координаты касания.
func (e TouchEvent) Point() image.Point { return image.Pt(e.X, e.Y) }

// Key — физическая кнопка или датчик.
type Key int

const (
	KeyUnknown Key = iota
	KeyPower
	KeySleepCover // нажата — обложка закрыта
	KeyPageForward
	KeyPageBack
	KeyHome
)

func (k Key) String() string {
	switch k {
	case KeyPower:
		return "power"
	case KeySleepCover:
		return "cover"
	case KeyPageForward:
		return "pgfwd"
	case KeyPageBack:
		return "pgback"
	case KeyHome:
		return "home"
	}
	return "key?"
}

// KeyEvent — кнопка нажата или отпущена.
type KeyEvent struct {
	Time    time.Time
	Key     Key
	Pressed bool
}

func (e KeyEvent) EventTime() time.Time { return e.Time }

// PowerSource — откуда приходит питание.
type PowerSource int

const (
	SourceComputer PowerSource = iota // кабель в компьютере
	SourceCharger                     // зарядное устройство
)

func (s PowerSource) String() string {
	if s == SourceCharger {
		return "зарядка"
	}
	return "компьютер"
}

// PowerSourceEvent — кабель подключили или отключили.
type PowerSourceEvent struct {
	Time      time.Time
	Source    PowerSource
	Connected bool
}

func (e PowerSourceEvent) EventTime() time.Time { return e.Time }

// OrientationEvent — датчик сообщил новое положение книги.
type OrientationEvent struct {
	Time     time.Time
	Rotation Rotation
}

func (e OrientationEvent) EventTime() time.Time { return e.Time }
