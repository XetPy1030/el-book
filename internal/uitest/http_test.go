package uitest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// fixtures — сеть UI-тестов: ответы из файлов по таблице routes.txt. Настоящая сеть тестам
// не нужна, а снимки не зависят от сегодняшней погоды и новостей.
type fixtures struct {
	dir    string
	routes []route

	mu     sync.Mutex
	missed []string // адреса, которым не нашлось ответа
}

type route struct{ match, file string }

// contentTypes — заголовок ответа по расширению. Кодировку XML лента объявляет сама.
var contentTypes = map[string]string{
	".json": "application/json",
	".xml":  "application/xml",
	".html": "text/html; charset=utf-8",
}

func loadFixtures(dir string) (*fixtures, error) {
	f, err := os.Open(filepath.Join(dir, "routes.txt"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fx := &fixtures{dir: dir}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		switch len(fields) {
		case 0:
			continue
		case 2:
			fx.routes = append(fx.routes, route{match: fields[0], file: fields[1]})
		default:
			return nil, fmt.Errorf("routes.txt:%d: ожидалось «подстрока файл»", n)
		}
	}
	return fx, sc.Err()
}

func (fx *fixtures) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	for _, r := range fx.routes {
		if !strings.Contains(url, r.match) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(fx.dir, r.file))
		if err != nil {
			return nil, err
		}
		kind := contentTypes[filepath.Ext(r.file)]
		return &http.Response{
			Status: "200 OK", StatusCode: http.StatusOK, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
			Header: http.Header{"Content-Type": {kind}}, Body: io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)), Request: req,
		}, nil
	}
	fx.mu.Lock()
	fx.missed = append(fx.missed, url)
	fx.mu.Unlock()
	return nil, fmt.Errorf("в testdata/http нет ответа для %s", url)
}

// Missed — адреса, запрошенные без ответа в таблице.
func (fx *fixtures) Missed() []string {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return append([]string(nil), fx.missed...)
}
