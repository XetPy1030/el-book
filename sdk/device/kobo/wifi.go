package kobo

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"el-book/sdk/device"
	"el-book/sdk/storage"
)

// wifi — Wi-Fi книги без Nickel. Питание чипа, модуль и wpa_supplicant с конфигом сетей
// Nickel поднимает scripts/device/wifi.sh; подключение и поиск идут через управляющий сокет
// wpa_supplicant (wpaConn); адрес получает тот же скрипт. Сети, добавленные в оболочке,
// хранятся отдельно — имя и ключ PSK, не пароль — и добавляются в wpa_supplicant при каждом
// включении: конфиг Nickel не меняется.
type wifi struct {
	run          func(ctx context.Context, cmd string) error // wifi.sh <cmd>
	ctrlDir      string                                      // каталог сокетов wpa_supplicant
	tmpDir       string                                      // где лежит сокет клиента
	iface        string
	file         string // сети оболочки: networks.json
	send         func(device.Event)
	log          *slog.Logger
	poll         time.Duration // пауза между опросами состояния
	assocTimeout time.Duration // сколько ждать подключения к сети

	op     sync.Mutex // Up, Down, Scan, Connect — по одному
	mu     sync.Mutex
	status device.NetStatus
	conn   *wpaConn
	cancel context.CancelFunc // прерывает текущую операцию: Down не ждёт долгого Up
}

// savedNet — сеть, добавленная в оболочке. PSK — ключ в шестнадцатеричном виде; пусто —
// открытая сеть.
type savedNet struct {
	SSID string `json:"ssid"`
	PSK  string `json:"psk,omitempty"`
}

// wifiLogLimit — размер журнала скрипта, после которого он уходит в .1.
const wifiLogLimit = 256 << 10

// newWifi создаёт Wi-Fi платформы в папке dir: скрипт dir/wifi.sh, сети оболочки
// в dir/wifi, вывод скрипта — в dir/logs/wifi.log.
func newWifi(dir, iface string, send func(device.Event), log *slog.Logger) *wifi {
	script := filepath.Join(dir, "wifi.sh")
	logPath := filepath.Join(dir, "logs", "wifi.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > wifiLogLimit {
		_ = os.Rename(logPath, logPath+".1")
	}
	return &wifi{
		run:     func(ctx context.Context, cmd string) error { return runScript(ctx, script, cmd, logPath) },
		ctrlDir: "/var/run/wpa_supplicant", tmpDir: "/tmp", iface: iface,
		file: filepath.Join(dir, "wifi", "networks.json"),
		send: send, log: log, poll: 500 * time.Millisecond, assocTimeout: 20 * time.Second,
	}
}

// runScript запускает «sh script cmd». Вывод идёт прямо в файл журнала, а не в канал:
// dhcpcd остаётся работать в фоне и держал бы канал открытым.
func runScript(ctx context.Context, script, cmd, logPath string) error {
	c := exec.CommandContext(ctx, "/bin/sh", script, cmd)
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		defer f.Close()
		fmt.Fprintf(f, "%s wifi.sh %s\n", time.Now().Format("2006-01-02 15:04:05"), cmd)
		c.Stdout, c.Stderr = f, f
	}
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && ctx.Err() == nil {
			return scriptError(cmd, exit.ExitCode())
		}
		return fmt.Errorf("wifi.sh %s: %w", cmd, err)
	}
	return nil
}

// scriptError переводит код выхода wifi.sh в понятную ошибку.
func scriptError(cmd string, code int) error {
	switch code {
	case 3:
		return fmt.Errorf("%w: модуль Wi-Fi этой книги не поддерживается", device.ErrNoWifi)
	case 4, 5:
		return fmt.Errorf("Wi-Fi не включился (wifi.sh %s, код %d)", cmd, code)
	case 6:
		return fmt.Errorf("wpa_supplicant не запустился (wifi.sh %s, код %d)", cmd, code)
	case 7:
		return fmt.Errorf("%w: адрес в сети не получен", device.ErrNetTimeout)
	}
	return fmt.Errorf("wifi.sh %s: код %d", cmd, code)
}

