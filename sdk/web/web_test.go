package web

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"el-book/sdk/device"
)

// memCache — кэш в памяти.
type memCache struct {
	files map[string][]byte
	at    map[string]time.Time
	now   func() time.Time
}

func newMemCache(now func() time.Time) *memCache {
	return &memCache{files: map[string][]byte{}, at: map[string]time.Time{}, now: now}
}

func (m *memCache) ReadCache(name string) ([]byte, time.Time, error) {
	b, ok := m.files[name]
	if !ok {
		return nil, time.Time{}, fs.ErrNotExist
	}
	return b, m.at[name], nil
}

func (m *memCache) WriteCache(name string, data []byte) error {
	m.files[name] = data
	m.at[name] = m.now()
	return nil
}

func TestFetchUsesCacheAndFallsBackWhenOffline(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Write([]byte(`{"t": 12}`))
	}))
	defer srv.Close()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	cache := newMemCache(clock)
	online := true
	connects := 0
	c := New(Options{Cache: cache, Now: clock, Connect: func(context.Context) error {
		connects++
		if !online {
			return device.ErrNoKnownNet
		}
		return nil
	}})
	ctx := context.Background()

	var v struct{ T int }
	if _, err := c.FetchJSON(ctx, srv.URL, time.Hour, &v); err != nil || v.T != 12 {
		t.Fatalf("первая загрузка: %v, %+v", err, v)
	}
	if _, err := c.Fetch(ctx, srv.URL, time.Hour); err != nil || hits != 1 {
		t.Fatalf("свежий ответ должен браться из кэша: обращений %d, %v", hits, err)
	}
	now = now.Add(2 * time.Hour)
	online = false
	res, err := c.Fetch(ctx, srv.URL, time.Hour)
	if err != nil || !res.Stale || !errors.Is(res.Err, device.ErrNoKnownNet) || string(res.Body) != `{"t": 12}` {
		t.Fatalf("без сети нужен сохранённый ответ: %+v, %v", res, err)
	}
	if connects != 2 {
		t.Fatalf("сеть включалась %d раз, нужно 2", connects)
	}
	if _, err := c.Fetch(ctx, srv.URL+"/other", time.Hour); !errors.Is(err, device.ErrNoKnownNet) {
		t.Fatalf("без сети и кэша нужна ошибка сети, получено %v", err)
	}
}

func TestErrorsAndLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/missing":
			http.NotFound(w, r)
		case "/big":
			w.Write([]byte(strings.Repeat("x", 100)))
		default:
			if r.Header.Get("User-Agent") != "el-book/test" {
				t.Errorf("User-Agent %q", r.Header.Get("User-Agent"))
			}
			w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	c := New(Options{UserAgent: "el-book/test", MaxBody: 50})
	ctx := context.Background()
	if _, err := c.Get(ctx, srv.URL+"/missing"); !strings.Contains(Explain(err), "не нашёл") {
		t.Fatalf("404: %v → %q", err, Explain(err))
	}
	if _, err := c.Get(ctx, srv.URL+"/big"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("большой ответ: %v", err)
	}
	if b, err := c.Get(ctx, srv.URL+"/"); err != nil || string(b) != "ok" {
		t.Fatalf("обычный ответ: %q, %v", b, err)
	}
	if got := URL("https://x/api", "q", "Самара", "n", "5"); got != "https://x/api?n=5&q=%D0%A1%D0%B0%D0%BC%D0%B0%D1%80%D0%B0" {
		t.Fatalf("URL = %s", got)
	}
}

func TestExtractText(t *testing.T) {
	doc := `<!doctype html><html><head><title>Кометы | Журнал</title>
<meta property="og:title" content="Кометы возвращаются">
<style>p{color:red}</style><script>var x = "<p>не текст</p>";</script></head>
<body><nav><a href="/">Главная</a> <a href="/news">Новости</a></nav>
<article><h1>Кометы возвращаются</h1>
<p>Астрономы заметили, что комета &laquo;Галлея&raquo; снова приближается к Солнцу.</p>
<!-- реклама --><div class="ad">Купите телескоп</div>
<h2>Что известно</h2>
<ul><li>Период — около 76 лет</li><li>Последний визит — 1986 год</li></ul>
<blockquote><p>Это событие поколения, — сказал директор обсерватории.</p></blockquote>
<p>Следующее сближение ожидается в 2061 году, и к нему уже готовятся.<br>Подробности позже, когда будут данные наблюдений.</p>
</article><footer>© Журнал, все права защищены</footer></body></html>`
	p := ExtractText([]byte(doc))
	if p.Title != "Кометы возвращаются" {
		t.Errorf("заголовок %q", p.Title)
	}
	var got []string
	for _, para := range p.Paras {
		prefix := ""
		switch {
		case para.Heading > 0:
			prefix = "# "
		case para.Item:
			prefix = "- "
		case para.Quote:
			prefix = "> "
		}
		got = append(got, prefix+para.Text)
	}
	want := []string{
		"Астрономы заметили, что комета «Галлея» снова приближается к Солнцу.",
		"# Что известно",
		"- Период — около 76 лет",
		"- Последний визит — 1986 год",
		"> Это событие поколения, — сказал директор обсерватории.",
		"Следующее сближение ожидается в 2061 году, и к нему уже готовятся.",
		"Подробности позже, когда будут данные наблюдений.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("абзацы:\n%s\nждали:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if s := StripTags("<p>Привет, <b>мир</b>&nbsp;&amp; всё</p>"); s != "Привет, мир & всё" {
		t.Errorf("StripTags: %q", s)
	}
}

func TestExtractTextShortTitle(t *testing.T) {
	// <title> короче заголовка статьи: заголовок не должен повториться в тексте.
	doc := `<html><head><title>Склад на ценниках</title></head><body><article>
<h1>Склад на ценниках: как мы не сошли с ума</h1>
<p>Двадцать тысяч ценников на e-ink-экранах висят на стеллажах склада.</p>
<p>Каждую ночь цены меняются, и к утру все ценники должны показывать новые.</p>
<p>Об этом и пойдёт речь дальше, с цифрами и картинками.</p></article></body></html>`
	p := ExtractText([]byte(doc))
	if p.Title != "Склад на ценниках: как мы не сошли с ума" {
		t.Errorf("заголовок %q", p.Title)
	}
	if len(p.Paras) == 0 || p.Paras[0].Heading > 0 {
		t.Errorf("первый абзац — снова заголовок: %+v", p.Paras)
	}
}
