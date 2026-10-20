package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type settings struct {
	Brightness int    `json:"brightness"`
	Name       string `json:"name"`
}

func TestSettingsRoundTripAndBackup(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := s.App("draw")
	var got settings
	if _, err := a.LoadSettings(&got); !errors.Is(err, ErrNoSettings) {
		t.Fatalf("ожидалось ErrNoSettings, получено %v", err)
	}
	if err := a.SaveSettings(1, settings{Brightness: 10, Name: "первый"}); err != nil {
		t.Fatal(err)
	}
	if err := a.SaveSettings(2, settings{Brightness: 20, Name: "второй"}); err != nil {
		t.Fatal(err)
	}
	v, err := a.LoadSettings(&got)
	if err != nil || v != 2 || got.Brightness != 20 {
		t.Fatalf("прочитано v=%d %+v, err=%v", v, got, err)
	}
	// Основной файл испорчен (например, отключилось питание) — читается предыдущая версия.
	path := filepath.Join(s.Root(), "apps", "draw", "settings.json")
	if err := os.WriteFile(path, []byte("{испорчено"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = settings{}
	v, err = a.LoadSettings(&got)
	if err != nil || v != 1 || got.Name != "первый" {
		t.Fatalf("резервная копия не подхватилась: v=%d %+v, err=%v", v, got, err)
	}
}

func TestFilesListRemoveAndNoEscape(t *testing.T) {
	s, _ := Open(t.TempDir())
	a := s.App("draw")
	if err := a.WriteFile("drawings/1.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := a.WriteFile("drawings/2.json", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	list, err := a.List("drawings")
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %+v, %v", list, err)
	}
	if err := a.Remove("drawings/1.json"); err != nil {
		t.Fatal(err)
	}
	if list, _ := a.List("drawings"); len(list) != 1 || list[0].Name != "2.json" {
		t.Fatalf("после удаления: %+v", list)
	}
	for _, bad := range []string{"../x", "/etc/passwd", "..", "a/../../b"} {
		if err := a.WriteFile(bad, nil); err == nil {
			t.Errorf("имя %q должно быть отвергнуто", bad)
		}
	}
	empty, err := s.App("none").List("")
	if err != nil || len(empty) != 0 {
		t.Fatalf("пустое хранилище: %+v, %v", empty, err)
	}
}

func TestExportAndNoTempLeftovers(t *testing.T) {
	s, _ := Open(t.TempDir())
	a := s.App("draw")
	rel, err := a.Export("рисунок.png", []byte("png"))
	if err != nil || rel != "export/draw/рисунок.png" {
		t.Fatalf("Export = %q, %v", rel, err)
	}
	entries, _ := os.ReadDir(filepath.Join(s.Root(), "export", "draw"))
	if len(entries) != 1 {
		t.Fatalf("в папке выгрузки лишние файлы: %v", entries)
	}
}
