package main

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	docsDir     = "docs/"
	indexFile   = "docs/README.md"
	instruction = "CLAUDE.md"
	rulesDir    = "docs/rules/"
	adrDir      = "docs/adr/"
	ruleBudget  = 120
)

// typeByDir — какой тип документа живёт в каталоге (§1 и §9 стандарта).
var typeByDir = map[string]string{
	"rules":       "правила",
	"adr":         "ADR",
	"reference":   "справочник",
	"explanation": "объяснение",
	"guides":      "руководство",
	"decisions":   "решение",
	"journal":     "журнал",
	"external":    "внешний материал",
}

// extraField — строка шапки, обязательная для типа (§3).
var extraField = map[string]string{
	"ADR":              "Статус решения",
	"руководство":      "Правила",
	"внешний материал": "Источник",
}

var statuses = map[string]bool{"живой": true, "снимок": true, "заморожен": true, "архив": true}

// budgetExempt — правила, которым разрешено превышать бюджет; каждое записано в реестре долга.
var budgetExempt = map[string]bool{
	"docs/rules/Стандарт документации.md": true,
}

// Issue — одно нарушение стандарта.
type Issue struct {
	File  string
	Line  int
	Check string
	Msg   string
}

func (i Issue) String() string {
	if i.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", i.File, i.Line, i.Check, i.Msg)
	}
	return fmt.Sprintf("%s: %s: %s", i.File, i.Check, i.Msg)
}

// Repo — снимок репозитория из одного источника.
type Repo struct {
	src     Source
	files   map[string]bool
	dirs    map[string]bool
	content map[string]string
}

func LoadRepo(src Source) (*Repo, error) {
	list, err := src.Files()
	if err != nil {
		return nil, err
	}
	r := &Repo{src: src, files: map[string]bool{}, dirs: map[string]bool{}, content: map[string]string{}}
	for _, f := range list {
		r.files[f] = true
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			r.dirs[d] = true
		}
	}
	return r, nil
}

func (r *Repo) read(p string) (string, error) {
	if c, ok := r.content[p]; ok {
		return c, nil
	}
	b, err := r.src.Read(p)
	if err != nil {
		return "", err
	}
	r.content[p] = string(b)
	return r.content[p], nil
}

