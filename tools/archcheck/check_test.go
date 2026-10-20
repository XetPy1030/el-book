package main

import (
	"sort"
	"strings"
	"testing"
)

func TestCheckFindsViolations(t *testing.T) {
	vs, err := Check("testdata/bad")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.String())
	}
	want := []string{
		"apps/x/x.go:6: правило 5: apps/x импортирует sdk/device/emu: приложение не знает реализаций устройства",
		"apps/x/x.go:8: правило 5: apps/x импортирует shell: приложение импортирует только sdk/...",
		"apps/x/x.go:16: правило 11: голый go в приложении: фоновая работа — через ctx.Go",
		"apps/x/x.go:17: правило 13: os.WriteFile: файлы пишутся только через sdk/storage",
		"apps/x/x.go:19: правило 13: os.OpenFile: файлы пишутся только через sdk/storage",
		"internal/foo/foo_linux_arm.go:1: правило 4: файл под книгу вне sdk/device/kobo",
		"sdk/ui/u.go:4: правило 4: unsafe — только в sdk/device/kobo",
		"sdk/ui/u.go:6: правило 6: sdk/ui импортирует apps/x: SDK не зависит от оболочки, приложений и сборки",
		"shell/s.go:3: правило 1: импорт \"C\" запрещён: сборка без cgo",
		"shell/s.go:8: правило 7: shell импортирует sdk/device/kobo: реализацию устройства выбирает только cmd/...",
		"shell/s.go:13: правило 13: os.Rename: файлы пишутся только через sdk/storage",
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("нарушения:\n%s\n\nожидались:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRepositoryIsClean(t *testing.T) {
	vs, err := Check("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		t.Error(v)
	}
}
