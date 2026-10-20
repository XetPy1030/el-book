package kobo

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"el-book/sdk/device"
)

// fakeWPA — wpa_supplicant для тестов: отвечает на команды управляющего сокета по своей
// модели сетей. Сеть подключается, если её ключ совпал с правильным, иначе застревает
// на обмене ключами.
type fakeWPA struct {
	dir  string
	conn *net.UnixConn

	mu        sync.Mutex
	nets      map[int]*fakeNet
	next      int
	visible   []string          // сети в эфире, по порядку
	good      map[string]string // ssid → правильный ключ
	selected  int               // SELECT_NETWORK; -1 — любая включённая
	connected *fakeNet
	polls     int // сколько ответов STATUS до подключения
	cmds      []string
}

type fakeNet struct {
	id      int
	ssid    string
	psk     string
	enabled bool
}

// wpaSetup — эфир и сети для поддельного wpa_supplicant.
type wpaSetup struct {
	nickel  []string          // сети из конфига Nickel
	visible []string          // сети в эфире
	good    map[string]string // ssid → правильный ключ сетей оболочки
	polls   int               // сколько ответов STATUS до подключения
}

// newFakeWPA запускает поддельный wpa_supplicant.
func newFakeWPA(t *testing.T, cfg wpaSetup) *fakeWPA {
	t.Helper()
	// Путь сокета ограничен ~100 байтами, поэтому каталог короткий, а не t.TempDir.
	dir, err := os.MkdirTemp("", "wpa")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: filepath.Join(dir, "eth0"), Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeWPA{dir: dir, conn: c, nets: map[int]*fakeNet{}, good: map[string]string{},
		visible: cfg.visible, polls: cfg.polls, selected: -1}
	for _, ssid := range cfg.nickel {
		f.add(ssid, "nickel")
		f.good[ssid] = "nickel"
	}
	for ssid, psk := range cfg.good {
		f.good[ssid] = psk
	}
	go f.serve()
	t.Cleanup(func() { c.Close() })
	return f
}

func (f *fakeWPA) add(ssid, psk string) *fakeNet {
	n := &fakeNet{id: f.next, ssid: ssid, psk: psk, enabled: true}
	f.nets[n.id] = n
	f.next++
	return n
}

func (f *fakeWPA) serve() {
	buf := make([]byte, 4096)
	for {
		n, addr, err := f.conn.ReadFromUnix(buf)
		if err != nil {
			return
		}
		reply := f.handle(string(buf[:n]))
		_, _ = f.conn.WriteToUnix([]byte(reply), addr)
	}
}

func (f *fakeWPA) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.cmds)
}

// has сообщает, знает ли wpa_supplicant сеть ssid.
func (f *fakeWPA) has(ssid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nets {
		if n.ssid == ssid {
			return true
		}
	}
	return false
}

func (f *fakeWPA) handle(cmd string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, cmd)
	args := strings.Fields(cmd)
	id := -1
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil {
			id = v
		}
	}
	switch args[0] {
	case "PING":
		return "PONG\n"
	case "LIST_NETWORKS":
		var b strings.Builder
		b.WriteString("network id / ssid / bssid / flags\n")
		for i := 0; i < f.next; i++ {
			if n, ok := f.nets[i]; ok {
				fmt.Fprintf(&b, "%d\t%s\tany\t\n", n.id, encodeSSID(n.ssid))
			}
		}
		return b.String()
	case "ADD_NETWORK":
		n := f.add("", "")
		n.enabled = false
		return strconv.Itoa(n.id) + "\n"
	case "SET_NETWORK":
		n, ok := f.nets[id]
		if !ok || len(args) < 4 {
			return "FAIL\n"
		}
		switch args[2] {
		case "ssid":
			b, err := hex.DecodeString(args[3])
			if err != nil {
				return "FAIL\n"
			}
			n.ssid = string(b)
		case "psk":
			n.psk = args[3]
		case "key_mgmt":
			n.psk = ""
		}
		return "OK\n"
	case "ENABLE_NETWORK":
		if len(args) > 1 && args[1] == "all" {
			for _, n := range f.nets {
				n.enabled = true
			}
			f.selected = -1
			return "OK\n"
		}
		if n, ok := f.nets[id]; ok {
			n.enabled = true
			return "OK\n"
		}
		return "FAIL\n"
	case "SELECT_NETWORK":
		if _, ok := f.nets[id]; !ok {
			return "FAIL\n"
		}
		f.selected = id
		f.connected = nil
		return "OK\n"
	case "REMOVE_NETWORK":
		if _, ok := f.nets[id]; !ok {
			return "FAIL\n"
		}
		delete(f.nets, id)
		return "OK\n"
	case "STATUS":
		return f.status()
	case "SCAN":
		return "OK\n"
	case "SCAN_RESULTS":
		var b strings.Builder
		b.WriteString("bssid / frequency / signal level / flags / ssid\n")
		for i, ssid := range f.visible {
			fmt.Fprintf(&b, "aa:bb:cc:dd:ee:%02x\t2412\t%d\t[WPA2-PSK-CCMP][ESS]\t%s\n", i, -50-10*i, encodeSSID(ssid))
		}
		return b.String()
	case "SIGNAL_POLL":
		return "RSSI=-60\nLINKSPEED=65\n"
	}
	return "UNKNOWN COMMAND\n"
}

