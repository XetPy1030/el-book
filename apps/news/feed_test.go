package news

import (
	"testing"
	"time"
)

const rssSample = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
<channel><title>Наука сегодня</title>
<item><title>Кометы &amp; астероиды</title><link>https://example.org/a</link><guid>a1</guid>
<description><![CDATA[<p>Астрономы <b>нашли</b> новую комету.</p>]]></description>
<pubDate>Mon, 19 Oct 2026 10:00:00 +0300</pubDate></item>
<item><title>Старое</title><link>https://example.org/b</link>
<description>Без даты в RFC — dc:date ниже</description>
<dc:date xmlns:dc="http://purl.org/dc/elements/1.1/">2026-10-18T09:00:00Z</dc:date></item>
</channel></rss>`

const atomSample = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>Блог</title>
<entry><title>Запись</title><id>urn:1</id><link rel="alternate" href="https://example.org/post"/>
<summary type="html">&lt;p&gt;Кратко о главном&lt;/p&gt;</summary><updated>2026-10-19T12:00:00Z</updated></entry>
</feed>`

func TestParseRSS(t *testing.T) {
	title, items, err := parseFeed([]byte(rssSample), Source{Title: "Наука", URL: "https://example.org/rss"})
	if err != nil {
		t.Fatal(err)
	}
	if title != "Наука сегодня" || len(items) != 2 {
		t.Fatalf("лента %q, %d новостей", title, len(items))
	}
	it := items[0]
	if it.Title != "Кометы & астероиды" || it.Summary != "Астрономы нашли новую комету." || it.Link != "https://example.org/a" {
		t.Errorf("новость: %+v", it)
	}
	if want := time.Date(2026, 10, 19, 7, 0, 0, 0, time.UTC); !it.Date.Equal(want) {
		t.Errorf("дата %v, ждали %v", it.Date, want)
	}
	if items[1].Date.IsZero() {
		t.Error("дата dc:date не прочитана")
	}
}

func TestParseAtomAndMerge(t *testing.T) {
	_, a, err := parseFeed([]byte(atomSample), Source{Title: "Блог", URL: "https://example.org/atom"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a[0].Link != "https://example.org/post" || a[0].Summary != "Кратко о главном" {
		t.Fatalf("Atom: %+v", a)
	}
	_, r, _ := parseFeed([]byte(rssSample), Source{Title: "Наука", URL: "https://example.org/rss"})
	all := merge(r, a, r)
	if len(all) != 3 {
		t.Fatalf("после слияния %d новостей, ждали 3 без повторов", len(all))
	}
	if all[0].Title != "Запись" {
		t.Errorf("свежая новость не первая: %q", all[0].Title)
	}
}

func TestParseCP1251(t *testing.T) {
	// «Привет» в windows-1251.
	data := append([]byte(`<?xml version="1.0" encoding="windows-1251"?><rss><channel><title>`),
		0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2)
	data = append(data, []byte(`</title></channel></rss>`)...)
	title, _, err := parseFeed(data, Source{URL: "x"})
	if err != nil || title != "Привет" {
		t.Errorf("windows-1251: %q, %v", title, err)
	}
	if _, _, err := parseFeed([]byte("<html><body>не лента</body></html>"), Source{URL: "x"}); err == nil {
		t.Error("страница HTML принята за ленту")
	}
}
