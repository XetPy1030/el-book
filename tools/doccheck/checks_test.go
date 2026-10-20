package main

import (
	"os"
	"strings"
	"testing"
)

// mapSource — источник в памяти.
type mapSource map[string]string

func (m mapSource) Name() string { return "тест" }

func (m mapSource) Files() ([]string, error) {
	var files []string
	for f := range m {
		files = append(files, f)
	}
	return files, nil
}

func (m mapSource) Read(path string) ([]byte, error) {
	s, ok := m[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(s), nil
}

func header(title, typ, extra string) string {
	return "# " + title + "\n\n**Тип:** " + typ + " · **Аудитория:** все · **Статус документа:** живой · **Актуально на:** 19.10.2026\n" +
		extra + "\n**Когда читать:** всегда.\n\n---\n\n"
}

// validRepo — минимальный репозиторий без нарушений.
func validRepo() mapSource {
	return mapSource{
		"CLAUDE.md":             "[Правило](docs/rules/Правило.md)\n",
		"docs/README.md":        "- [Правило](rules/Правило.md)\n- [Объяснение](explanation/Как%20устроено.md)\n",
		"docs/rules/Правило.md": header("Правило", "правила", "") + "Смотри [механизм](../explanation/Как%20устроено.md#14-перенос--итог).\n",
		"docs/explanation/Как устроено.md": header("Как устроено", "объяснение", "") +
			"## 14. Перенос — итог\n\nКод: `sdk/app/app.go` → `Run()`.\n",
		"sdk/app/app.go": "package app\n\nfunc Run() {}\n",
	}
}

func check(t *testing.T, src mapSource) []Issue {
	t.Helper()
	r, err := LoadRepo(src)
	if err != nil {
		t.Fatal(err)
	}
	return Check(r)
}

func TestValidRepo(t *testing.T) {
	if issues := check(t, validRepo()); len(issues) > 0 {
		t.Fatalf("ожидалось без нарушений, получено: %v", issues)
	}
}

func TestViolations(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(m mapSource)
		check string
	}{
		{"нет в индексе", func(m mapSource) {
			m["docs/reference/Справка.md"] = header("Справка", "справочник", "")
		}, "индекс"},
		{"правило не названо в инструкции", func(m mapSource) {
			m["docs/rules/Второе.md"] = header("Второе", "правила", "")
			m["docs/README.md"] += "- [Второе](rules/Второе.md)\n"
		}, "инструкция"},
		{"битая ссылка", func(m mapSource) {
			m["docs/rules/Правило.md"] += "[нет](../nope.md)\n"
		}, "ссылка"},
		{"нет якоря", func(m mapSource) {
			m["docs/rules/Правило.md"] += "[нет](../explanation/Как%20устроено.md#другое)\n"
		}, "ссылка"},
		{"номер строки в прозе", func(m mapSource) {
			m["docs/rules/Правило.md"] += "См. `sdk/app/app.go:3`.\n"
		}, "номер-строки"},
		{"номер строки в ссылке наружу", func(m mapSource) {
			m["docs/rules/Правило.md"] += "[x](https://github.com/a/b/blob/c/d.go#L10)\n"
		}, "номер-строки"},
		{"якорь кода не найден", func(m mapSource) {
			m["docs/rules/Правило.md"] += "`sdk/app/app.go` → `Stop()`\n"
		}, "код"},
		{"файл кода не найден", func(m mapSource) {
			m["docs/rules/Правило.md"] += "`sdk/nope.go` → `X`\n"
		}, "код"},
		{"H1 не совпадает с именем", func(m mapSource) {
			m["docs/rules/Правило.md"] = strings.Replace(m["docs/rules/Правило.md"], "# Правило", "# Другое", 1)
		}, "шапка"},
		{"тип не совпадает с каталогом", func(m mapSource) {
			m["docs/rules/Правило.md"] = strings.Replace(m["docs/rules/Правило.md"], "правила", "справочник", 1)
		}, "шапка"},
		{"снимок без даты снятия", func(m mapSource) {
			m["docs/rules/Правило.md"] = strings.Replace(m["docs/rules/Правило.md"], "живой", "снимок", 1)
		}, "шапка"},
		{"руководство без правил", func(m mapSource) {
			m["docs/guides/Как сделать.md"] = header("Как сделать", "руководство", "")
			m["docs/README.md"] += "- [Как сделать](guides/Как%20сделать.md)\n"
		}, "шапка"},
		{"ADR ссылается на правило", func(m mapSource) {
			m["docs/adr/ADR-0001 Решение.md"] = header("ADR-0001 Решение", "ADR", "**Статус решения:** принято\n") + "[правило](../rules/Правило.md)\n"
			m["docs/README.md"] += "- [ADR](adr/ADR-0001%20Решение.md)\n"
		}, "adr-правило"},
		{"правило длиннее бюджета", func(m mapSource) {
			m["docs/rules/Правило.md"] += strings.Repeat("строка\n", ruleBudget)
		}, "бюджет"},
		{"документ вне каталога типа", func(m mapSource) {
			m["docs/Лишний.md"] = header("Лишний", "справочник", "")
			m["docs/README.md"] += "- [Лишний](Лишний.md)\n"
		}, "шапка"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := validRepo()
			c.edit(src)
			issues := check(t, src)
			for _, i := range issues {
				if i.Check == c.check {
					return
				}
			}
			t.Fatalf("ожидалось нарушение %q, получено: %v", c.check, issues)
		})
	}
}

func TestIgnoresFencedCodeAndExternalCode(t *testing.T) {
	src := validRepo()
	src["docs/rules/Правило.md"] += "```markdown\n`чужой/path.go` → `X` и `file.go:12`\n```\n" +
		"[`fbink.c` → `refresh()`](https://github.com/NiLuJe/FBInk/blob/abc/fbink.c)\n"
	if issues := check(t, src); len(issues) > 0 {
		t.Fatalf("ожидалось без нарушений, получено: %v", issues)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"14. Перенос в другой репозиторий":       "14-перенос-в-другой-репозиторий",
		"Фаза 1 — Спайки на устройстве":          "фаза-1--спайки-на-устройстве",
		"rmkit и vlasovsoft — образцы платформы": "rmkit-и-vlasovsoft--образцы-платформы",
		"`ADR` и snake_case": "adr-и-snake_case",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDuplicateHeadingAnchors(t *testing.T) {
	a := anchors("## Итог\n\n## Итог\n")
	if !a["итог"] || !a["итог-1"] {
		t.Fatalf("ожидались якоря итог и итог-1, получено %v", a)
	}
}