func (w *wifi) Status() device.NetStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *wifi) set(s device.NetStatus) {
	w.mu.Lock()
	changed := s != w.status
	w.status = s
	w.mu.Unlock()
	if changed {
		w.log.Info("Wi-Fi", "состояние", s.State.String(), "сеть", s.SSID, "адрес", s.Addr, "ошибка", s.Err)
		w.send(device.NetworkEvent{Time: time.Now(), Status: s})
	}
}

func (w *wifi) fail(ssid string, err error) error {
	msg := err.Error()
	if errors.Is(err, context.Canceled) {
		msg = "подключение прервано"
	}
	w.set(device.NetStatus{State: device.NetFailed, SSID: ssid, Err: msg})
	return err
}

// begin занимает Wi-Fi под операцию; Down может её прервать.
func (w *wifi) begin(ctx context.Context) (context.Context, func()) {
	w.op.Lock()
	ctx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancel = cancel
	w.mu.Unlock()
	return ctx, func() {
		w.mu.Lock()
		w.cancel = nil
		w.mu.Unlock()
		cancel()
		w.op.Unlock()
	}
}

// interrupt прерывает текущую операцию и занимает Wi-Fi.
func (w *wifi) interrupt() {
	w.mu.Lock()
	if w.cancel != nil {
		w.cancel()
	}
	w.mu.Unlock()
	w.op.Lock()
}

// start включает Wi-Fi, если он выключен, подключается к wpa_supplicant и добавляет в него
// сети оболочки. После start wpa_supplicant сам подключается к знакомой сети.
func (w *wifi) start(ctx context.Context) error {
	if w.conn != nil {
		if r, err := w.conn.request("PING", time.Second); err == nil && strings.TrimSpace(r) == "PONG" {
			return nil
		}
		w.conn.Close()
		w.conn = nil
	}
	if err := w.run(ctx, "up"); err != nil {
		return err
	}
	c, err := dialWPA(w.ctrlDir, w.iface, w.tmpDir)
	if err != nil {
		return err
	}
	w.conn = c
	return w.addSaved()
}

// addSaved добавляет в wpa_supplicant сети оболочки, которых в нём ещё нет.
func (w *wifi) addSaved() error {
	saved, err := w.loadSaved()
	if err != nil {
		w.log.Warn("сети оболочки не прочитались", "err", err)
		return nil
	}
	have, err := w.networks()
	if err != nil {
		return err
	}
	for _, s := range saved {
		if _, ok := have[s.SSID]; ok {
			continue
		}
		if _, err := w.addNetwork(s); err != nil {
			w.log.Warn("сеть оболочки не добавилась", "сеть", s.SSID, "err", err)
		}
	}
	return nil
}

// networks — сети wpa_supplicant: имя → номер.
func (w *wifi) networks() (map[string]int, error) {
	reply, err := w.conn.request("LIST_NETWORKS", 3*time.Second)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, n := range parseNetworks(reply) {
		out[n.SSID] = n.ID
	}
	return out, nil
}

func (w *wifi) addNetwork(s savedNet) (int, error) {
	reply, err := w.conn.request("ADD_NETWORK", 3*time.Second)
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(strings.TrimSpace(reply))
	if err != nil {
		return 0, fmt.Errorf("wpa_supplicant ADD_NETWORK: %s", strings.TrimSpace(reply))
	}
	if err := w.configure(id, s); err != nil {
		return 0, err
	}
	return id, w.conn.ok(fmt.Sprintf("ENABLE_NETWORK %d", id))
}

// configure задаёт имя и ключ сети. Имя — в шестнадцатеричном виде: так wpa_supplicant
// принимает любые символы; ключ — 64 шестнадцатеричные цифры без кавычек.
func (w *wifi) configure(id int, s savedNet) error {
	if err := w.conn.ok(fmt.Sprintf("SET_NETWORK %d ssid %s", id, hex.EncodeToString([]byte(s.SSID)))); err != nil {
		return err
	}
	if s.PSK == "" {
		return w.conn.ok(fmt.Sprintf("SET_NETWORK %d key_mgmt NONE", id))
	}
	return w.conn.ok(fmt.Sprintf("SET_NETWORK %d psk %s", id, s.PSK))
}

