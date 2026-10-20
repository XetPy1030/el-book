// Doccheck проверяет документацию на соответствие docs/rules/Стандарт документации.md:
// шапки, индекс, ссылки между документами, ссылки на код, номера строк, бюджет правил,
// файл-инструкцию. Читает файлы из коммита (по умолчанию), индекса или рабочего дерева.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	src := flag.String("src", "head", "откуда читать файлы: head, index (для хука перед коммитом) или worktree")
	flag.Parse()

	// Проверка привязана к корню репозитория, а не к каталогу запуска.
	root, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		fail(err)
	}
	if err := os.Chdir(strings.TrimSpace(string(root))); err != nil {
		fail(err)
	}

	source, err := NewSource(*src)
	if err != nil {
		fail(err)
	}
	repo, err := LoadRepo(source)
	if err != nil {
		fail(err)
	}
	issues := Check(repo)
	for _, i := range issues {
		fmt.Println(i)
	}
	if len(issues) > 0 {
		fmt.Fprintf(os.Stderr, "doccheck (%s): нарушений — %d\n", source.Name(), len(issues))
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "doccheck (%s): документация соответствует стандарту, документов — %d\n", source.Name(), len(repo.documents()))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "doccheck:", err)
	os.Exit(2)
}