func (r *Repo) markdown() []string {
	var out []string
	for f := range r.files {
		if strings.HasSuffix(f, ".md") {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// documents — документы под docs/, кроме индекса.
func (r *Repo) documents() []string {
	var out []string
	for _, f := range r.markdown() {
		if strings.HasPrefix(f, docsDir) && f != indexFile {
			out = append(out, f)
		}
	}
	return out
}

// Check прогоняет все проверки и возвращает нарушения по порядку файлов.
func Check(r *Repo) []Issue {
	var issues []Issue
	for _, f := range r.markdown() {
		c, err := r.read(f)
		if err != nil {
			issues = append(issues, Issue{File: f, Check: "чтение", Msg: err.Error()})
			continue
		}
		issues = append(issues, checkLinks(r, f, c)...)
		issues = append(issues, checkLineNumbers(f, c)...)
		issues = append(issues, checkCodeRefs(r, f, c)...)
	}
	for _, f := range r.documents() {
		c, _ := r.read(f)
		issues = append(issues, checkHeader(f, c)...)
		if strings.HasPrefix(f, rulesDir) && !budgetExempt[f] {
			if n := strings.Count(strings.TrimRight(c, "\n"), "\n") + 1; n > ruleBudget {
				issues = append(issues, Issue{File: f, Check: "бюджет", Msg: fmt.Sprintf("правило длиннее %d строк", ruleBudget)})
			}
		}
	}
	issues = append(issues, checkLinkedFrom(r, indexFile, "индекс", r.documents())...)
	var rules []string
	for _, f := range r.documents() {
		if strings.HasPrefix(f, rulesDir) {
			rules = append(rules, f)
		}
	}
	issues = append(issues, checkLinkedFrom(r, instruction, "инструкция", rules)...)
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		return issues[i].Line < issues[j].Line
	})
	return issues
}

// checkLinks: цель ссылки существует, якорь есть среди заголовков, ADR не ссылается на правила.
func checkLinks(r *Repo, from, content string) []Issue {
	var issues []Issue
	for _, l := range links(content) {
		if isExternal(l.Target) {
			if lineAnchorRe.MatchString(l.Target) {
				issues = append(issues, Issue{File: from, Line: l.Line, Check: "номер-строки", Msg: "ссылка на номер строки: " + l.Target})
			}
			continue
		}
		file, anchor, ok := resolve(from, l.Target)
		if !ok {
			issues = append(issues, Issue{File: from, Line: l.Line, Check: "ссылка", Msg: "ссылка выходит за корень репозитория: " + l.Target})
			continue
		}
		if !r.files[file] && !r.dirs[file] {
			issues = append(issues, Issue{File: from, Line: l.Line, Check: "ссылка", Msg: fmt.Sprintf("нет файла в %s: %s", r.src.Name(), l.Target)})
			continue
		}
		if strings.HasPrefix(from, adrDir) && strings.HasPrefix(file, rulesDir) {
			issues = append(issues, Issue{File: from, Line: l.Line, Check: "adr-правило", Msg: "ADR не ссылается на правила: " + l.Target})
		}
		if anchor == "" || !strings.HasSuffix(file, ".md") {
			continue
		}
		target, err := r.read(file)
		if err != nil {
			continue
		}
		if !anchors(target)[strings.ToLower(anchor)] {
			issues = append(issues, Issue{File: from, Line: l.Line, Check: "ссылка", Msg: fmt.Sprintf("нет заголовка #%s в %s", anchor, file)})
		}
	}
	return issues
}

var (
	lineRefRe    = regexp.MustCompile(`[\w./-]+\.[A-Za-z]{1,6}:\d+`)
	lineAnchorRe = regexp.MustCompile(`#L\d+`)
	urlRe        = regexp.MustCompile(`https?://\S+`)
	codeRefRe    = regexp.MustCompile("`([^`\\s]+)`\\s*→\\s*`([^`]+)`")
	extLinkRe    = regexp.MustCompile(`\[[^\]]*\]\(https?://[^)]*\)`)
	headerRe     = regexp.MustCompile(`\*\*([^*]+):\*\*\s*([^·]*)`)
	dateRe       = regexp.MustCompile(`^\d{2}\.\d{2}\.\d{4}$`)
	headerKeys   = []string{"Тип", "Аудитория", "Статус документа", "Когда читать"}
)

// checkLineNumbers: форма «путь:номер» в прозе запрещена (§5).
func checkLineNumbers(file, content string) []Issue {
	var issues []Issue
	for _, l := range proseLines(content) {
		text := urlRe.ReplaceAllString(l.Text, "")
		if m := lineRefRe.FindString(text); m != "" {
			issues = append(issues, Issue{File: file, Line: l.N, Check: "номер-строки", Msg: "ссылка на номер строки: " + m})
		}
	}
	return issues
}

// checkCodeRefs: «`путь` → `якорь`» — файл есть в источнике, якорь находится в нём поиском (§5).
// Ссылки на чужой код оформлены ссылками наружу и не проверяются.
func checkCodeRefs(r *Repo, file, content string) []Issue {
	var issues []Issue
	for _, l := range proseLines(content) {
		text := extLinkRe.ReplaceAllString(l.Text, "")
		for _, m := range codeRefRe.FindAllStringSubmatch(text, -1) {
			p, anchor := m[1], strings.TrimSuffix(m[2], "()")
			if !r.files[p] {
				issues = append(issues, Issue{File: file, Line: l.N, Check: "код", Msg: fmt.Sprintf("нет файла %s в %s", p, r.src.Name())})
				continue
			}
			c, err := r.read(p)
			if err != nil || !strings.Contains(c, anchor) {
				issues = append(issues, Issue{File: file, Line: l.N, Check: "код", Msg: fmt.Sprintf("якорь %q не найден в %s", anchor, p)})
			}
		}
	}
	return issues
}

// checkHeader: H1 = имя файла, шапка заполнена, тип совпадает с каталогом (§3, §9).
func checkHeader(file, content string) []Issue {
	issue := func(msg string) Issue { return Issue{File: file, Check: "шапка", Msg: msg} }
	lines := strings.Split(content, "\n")
	want := strings.TrimSuffix(path.Base(file), ".md")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		return []Issue{issue("первая строка — не заголовок H1")}
	}
	var issues []Issue
	if h1 := strings.ReplaceAll(strings.TrimPrefix(lines[0], "# "), "`", ""); h1 != want {
		issues = append(issues, issue(fmt.Sprintf("H1 %q не совпадает с именем файла %q", h1, want)))
	}
	fields := map[string]string{}
	closed := false
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			closed = true
			break
		}
		for _, m := range headerRe.FindAllStringSubmatch(l, -1) {
			fields[m[1]] = strings.TrimSpace(m[2])
		}
	}
	if !closed {
		return append(issues, issue("шапка не закрыта строкой ---"))
	}
	for _, k := range headerKeys {
		if fields[k] == "" {
			issues = append(issues, issue("нет поля «"+k+"»"))
		}
	}
	typ, status := fields["Тип"], fields["Статус документа"]
	if status != "" && !statuses[status] {
		issues = append(issues, issue("неизвестный статус «"+status+"»"))
	}
	dateKey := "Актуально на"
	if status == "снимок" {
		dateKey = "Снято"
	}
	if !dateRe.MatchString(fields[dateKey]) {
		issues = append(issues, issue("нет даты ДД.ММ.ГГГГ в поле «"+dateKey+"»"))
	}
	if extra, ok := extraField[typ]; ok && fields[extra] == "" {
		issues = append(issues, issue("для типа «"+typ+"» нужно поле «"+extra+"»"))
	}
	dir := strings.SplitN(strings.TrimPrefix(file, docsDir), "/", 2)[0]
	switch wantType, known := typeByDir[dir]; {
	case dir == "archive":
		if status != "архив" {
			issues = append(issues, issue("в archive/ статус документа должен быть «архив»"))
		}
	case !known:
		issues = append(issues, issue("документ вне каталога типа (rules, adr, reference, …)"))
	case typ != wantType:
		issues = append(issues, issue(fmt.Sprintf("в каталоге %s/ тип «%s», а не «%s»", dir, wantType, typ)))
	}
	return issues
}

// checkLinkedFrom: каждый файл из want упомянут ссылкой в owner.
func checkLinkedFrom(r *Repo, owner, check string, want []string) []Issue {
	content, err := r.read(owner)
	if err != nil {
		return []Issue{{File: owner, Check: check, Msg: "файла нет в " + r.src.Name()}}
	}
	linked := map[string]bool{}
	for _, l := range links(content) {
		if f, _, ok := resolve(owner, l.Target); ok {
			linked[f] = true
		}
	}
	var issues []Issue
	for _, f := range want {
		if !linked[f] {
			issues = append(issues, Issue{File: owner, Check: check, Msg: "нет ссылки на " + f})
		}
	}
	return issues
}
