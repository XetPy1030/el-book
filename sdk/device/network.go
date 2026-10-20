package device

import (
	"context"
	"errors"
	"time"
)

// Networker — устройство с Wi-Fi. Сеть — необязательная часть устройства: у реализации
// без Wi-Fi этого метода нет.
type Networker interface {
	Network() Network
}

// Network — Wi-Fi. Методы можно вызывать из любой горутины; Up, Scan и Connect блокируются
// и поэтому вызываются только из фоновой работы, не из цикла интерфейса.
type Network interface {
	// Status — текущее состояние.
	Status() NetStatus
	// Up включает Wi-Fi и ждёт подключения к одной из сохранённых сетей и адреса.
	// Уже подключённая сеть возвращает nil сразу. ctx ограничивает ожидание.
	Up(ctx context.Context) error
	// Down выключает Wi-Fi целиком: модуль, питание чипа.
	Down() error
	// Scan ищет сети поблизости; включает Wi-Fi, если он выключен.
	Scan(ctx context.Context) ([]AccessPoint, error)
	// Connect подключается к сети ssid с паролем psk (пусто — открытая сеть) и запоминает её.
	Connect(ctx context.Context, ssid, psk string) error
	// Forget забывает сеть, сохранённую через Connect.
	Forget(ssid string) error
}

// NetState — состояние Wi-Fi.
type NetState int

const (
	NetOff        NetState = iota // выключен
	NetConnecting                 // включается, подключается, получает адрес
	NetOnline                     // подключён, адрес есть
	NetFailed                     // включался, но подключиться не удалось
)

func (s NetState) String() string {
	switch s {
	case NetOff:
		return "выключен"
	case NetConnecting:
		return "подключается"
	case NetOnline:
		return "подключён"
	case NetFailed:
		return "ошибка"
	}
	return "сеть?"
}

// NetStatus — состояние Wi-Fi и подробности для статус-бара и настроек.
type NetStatus struct {
	State  NetState
	SSID   string // сеть, к которой подключены или подключаемся
	Addr   string // адрес в сети
	Signal int    // уровень сигнала 0…100; 0 — неизвестен
	Err    string // почему не подключились: для пользователя
}

// AccessPoint — сеть, найденная поиском.
type AccessPoint struct {
	SSID   string
	Signal int  // 0…100
	Secure bool // нужен пароль
	Saved  bool // сеть сохранена: подключится без пароля
}

// NetworkEvent — состояние Wi-Fi изменилось. Устройство кладёт его в поток событий.
type NetworkEvent struct {
	Time   time.Time
	Status NetStatus
}

func (e NetworkEvent) EventTime() time.Time { return e.Time }

// Ошибки сети, понятные пользователю.
var (
	ErrNoWifi       = errors.New("на книге нет Wi-Fi")
	ErrNoKnownNet   = errors.New("рядом нет сохранённых сетей Wi-Fi")
	ErrWrongPass    = errors.New("неверный пароль сети")
	ErrNetTimeout   = errors.New("сеть не ответила вовремя")
	ErrWifiDisabled = errors.New("Wi-Fi выключен в настройках")
)
