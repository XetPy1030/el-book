// Archcheck проверяет правила кода, которые можно проверить по исходникам
// (docs/rules/Правила кода.md): запрет cgo, слои пакетов, место кода под Kobo,
// голый go в приложениях и запись файлов в обход sdk/storage.
//
//	go run ./tools/archcheck            — проверить репозиторий
//	go run ./tools/archcheck -root dir  — проверить другое дерево (для тестов)
//
// Файлы разбираются go/parser все сразу, без учёта тегов сборки: так проверяется и код
// под книгу (*_linux_arm.go), который на компьютере не собирается.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	root := flag.String("root", "", "корень модуля; по умолчанию — корень репозитория")
	flag.Parse()
	dir := *root
	if dir == "" {
		out, err := exec.Command("go", "env", "GOMOD").Output()
		if err != nil {
			fail(fmt.Errorf("найти go.mod: %w", err))
		}
		dir = filepath.Dir(strings.TrimSpace(string(out)))
	}
	vs, err := Check(dir)
	if err != nil {
		fail(err)
	}
	for _, v := range vs {
		fmt.Println(v)
	}
	if len(vs) > 0 {
		fmt.Fprintf(os.Stderr, "archcheck: нарушений: %d (docs/rules/Правила кода.md)\n", len(vs))
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "archcheck:", err)
	os.Exit(2)
}
