package kobo

import (
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"el-book/sdk/device"
)

// wpaConn — управляющий сокет wpa_supplicant: датаграммы «команда → ответ», как у wpa_cli.
// Клиент привязывает свой сокет к файлу в /tmp, иначе ответу некуда прийти.
type wpaConn struct {
	c     *net.UnixConn
	local string
}

var wpaSeq atomic.Int64

// dialWPA подключается к сокету интерфейса iface в каталоге dir (/var/run/wpa_supplicant).
func dialWPA(dir, iface, localDir string) (*wpaConn, error) {
	local := filepath.Join(localDir, fmt.Sprintf("el-book-wpa-%d-%d", os.Getpid(), wpaSeq.Add(1)))
	_ = os.Remove(local)
	c, err := net.DialUnix("unixgram",
		&net.UnixAddr{Name: local, Net: "unixgram"},
		&net.UnixAddr{Name: filepath.Join(dir, iface), Net: "unixgram"})
	if err != nil {
		return nil, fmt.Errorf("сокет wpa_supplicant: %w", err)
	}
	return &wpaConn{c: c, local: local}, nil
}

func (w *wpaConn) Close() error {
	err := w.c.Close()
	_ = os.Remove(w.local)
	return err
}

// request отправляет команду и ждёт ответа не дольше timeout.
func (w *wpaConn) request(cmd string, timeout time.Duration) (string, error) {
	if err := w.c.SetDeadline(time.Now().Add(timeout)); err != nil {
		return "", err
	}
	if _, err := w.c.Write([]byte(cmd)); err != nil {
		return "", fmt.Errorf("wpa_supplicant %s: %w", firstWord(cmd), err)
	}
	buf := make([]byte, 8192)
	for {
		n, err := w.c.Read(buf)
		if err != nil {
			return "", fmt.Errorf("wpa_supplicant %s: %w", firstWord(cmd), err)
		}
		reply := string(buf[:n])
		if strings.HasPrefix(reply, "<") {
			continue // событие для подписчика, а не ответ
		}
		return reply, nil
	}
}

// ok отправляет команду, на которую ответ — «OK».
func (w *wpaConn) ok(cmd string) error {
	reply, err := w.request(cmd, 3*time.Second)
	if err != nil {
		return err
	}
	if strings.TrimSpace(reply) != "OK" {
		return fmt.Errorf("wpa_supplicant %s: %s", firstWord(cmd), strings.TrimSpace(reply))
	}
	return nil
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}

// wpaStatus — ответ STATUS.
type wpaStatus struct {
	State string // wpa_state: COMPLETED, SCANNING, 4WAY_HANDSHAKE…
	SSID  string
	IP    string
}

func parseStatus(s string) wpaStatus {
	var st wpaStatus
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "wpa_state":
			st.State = v
		case "ssid":
			st.SSID = unescapeSSID(v)
		case "ip_address":
			st.IP = v
		}
	}
	return st
}

// wpaNetwork — строка LIST_NETWORKS.
type wpaNetwork struct {
	ID   int
	SSID string
}

func parseNetworks(s string) []wpaNetwork {
	var out []wpaNetwork
	for i, line := range strings.Split(s, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // заголовок «network id / ssid / bssid / flags»
		}
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			continue
		}
		id, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		out = append(out, wpaNetwork{ID: id, SSID: unescapeSSID(f[1])})
	}
	return out
}

// parseScan разбирает SCAN_RESULTS: «bssid, частота, уровень, флаги, имя» через табуляцию.
// Одна сеть с несколькими точками доступа — одна строка с лучшим сигналом; скрытые сети
// без имени пропускаются. Сильные сети первыми.
func parseScan(s string, saved map[string]bool) []device.AccessPoint {
	best := map[string]device.AccessPoint{}
	for i, line := range strings.Split(s, "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		ssid := unescapeSSID(f[4])
		if ssid == "" {
			continue
		}
		level, _ := strconv.Atoi(f[2])
		ap := device.AccessPoint{
			SSID:   ssid,
			Signal: signalPercent(level),
			Secure: strings.Contains(f[3], "WPA") || strings.Contains(f[3], "WEP") || strings.Contains(f[3], "RSN"),
			Saved:  saved[ssid],
		}
		if old, ok := best[ssid]; !ok || ap.Signal > old.Signal {
			best[ssid] = ap
		}
	}
	out := make([]device.AccessPoint, 0, len(best))
	for _, ap := range best {
		out = append(out, ap)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Signal != out[j].Signal {
			return out[i].Signal > out[j].Signal
		}
		return out[i].SSID < out[j].SSID
	})
	return out
}

// signalPercent переводит уровень сигнала в проценты: драйверы отдают либо дБм (отрицательные,
// −100 — нет сигнала, −50 — отлично), либо уже качество 0…100.
func signalPercent(level int) int {
	if level < 0 {
		return min(max(2*(level+100), 0), 100)
	}
	return min(level, 100)
}

// unescapeSSID снимает экранирование printf_encode, которым wpa_supplicant печатает имена
// сетей: «\xd0\x94ом» — «Дом».
func unescapeSSID(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b = append(b, s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b = append(b, '\n')
		case 'r':
			b = append(b, '\r')
		case 't':
			b = append(b, '\t')
		case 'e':
			b = append(b, 0x1b)
		case 'x':
			if i+2 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					b = append(b, byte(v))
					i += 2
					continue
				}
			}
			b = append(b, 'x')
		default:
			b = append(b, s[i])
		}
	}
	return string(b)
}

// derivePSK — ключ сети WPA из пароля и имени: PBKDF2-SHA1, 4096 итераций, 32 байта. В таком
// виде wpa_supplicant принимает ключ вместо пароля, а сам пароль на книге не хранится.
func derivePSK(ssid, passphrase string) (string, error) {
	if n := len(passphrase); n < 8 || n > 63 {
		return "", fmt.Errorf("%w: пароль сети — от 8 до 63 символов", device.ErrWrongPass)
	}
	key, err := pbkdf2.Key(sha1.New, passphrase, []byte(ssid), 4096, 32)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(key), nil
}
