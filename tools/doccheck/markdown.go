package main

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Line — строка Markdown вне блоков кода, с номером для сообщений.
type Line struct {
	N    int
	Text string
}

// proseLines возвращает строки вне огороженных блоков кода (``` и ~~~).
func proseLines(content string) []Line {
	var out []Line
	fence := ""
	for i, l := range strings.Split(content, "\n") {
		t := strings.TrimSpace(l)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fence = t[:3]
			continue
		}
		out = append(out, Line{N: i + 1, Text: l})
	}
	return out
}

var (
	codeSpanRe = regexp.MustCompile("`[^`]*`")
	linkRe     = regexp.MustCompile(`\[[^\]]*\]\((?:<([^>]+)>|([^)\s]+))(?:\s+"[^"]*")?\)`)
	headingRe  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
)

// Link — ссылка Markdown: цель как написана и строка, где стоит.
type Link struct {
	Line   int
	Target string
}

// links находит ссылки вне блоков кода. Внутри `кода` ссылки не ищутся.
func links(content string) []Link {
	var out []Link
	for _, l := range proseLines(content) {
		text := codeSpanRe.ReplaceAllStringFunc(l.Text, func(s string) string {
			return strings.Repeat("x", len(s))
		})
		// Код в тексте ссылки ([`путь` → `якорь`](url)) затёрт, но скобки ссылки на месте.
		for _, m := range linkRe.FindAllStringSubmatchIndex(text, -1) {
			var target string
			if m[2] >= 0 {
				target = l.Text[m[2]:m[3]]
			} else {
				target = l.Text[m[4]:m[5]]
			}
			out = append(out, Link{Line: l.N, Target: target})
		}
	}
	return out
}

// isExternal — ссылка наружу, её существование не проверяется.
func isExternal(target string) bool {
	for _, p := range []string{"http://", "https://", "mailto:"} {
		if strings.HasPrefix(target, p) {
			return true
		}
	}
	return false
}

// resolve переводит относительную ссылку из файла from в путь от корня и якорь.
func resolve(from, target string) (file, anchor string, ok bool) {
	p, a, _ := strings.Cut(target, "#")
	if dp, err := url.PathUnescape(p); err == nil {
		p = dp
	}
	if da, err := url.PathUnescape(a); err == nil {
		a = da
	}
	if p == "" {
		return from, a, true
	}
	joined := path.Clean(path.Join(path.Dir(from), p))
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", "", false
	}
	return joined, a, true
}

// Heading — заголовок документа.
type Heading struct {
	Level int
	Text  string
}

func headings(content string) []Heading {
	var out []Heading
	for _, l := range proseLines(content) {
		if m := headingRe.FindStringSubmatch(l.Text); m != nil {
			out = append(out, Heading{Level: len(m[1]), Text: m[2]})
		}
	}
	return out
}

// slug строит якорь заголовка так же, как GitHub: строчные буквы, без пунктуации,
// пробелы → дефисы; повтор получает суффикс -1, -2…
func slug(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case unicode.IsLetter(r), unicode.IsMark(r), unicode.IsDigit(r), unicode.Is(unicode.Pc, r), r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

func anchors(content string) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	for _, h := range headings(content) {
		s := slug(h.Text)
		if n, dup := seen[s]; dup {
			seen[s] = n + 1
			s = s + "-" + strconv.Itoa(n+1)
		} else {
			seen[s] = 0
		}
		out[s] = true
	}
	return out
}