// waitAssociated ждёт, пока wpa_supplicant подключится к сети, и возвращает её имя.
// Застряли на обмене ключами — неверный пароль.
func (w *wifi) waitAssociated(ctx context.Context) (string, error) {
	deadline := time.Now().Add(w.assocTimeout)
	handshake := false
	for {
		reply, err := w.conn.request("STATUS", 3*time.Second)
		if err != nil {
			return "", err
		}
		st := parseStatus(reply)
		switch st.State {
		case "COMPLETED":
			return st.SSID, nil
		case "4WAY_HANDSHAKE", "GROUP_HANDSHAKE":
			handshake = true
		}
		if time.Now().After(deadline) {
			if handshake {
				return "", device.ErrWrongPass
			}
			return "", w.explainTimeout()
		}
		t := time.NewTimer(w.poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return "", ctx.Err()
		case <-t.C:
		}
	}
}

// explainTimeout — почему не подключились: знакомых сетей рядом нет или сеть молчит.
func (w *wifi) explainTimeout() error {
	have, err := w.networks()
	if err != nil || len(have) == 0 {
		return device.ErrNoKnownNet
	}
	reply, err := w.conn.request("SCAN_RESULTS", 3*time.Second)
	if err != nil {
		return device.ErrNetTimeout
	}
	for _, ap := range parseScan(reply, nil) {
		if _, ok := have[ap.SSID]; ok {
			return device.ErrNetTimeout
		}
	}
	return device.ErrNoKnownNet
}

// online получает адрес и сообщает, что сеть готова.
func (w *wifi) online(ctx context.Context, ssid string) error {
	w.set(device.NetStatus{State: device.NetConnecting, SSID: ssid})
	if err := w.run(ctx, "dhcp"); err != nil {
		return w.fail(ssid, err)
	}
	w.set(device.NetStatus{State: device.NetOnline, SSID: ssid, Addr: interfaceAddr(w.iface), Signal: w.signal()})
	return nil
}

// signal — уровень сигнала текущей сети в процентах; 0 — неизвестен.
func (w *wifi) signal() int {
	reply, err := w.conn.request("SIGNAL_POLL", time.Second)
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(reply, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "RSSI="); ok {
			if n, err := strconv.Atoi(v); err == nil {
				return signalPercent(n)
			}
		}
	}
	return 0
}

// interfaceAddr — первый IPv4-адрес интерфейса.
func interfaceAddr(name string) string {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}
	addrs, _ := ifc.Addrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil {
			return ip.IP.String()
		}
	}
	return ""
}

// Up включает Wi-Fi и ждёт подключения к знакомой сети и адреса.
func (w *wifi) Up(ctx context.Context) error {
	ctx, end := w.begin(ctx)
	defer end()
	if w.Status().State == device.NetOnline && w.conn != nil {
		if r, err := w.conn.request("STATUS", time.Second); err == nil && parseStatus(r).State == "COMPLETED" {
			return nil
		}
	}
	w.set(device.NetStatus{State: device.NetConnecting})
	if err := w.start(ctx); err != nil {
		return w.fail("", err)
	}
	ssid, err := w.waitAssociated(ctx)
	if err != nil {
		return w.fail("", err)
	}
	return w.online(ctx, ssid)
}

