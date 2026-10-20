package emu

import (
	"context"
	"strings"
	"sync"
	"time"

	"el-book/sdk/device"
)

// network — Wi-Fi эмулятора. Состояния и поиск сетей имитируются, а сами запросы идут
// через сеть компьютера. Режим «нет сети» (SetOffline) проверяет, как приложения живут
// без интернета.
type network struct {
	e       *Emulator
	mu      sync.Mutex
	status  device.NetStatus
	offline bool
	saved   map[string]string // ssid → пароль
	delay   time.Duration
}

// knownAPs — сети «вокруг» эмулированной книги.
var knownAPs = []device.AccessPoint{
	{SSID: "Дом", Signal: 82, Secure: true},
	{SSID: "Кофейня", Signal: 55},
	{SSID: "Сосед_5G", Signal: 31, Secure: true},
}

func newNetwork(e *Emulator) *network {
	delay := 1200 * time.Millisecond
	if e.opts.AutoWake {
		delay = 0 // скрипты и тесты не ждут
	}
	return &network{e: e, saved: map[string]string{"Дом": "password"}, delay: delay}
}

// Network — Wi-Fi эмулятора.
func (e *Emulator) Network() device.Network { return e.net }

// SetOffline включает режим «нет сети»: Up не находит знакомых сетей.
func (e *Emulator) SetOffline(off bool) {
	n := e.net
	n.mu.Lock()
	n.offline = off
	n.mu.Unlock()
	if off {
		_ = n.Down()
	}
	e.broadcastState()
}

func (n *network) Status() device.NetStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.status
}

func (n *network) set(s device.NetStatus) {
	n.mu.Lock()
	changed := s != n.status
	n.status = s
	n.mu.Unlock()
	if changed {
		n.e.send(device.NetworkEvent{Time: time.Now(), Status: s})
		n.e.broadcastState()
	}
}

func (n *network) wait(ctx context.Context) error {
	if n.delay == 0 {
		return ctx.Err()
	}
	t := time.NewTimer(n.delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (n *network) Up(ctx context.Context) error {
	st := n.Status()
	if st.State == device.NetOnline {
		return nil
	}
	n.mu.Lock()
	offline := n.offline
	n.mu.Unlock()
	n.set(device.NetStatus{State: device.NetConnecting})
	if err := n.wait(ctx); err != nil {
		n.set(device.NetStatus{State: device.NetFailed, Err: err.Error()})
		return err
	}
	if offline {
		n.set(device.NetStatus{State: device.NetFailed, Err: device.ErrNoKnownNet.Error()})
		return device.ErrNoKnownNet
	}
	n.set(device.NetStatus{State: device.NetOnline, SSID: "Дом", Addr: "192.168.1.42", Signal: 82})
	return nil
}

func (n *network) Down() error {
	n.set(device.NetStatus{State: device.NetOff})
	return nil
}

func (n *network) Scan(ctx context.Context) ([]device.AccessPoint, error) {
	if err := n.wait(ctx); err != nil {
		return nil, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.offline {
		return nil, nil
	}
	out := make([]device.AccessPoint, len(knownAPs))
	for i, ap := range knownAPs {
		_, ap.Saved = n.saved[ap.SSID]
		out[i] = ap
	}
	return out, nil
}

// Connect принимает любой пароль, где нет «wrong»: так проверяется неверный пароль.
func (n *network) Connect(ctx context.Context, ssid, psk string) error {
	n.set(device.NetStatus{State: device.NetConnecting, SSID: ssid})
	if err := n.wait(ctx); err != nil {
		return err
	}
	if strings.Contains(psk, "wrong") {
		n.set(device.NetStatus{State: device.NetFailed, SSID: ssid, Err: device.ErrWrongPass.Error()})
		return device.ErrWrongPass
	}
	n.mu.Lock()
	n.saved[ssid] = psk
	n.mu.Unlock()
	n.set(device.NetStatus{State: device.NetOnline, SSID: ssid, Addr: "192.168.1.42", Signal: 70})
	return nil
}

func (n *network) Forget(ssid string) error {
	n.mu.Lock()
	delete(n.saved, ssid)
	n.mu.Unlock()
	return nil
}
