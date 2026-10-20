package web

import (
	"html"
	"regexp"
	"strings"
)

// Para — абзац текста, извлечённого из веб-страницы.
type Para struct {
	Heading int  // 1…3 — заголовок этого уровня, 0 — обычный текст
	Item    bool // пункт списка
	Quote   bool // цитата
	Text    string
}

// Page — читаемый текст веб-страницы.
type Page struct {
	Title string
	Paras []Para
}

// skipTags — содержимое этих тегов не текст статьи.
var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "svg": true, "iframe": true, "form": true,
	"nav": true, "header": true, "footer": true, "aside": true, "button": true, "select": true,
	"template": true, "figure": true,
}

// blockTags — границы абзацев.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true, "main": true, "br": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"li": true, "ul": true, "ol": true, "blockquote": true, "pre": true, "tr": true, "table": true,
	"dd": true, "dt": true, "hr": true, "body": true,
}

var spaces = regexp.MustCompile(`[\s\x{00a0}]+`)

// ExtractText достаёт из HTML читаемый текст: заголовок и абзацы основного содержимого.
// Если на странице есть <article> или <main>, берётся только он; меню, подвалы, скрипты
// и формы отбрасываются. Это не полноценный разбор HTML, а быстрый проход для читалки
// на книге: для новостей и статей его хватает.
func ExtractText(doc []byte) Page {
	src := string(doc)
	var p Page
	p.Title = metaTitle(src)
	if p.Title == "" {
		p.Title = tagText(src, "title")
	}
	body := src
	for _, tag := range []string{"article", "main"} {
		if inner, ok := innerOf(src, tag); ok && len(strings.TrimSpace(inner)) > 200 {
			body = inner
			break
		}
	}
	p.Paras = paragraphs(body)
	// Короткие обрывки вне списков и заголовков — подписи, кнопки, даты: без них чище.
	kept := p.Paras[:0]
	for _, para := range p.Paras {
		if para.Heading == 0 && !para.Item && len([]rune(para.Text)) < 25 {
			continue
		}
		kept = append(kept, para)
	}
	p.Paras = kept
	// Заголовок страницы и первый заголовок статьи обычно об одном: «Статья — Сайт» или
	// укороченное название. Остаётся заголовок статьи, и он не повторяется в тексте.
	if len(p.Paras) > 0 && p.Paras[0].Heading > 0 && sameTitle(p.Title, p.Paras[0].Text) {
		p.Title = p.Paras[0].Text
		p.Paras = p.Paras[1:]
	}
	return p
}

// sameTitle — один заголовок входит в другой без учёта регистра.
func sameTitle(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	return a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a))
}

// paragraphs режет HTML на абзацы по блочным тегам.
func paragraphs(src string) []Para {
	var out []Para
	var buf strings.Builder
	cur := Para{}
	skip := 0
	quote := 0
	emit := func() {
		text := strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(buf.String()), " "))
		buf.Reset()
		if text != "" {
			cur.Text = text
			cur.Quote = cur.Quote || quote > 0
			out = append(out, cur)
		}
		cur = Para{}
	}
	for len(src) > 0 {
		lt := strings.IndexByte(src, '<')
		if lt < 0 {
			if skip == 0 {
				buf.WriteString(src)
			}
			break
		}
		if skip == 0 {
			buf.WriteString(src[:lt])
		}
		src = src[lt:]
		if strings.HasPrefix(src, "<!--") {
			end := strings.Index(src, "-->")
			if end < 0 {
				break
			}
			src = src[end+3:]
			continue
		}
		gt := strings.IndexByte(src, '>')
		if gt < 0 {
			break
		}
		tag := src[1:gt]
		src = src[gt+1:]
		closing := strings.HasPrefix(tag, "/")
		name := strings.ToLower(strings.TrimLeft(strings.Fields(strings.TrimPrefix(tag+" ", "/"))[0], "/"))
		name = strings.TrimSuffix(name, "/")
		if skipTags[name] {
			switch {
			case strings.HasSuffix(tag, "/"):
			case closing:
				skip = max(skip-1, 0)
			default:
				skip++
			}
			continue
		}
		if skip > 0 || !blockTags[name] {
			continue
		}
		emit()
		if name == "blockquote" {
			if closing {
				quote = max(quote-1, 0)
			} else {
				quote++
			}
		}
		if closing {
			continue
		}
		switch name {
		case "h1", "h2":
			cur.Heading = 1
		case "h3", "h4":
			cur.Heading = 2
		case "h5", "h6":
			cur.Heading = 3
		case "li":
			cur.Item = true
		}
	}
	emit()
	return out
}

// innerOf — содержимое первого тега tag.
func innerOf(src, tag string) (string, bool) {
	lower := strings.ToLower(src)
	start := strings.Index(lower, "<"+tag)
	for start >= 0 {
		after := lower[start+1+len(tag):]
		if after != "" && (after[0] == '>' || after[0] == ' ' || after[0] == '\n' || after[0] == '\t') {
			break
		}
		next := strings.Index(lower[start+1:], "<"+tag)
		if next < 0 {
			return "", false
		}
		start += 1 + next
	}
	if start < 0 {
		return "", false
	}
	open := strings.IndexByte(src[start:], '>')
	if open < 0 {
		return "", false
	}
	from := start + open + 1
	end := strings.LastIndex(lower, "</"+tag+">")
	if end < from {
		return src[from:], true
	}
	return src[from:end], true
}

// tagText — текст первого тега tag без разметки.
func tagText(src, tag string) string {
	inner, ok := innerOf(src, tag)
	if !ok {
		return ""
	}
	if end := strings.Index(strings.ToLower(inner), "</"+tag); end >= 0 {
		inner = inner[:end]
	}
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(stripTags(inner)), " "))
}

var metaRe = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]*>`)
var contentRe = regexp.MustCompile(`(?is)content=["']([^"']*)["']`)

func metaTitle(src string) string {
	m := metaRe.FindString(src)
	if m == "" {
		return ""
	}
	c := contentRe.FindStringSubmatch(m)
	if c == nil {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(c[1]))
}

var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// StripTags убирает разметку из фрагмента HTML и раскрывает сущности: для описаний в RSS.
func StripTags(s string) string {
	return strings.TrimSpace(spaces.ReplaceAllString(html.UnescapeString(stripTags(s)), " "))
}

func stripTags(s string) string { return tagRe.ReplaceAllString(s, " ") }