// status — ответ STATUS: подключаемся к выбранной сети или к первой знакомой в эфире.
func (f *fakeWPA) status() string {
	if c := f.connected; c != nil {
		return "wpa_state=COMPLETED\nssid=" + encodeSSID(c.ssid) + "\nip_address=192.168.1.5\n"
	}
	var target *fakeNet
	if n, ok := f.nets[f.selected]; ok {
		target = n
	} else {
		for i := 0; i < f.next && target == nil; i++ {
			if n, ok := f.nets[i]; ok && n.enabled && slices.Contains(f.visible, n.ssid) {
				target = n
			}
		}
	}
	switch {
	case target == nil:
		return "wpa_state=SCANNING\n"
	case f.good[target.ssid] != target.psk:
		return "wpa_state=4WAY_HANDSHAKE\nssid=" + encodeSSID(target.ssid) + "\n"
	case f.polls > 0:
		f.polls--
		return "wpa_state=ASSOCIATING\n"
	}
	f.connected = target
	return f.status()
}

// encodeSSID печатает имя, как wpa_supplicant: байты вне ASCII — \xNN.
func encodeSSID(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' || c == '"':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 32 || c >= 127:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// fakeScript — wifi.sh для тестов: записывает команды.
type fakeScript struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]error
	wpa   *fakeWPA
}

func (s *fakeScript) run(ctx context.Context, cmd string) error {
	s.mu.Lock()
	s.calls = append(s.calls, cmd)
	err := s.fail[cmd]
	s.mu.Unlock()
	if cmd == "down" && s.wpa != nil {
		s.wpa.mu.Lock()
		s.wpa.connected = nil
		s.wpa.mu.Unlock()
	}
	return err
}

func (s *fakeScript) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

type statusLog struct {
	mu   sync.Mutex
	list []device.NetStatus
}

func (l *statusLog) states() []device.NetState {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []device.NetState
	for _, s := range l.list {
		out = append(out, s.State)
	}
	return out
}

// newTestWifi — Wi-Fi поверх поддельных wpa_supplicant и скрипта; file — сети оболочки.
func newTestWifi(f *fakeWPA, file string) (*wifi, *fakeScript, *statusLog) {
	script := &fakeScript{fail: map[string]error{}, wpa: f}
	events := &statusLog{}
	w := &wifi{
		run: script.run, ctrlDir: f.dir, tmpDir: f.dir, iface: "eth0", file: file,
		send: func(e device.Event) {
			events.mu.Lock()
			events.list = append(events.list, e.(device.NetworkEvent).Status)
			events.mu.Unlock()
		},
		log: slog.New(slog.DiscardHandler), poll: time.Millisecond, assocTimeout: 300 * time.Millisecond,
	}
	return w, script, events
}

func TestWifiUpJoinsNickelNetwork(t *testing.T) {
	f := newFakeWPA(t, wpaSetup{nickel: []string{"Дом"}, visible: []string{"Дом"}, polls: 2})
	w, script, events := newTestWifi(f, filepath.Join(t.TempDir(), "networks.json"))
	defer w.Close()

	if err := w.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	st := w.Status()
	if st.State != device.NetOnline || st.SSID != "Дом" || st.Signal != 80 {
		t.Errorf("состояние %+v, ждали «Дом» в сети с сигналом 80", st)
	}
	if got := script.commands(); !slices.Equal(got, []string{"up", "dhcp"}) {
		t.Errorf("скрипт: %v", got)
	}
	want := []device.NetState{device.NetConnecting, device.NetConnecting, device.NetOnline}
	if got := events.states(); !slices.Equal(got, want) {
		t.Errorf("события %v, ждали %v", got, want)
	}

	// Повторный Up при живой сети ничего не запускает.
	if err := w.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := script.commands(); len(got) != 2 {
		t.Errorf("повторный Up запустил скрипт: %v", got)
	}
}