// Down выключает Wi-Fi целиком; начатое подключение прерывается.
func (w *wifi) Down() error {
	w.interrupt()
	defer w.op.Unlock()
	if w.conn != nil {
		w.conn.Close()
		w.conn = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := w.run(ctx, "down")
	w.set(device.NetStatus{State: device.NetOff})
	return err
}

// Close закрывает сокет. Сам Wi-Fi гасит Down, а при возврате в Nickel — скрипт запуска.
func (w *wifi) Close() error {
	w.interrupt()
	defer w.op.Unlock()
	if w.conn == nil {
		return nil
	}
	err := w.conn.Close()
	w.conn = nil
	return err
}

// Scan ищет сети поблизости.
func (w *wifi) Scan(ctx context.Context) ([]device.AccessPoint, error) {
	ctx, end := w.begin(ctx)
	defer end()
	if err := w.start(ctx); err != nil {
		return nil, err
	}
	if err := w.conn.ok("SCAN"); err != nil && !strings.Contains(err.Error(), "FAIL-BUSY") {
		return nil, err
	}
	have, err := w.networks()
	if err != nil {
		return nil, err
	}
	saved := map[string]bool{}
	for ssid := range have {
		saved[ssid] = true
	}
	// Результаты поиска копятся несколько секунд: ждём, пока список перестанет расти.
	var best []device.AccessPoint
	for i := 0; i < 10; i++ {
		t := time.NewTimer(w.poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return best, ctx.Err()
		case <-t.C:
		}
		reply, err := w.conn.request("SCAN_RESULTS", 3*time.Second)
		if err != nil {
			return best, err
		}
		aps := parseScan(reply, saved)
		if len(aps) > 0 && len(aps) == len(best) && i >= 3 {
			return aps, nil
		}
		best = aps
	}
	return best, nil
}

// Connect подключается к сети ssid и запоминает её. Пустой psk — открытая сеть или сеть,
// уже сохранённая в Nickel.
func (w *wifi) Connect(ctx context.Context, ssid, psk string) error {
	ctx, end := w.begin(ctx)
	defer end()
	w.set(device.NetStatus{State: device.NetConnecting, SSID: ssid})
	if err := w.start(ctx); err != nil {
		return w.fail(ssid, err)
	}
	have, err := w.networks()
	if err != nil {
		return w.fail(ssid, err)
	}
	s := savedNet{SSID: ssid}
	if psk != "" {
		if s.PSK, err = derivePSK(ssid, psk); err != nil {
			return w.fail(ssid, err)
		}
	}
	id, known := have[ssid]
	switch {
	case !known:
		if id, err = w.addNetwork(s); err != nil {
			return w.fail(ssid, err)
		}
	case psk != "":
		if err := w.configure(id, s); err != nil {
			return w.fail(ssid, err)
		}
	}
	if err := w.conn.ok(fmt.Sprintf("SELECT_NETWORK %d", id)); err != nil {
		return w.fail(ssid, err)
	}
	got, err := w.waitAssociated(ctx)
	// SELECT_NETWORK выключает остальные сети; возвращаем их, чтобы потом подключаться к любой.
	_ = w.conn.ok("ENABLE_NETWORK all")
	if err != nil {
		if !known {
			_ = w.conn.ok(fmt.Sprintf("REMOVE_NETWORK %d", id))
		}
		return w.fail(ssid, err)
	}
	if psk != "" || !known {
		if err := w.remember(s); err != nil {
			w.log.Warn("сеть не сохранилась", "сеть", ssid, "err", err)
		}
	}
	return w.online(ctx, got)
}

// Forget забывает сеть оболочки. Сети Nickel оболочка не трогает.
func (w *wifi) Forget(ssid string) error {
	_, end := w.begin(context.Background())
	defer end()
	saved, err := w.loadSaved()
	if err != nil {
		return err
	}
	kept := saved[:0]
	found := false
	for _, s := range saved {
		if s.SSID == ssid {
			found = true
			continue
		}
		kept = append(kept, s)
	}
	if !found {
		return fmt.Errorf("сеть «%s» сохранена в Nickel: забыть её можно только там", ssid)
	}
	if w.conn != nil {
		if have, err := w.networks(); err == nil {
			if id, ok := have[ssid]; ok {
				_ = w.conn.ok(fmt.Sprintf("REMOVE_NETWORK %d", id))
			}
		}
	}
	return w.saveSaved(kept)
}

func (w *wifi) loadSaved() ([]savedNet, error) {
	data, err := os.ReadFile(w.file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []savedNet
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("%s: %w", w.file, err)
	}
	return out, nil
}

func (w *wifi) saveSaved(nets []savedNet) error {
	data, err := json.MarshalIndent(nets, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteAtomic(w.file, data, true)
}

func (w *wifi) remember(s savedNet) error {
	saved, err := w.loadSaved()
	if err != nil {
		return err
	}
	for i := range saved {
		if saved[i].SSID == s.SSID {
			saved[i] = s
			return w.saveSaved(saved)
		}
	}
	return w.saveSaved(append(saved, s))
}
