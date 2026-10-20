package news

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"el-book/sdk/web"
)

// Source — лента новостей.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Item — новость.
type Item struct {
	ID      string    `json:"id"`
	Source  string    `json:"source"` // название ленты
	Title   string    `json:"title"`
	Link    string    `json:"link"`
	Summary string    `json:"summary"`
	Date    time.Time `json:"date"`
}

// rss — RSS 2.0 и RSS 1.0 (RDF).
type rss struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
	Items []rssItem `xml:"item"` // RSS 1.0: элементы рядом с каналом
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	Encoded     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	PubDate     string `xml:"pubDate"`
	DCDate      string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

// atom — лента Atom.
type atom struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title string `xml:"title"`
	ID    string `xml:"id"`
	Links []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	} `xml:"link"`
	Summary   string `xml:"summary"`
	Content   string `xml:"content"`
	Updated   string `xml:"updated"`
	Published string `xml:"published"`
}

// dateLayouts — как ленты пишут даты.
var dateLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC3339, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST",
	"2 Jan 2006 15:04:05 -0700", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02",
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func itemID(source, key string) string {
	sum := sha1.Sum([]byte(source + "\x00" + key))
	return hex.EncodeToString(sum[:8])
}

// parseFeed разбирает RSS или Atom. Описания очищаются от разметки и укорачиваются.
func parseFeed(data []byte, src Source) (title string, items []Item, err error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = charsetReader
	var root xml.StartElement
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", nil, fmt.Errorf("лента %s не разбирается: %w", src.URL, err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			root = se
			break
		}
	}
	switch strings.ToLower(root.Name.Local) {
	case "rss", "rdf":
		var r rss
		if err := dec.DecodeElement(&r, &root); err != nil {
			return "", nil, fmt.Errorf("лента %s: %w", src.URL, err)
		}
		all := append(r.Channel.Items, r.Items...)
		for _, it := range all {
			key := it.GUID
			if key == "" {
				key = it.Link
			}
			date := parseDate(it.PubDate)
			if date.IsZero() {
				date = parseDate(it.DCDate)
			}
			body := it.Description
			if body == "" {
				body = it.Encoded
			}
			items = append(items, Item{ID: itemID(src.URL, key+it.Title), Source: src.Title, Title: clean(it.Title),
				Link: strings.TrimSpace(it.Link), Summary: clean(body), Date: date})
		}
		return clean(r.Channel.Title), items, nil
	case "feed":
		var a atom
		if err := dec.DecodeElement(&a, &root); err != nil {
			return "", nil, fmt.Errorf("лента %s: %w", src.URL, err)
		}
		for _, e := range a.Entries {
			link := ""
			for _, l := range e.Links {
				if l.Rel == "" || l.Rel == "alternate" {
					link = l.Href
					break
				}
			}
			date := parseDate(e.Published)
			if date.IsZero() {
				date = parseDate(e.Updated)
			}
			body := e.Summary
			if body == "" {
				body = e.Content
			}
			items = append(items, Item{ID: itemID(src.URL, e.ID+e.Title), Source: src.Title, Title: clean(e.Title),
				Link: link, Summary: clean(body), Date: date})
		}
		return clean(a.Title), items, nil
	}
	return "", nil, fmt.Errorf("%s — не лента RSS или Atom", src.URL)
}

// charsetReader понимает windows-1251 и koi8-r — ими до сих пор пишут русские ленты.
func charsetReader(charset string, in io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "utf-8", "utf8", "":
		return in, nil
	case "windows-1251", "cp1251":
		return decode8(in, cp1251)
	case "koi8-r":
		return decode8(in, koi8r)
	}
	return nil, fmt.Errorf("кодировка %s не поддерживается", charset)
}

func decode8(in io.Reader, table *[128]rune) (io.Reader, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for _, c := range data {
		if c < 0x80 {
			b.WriteByte(c)
		} else {
			b.WriteRune(table[c-0x80])
		}
	}
	return strings.NewReader(b.String()), nil
}

// clean убирает разметку и лишние пробелы; длинное описание укорачивается.
func clean(s string) string {
	s = web.StripTags(s)
	if r := []rune(s); len(r) > 1200 {
		s = string(r[:1200]) + "…"
	}
	return s
}

// merge объединяет новости лент: свежие первыми, повторы убираются.
func merge(lists ...[]Item) []Item {
	seen := map[string]bool{}
	var out []Item
	for _, l := range lists {
		for _, it := range l {
			if seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out
}

var cp1251 = &[128]rune{
	'Ђ', 'Ѓ', '‚', 'ѓ', '„', '…', '†', '‡', '€', '‰', 'Љ', '‹', 'Њ', 'Ќ', 'Ћ', 'Џ',
	'ђ', '‘', '’', '“', '”', '•', '–', '—', '\uFFFD', '™', 'љ', '›', 'њ', 'ќ', 'ћ', 'џ',
	'\u00A0', 'Ў', 'ў', 'Ј', '¤', 'Ґ', '¦', '§', 'Ё', '©', 'Є', '«', '¬', '\u00AD', '®', 'Ї',
	'°', '±', 'І', 'і', 'ґ', 'µ', '¶', '·', 'ё', '№', 'є', '»', 'ј', 'Ѕ', 'ѕ', 'ї',
	'А', 'Б', 'В', 'Г', 'Д', 'Е', 'Ж', 'З', 'И', 'Й', 'К', 'Л', 'М', 'Н', 'О', 'П',
	'Р', 'С', 'Т', 'У', 'Ф', 'Х', 'Ц', 'Ч', 'Ш', 'Щ', 'Ъ', 'Ы', 'Ь', 'Э', 'Ю', 'Я',
	'а', 'б', 'в', 'г', 'д', 'е', 'ж', 'з', 'и', 'й', 'к', 'л', 'м', 'н', 'о', 'п',
	'р', 'с', 'т', 'у', 'ф', 'х', 'ц', 'ч', 'ш', 'щ', 'ъ', 'ы', 'ь', 'э', 'ю', 'я',
}

var koi8r = &[128]rune{
	'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼', '▀', '▄', '█', '▌', '▐',
	'░', '▒', '▓', '⌠', '■', '∙', '√', '≈', '≤', '≥', '\u00A0', '⌡', '°', '²', '·', '÷',
	'═', '║', '╒', 'ё', '╓', '╔', '╕', '╖', '╗', '╘', '╙', '╚', '╛', '╜', '╝', '╞',
	'╟', '╠', '╡', 'Ё', '╢', '╣', '╤', '╥', '╦', '╧', '╨', '╩', '╪', '╫', '╬', '©',
	'ю', 'а', 'б', 'ц', 'д', 'е', 'ф', 'г', 'х', 'и', 'й', 'к', 'л', 'м', 'н', 'о',
	'п', 'я', 'р', 'с', 'т', 'у', 'ж', 'в', 'ь', 'ы', 'з', 'ш', 'э', 'щ', 'ч', 'ъ',
	'Ю', 'А', 'Б', 'Ц', 'Д', 'Е', 'Ф', 'Г', 'Х', 'И', 'Й', 'К', 'Л', 'М', 'Н', 'О',
	'П', 'Я', 'Р', 'С', 'Т', 'У', 'Ж', 'В', 'Ь', 'Ы', 'З', 'Ш', 'Э', 'Щ', 'Ч', 'Ъ',
}