func TestWifiUpFailures(t *testing.T) {
	cases := []struct {
		name    string
		nickel  []string
		visible []string
		polls   int
		want    error
	}{
		{"нет сохранённых сетей", nil, []string{"Сосед"}, 0, device.ErrNoKnownNet},
		{"знакомой сети нет в эфире", []string{"Дом"}, []string{"Сосед"}, 0, device.ErrNoKnownNet},
		{"сеть в эфире молчит", []string{"Дом"}, []string{"Дом"}, 1 << 30, device.ErrNetTimeout},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeWPA(t, wpaSetup{nickel: c.nickel, visible: c.visible, polls: c.polls})
			w, _, _ := newTestWifi(f, filepath.Join(t.TempDir(), "networks.json"))
			defer w.Close()
			err := w.Up(context.Background())
			if !errors.Is(err, c.want) {
				t.Fatalf("ошибка %v, ждали %v", err, c.want)
			}
			if st := w.Status(); st.State != device.NetFailed || st.Err != c.want.Error() {
				t.Errorf("состояние %+v", st)
			}
		})
	}
}

func TestWifiScriptFailure(t *testing.T) {
	f := newFakeWPA(t, wpaSetup{nickel: []string{"Дом"}})
	w, script, _ := newTestWifi(f, filepath.Join(t.TempDir(), "networks.json"))
	defer w.Close()
	script.fail["up"] = scriptError("up", 3)
	err := w.Up(context.Background())
	if !errors.Is(err, device.ErrNoWifi) {
		t.Fatalf("ошибка %v, ждали ErrNoWifi", err)
	}
	if w.Status().State != device.NetFailed {
		t.Errorf("состояние %v", w.Status())
	}
}

func TestWifiConnectWrongPassword(t *testing.T) {
	psk, err := derivePSK("Кофейня", "правильный пароль")
	if err != nil {
		t.Fatal(err)
	}
	f := newFakeWPA(t, wpaSetup{nickel: []string{"Дом"}, visible: []string{"Кофейня", "Дом"},
		good: map[string]string{"Кофейня": psk}})
	file := filepath.Join(t.TempDir(), "networks.json")
	w, _, _ := newTestWifi(f, file)
	defer w.Close()

	err = w.Connect(context.Background(), "Кофейня", "неверный пароль")
	if !errors.Is(err, device.ErrWrongPass) {
		t.Fatalf("ошибка %v, ждали ErrWrongPass", err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("сеть с неверным паролем сохранилась: %v", err)
	}
	if f.has("Кофейня") {
		t.Errorf("сеть с неверным паролем осталась в wpa_supplicant")
	}
	if !slices.Contains(f.commands(), "ENABLE_NETWORK all") {
		t.Errorf("остальные сети не включены обратно: %v", f.commands())
	}
}

func TestWifiConnectRemembersKeyNotPassword(t *testing.T) {
	psk, _ := derivePSK("Кофейня", "правильный пароль")
	setup := wpaSetup{nickel: []string{"Дом"}, visible: []string{"Кофейня"}, good: map[string]string{"Кофейня": psk}}
	f := newFakeWPA(t, setup)
	file := filepath.Join(t.TempDir(), "networks.json")
	w, script, _ := newTestWifi(f, file)

	if err := w.Connect(context.Background(), "Кофейня", "правильный пароль"); err != nil {
		t.Fatal(err)
	}
	if st := w.Status(); st.State != device.NetOnline || st.SSID != "Кофейня" {
		t.Errorf("состояние %+v", st)
	}
	if got := script.commands(); !slices.Equal(got, []string{"up", "dhcp"}) {
		t.Errorf("скрипт: %v", got)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "правильный") || !strings.Contains(string(data), psk) {
		t.Errorf("в файле сетей не ключ, а пароль или ничего:\n%s", data)
	}
	if !slices.Contains(f.commands(), "SET_NETWORK 1 ssid "+hex.EncodeToString([]byte("Кофейня"))) {
		t.Errorf("имя сети передано не в шестнадцатеричном виде: %v", f.commands())
	}
	w.Close()

	// Новый запуск: wpa_supplicant знает только сети Nickel, сеть оболочки добавляется сама.
	f2 := newFakeWPA(t, setup)
	w2, _, _ := newTestWifi(f2, file)
	defer w2.Close()
	if err := w2.Up(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := w2.Status(); st.SSID != "Кофейня" {
		t.Errorf("после перезапуска подключились к %q", st.SSID)
	}
}

func TestWifiForget(t *testing.T) {
	f := newFakeWPA(t, wpaSetup{nickel: []string{"Дом"}, visible: []string{"Кофейня"}, good: map[string]string{"Кофейня": ""}})
	file := filepath.Join(t.TempDir(), "networks.json")
	w, _, _ := newTestWifi(f, file)
	defer w.Close()
	if err := w.Connect(context.Background(), "Кофейня", ""); err != nil {
		t.Fatal(err)
	}
	if err := w.Forget("Дом"); err == nil {
		t.Error("сеть Nickel забыта")
	}
	if err := w.Forget("Кофейня"); err != nil {
		t.Fatal(err)
	}
	saved, err := w.loadSaved()
	if err != nil || len(saved) != 0 {
		t.Errorf("после Forget сохранено %v, %v", saved, err)
	}
	if f.has("Кофейня") {
		t.Error("забытая сеть осталась в wpa_supplicant")
	}
}

func TestWifiScan(t *testing.T) {
	f := newFakeWPA(t, wpaSetup{nickel: []string{"Дом"}, visible: []string{"Дом", "Кофейня", "Дом"}})
	w, _, _ := newTestWifi(f, filepath.Join(t.TempDir(), "networks.json"))
	defer w.Close()
	aps, err := w.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []device.AccessPoint{
		{SSID: "Дом", Signal: 100, Secure: true, Saved: true},
		{SSID: "Кофейня", Signal: 80, Secure: true},
	}
	if !slices.Equal(aps, want) {
		t.Errorf("сети %+v, ждали %+v", aps, want)
	}
}

func TestWifiDownInterruptsUp(t *testing.T) {
	f := newFakeWPA(t, wpaSetup{nickel: []string{"Дом"}, visible: []string{"Дом"}, polls: 1 << 30})
	w, script, _ := newTestWifi(f, filepath.Join(t.TempDir(), "networks.json"))
	w.assocTimeout = time.Minute
	defer w.Close()

	done := make(chan error, 1)
	go func() { done <- w.Up(context.Background()) }()
	for w.Status().State != device.NetConnecting {
		time.Sleep(time.Millisecond)
	}
	if err := w.Down(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Up вернул %v, ждали отмену", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Down не прервал Up")
	}
	if st := w.Status(); st.State != device.NetOff {
		t.Errorf("после Down состояние %+v", st)
	}
	if got := script.commands(); got[len(got)-1] != "down" {
		t.Errorf("скрипт: %v", got)
	}
}

func TestRunScript(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "wifi.log")
	script := filepath.Join(dir, "wifi.sh")
	// dhcp оставляет процесс в фоне с открытым выводом: runScript не должен его ждать.
	body := "case \"$1\" in\n  dhcp) sleep 3 & echo адрес получен ;;\n  down) exit 7 ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := runScript(context.Background(), script, "dhcp", logPath); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("runScript ждал фоновый процесс %v", d)
	}
	if err := runScript(context.Background(), script, "down", logPath); !errors.Is(err, device.ErrNetTimeout) {
		t.Errorf("код 7: %v", err)
	}
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "wifi.sh dhcp") || !strings.Contains(string(data), "адрес получен") {
		t.Errorf("журнал скрипта:\n%s", data)
	}
}

func TestDerivePSK(t *testing.T) {
	// Пример из IEEE 802.11i, приложение H.4.
	got, err := derivePSK("IEEE", "password")
	if err != nil {
		t.Fatal(err)
	}
	if want := "f42c6fc52df0ebef9ebb4b90b38a5f902e83fe1b135a70e23aed762e9710a12e"; got != want {
		t.Errorf("ключ %s, ждали %s", got, want)
	}
	if _, err := derivePSK("Дом", "short"); !errors.Is(err, device.ErrWrongPass) {
		t.Errorf("короткий пароль: %v", err)
	}
}

func TestWPAParsers(t *testing.T) {
	if got := unescapeSSID(encodeSSID(`Дом "у реки" \ 5G`)); got != `Дом "у реки" \ 5G` {
		t.Errorf("unescapeSSID: %q", got)
	}
	st := parseStatus("bssid=aa\nwpa_state=COMPLETED\nssid=\\xd0\\x94\\xd0\\xbe\\xd0\\xbc\nip_address=10.0.0.2\n")
	if st != (wpaStatus{State: "COMPLETED", SSID: "Дом", IP: "10.0.0.2"}) {
		t.Errorf("parseStatus: %+v", st)
	}
	for level, want := range map[int]int{-100: 0, -75: 50, -50: 100, -30: 100, -120: 0, 70: 70, 130: 100} {
		if got := signalPercent(level); got != want {
			t.Errorf("signalPercent(%d) = %d, ждали %d", level, got, want)
		}
	}
	aps := parseScan("bssid / frequency / signal level / flags / ssid\n"+
		"aa\t2412\t-80\t[ESS]\tОткрытая\n"+
		"bb\t2412\t-40\t[WPA2-PSK-CCMP][ESS]\t\n"+
		"cc\t2412\t-60\t[WEP][ESS]\tСтарая\n", map[string]bool{"Старая": true})
	want := []device.AccessPoint{
		{SSID: "Старая", Signal: 80, Secure: true, Saved: true},
		{SSID: "Открытая", Signal: 40},
	}
	if !slices.Equal(aps, want) {
		t.Errorf("parseScan: %+v", aps)
	}
}

func TestDeviceWithoutWifi(t *testing.T) {
	if n := (&Device{}).Network(); n != nil {
		t.Errorf("без Wi-Fi Network() = %v, ждали nil", n)
	}
}
