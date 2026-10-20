// Package uitest прогоняет скрипты эмулятора (emulator/scripts) на оболочке со встроенными
// приложениями и сравнивает снимки панели с эталонами в testdata.
//
//	go test ./internal/uitest            — проверить
//	go test ./internal/uitest -update    — перезаписать эталоны после осознанного изменения интерфейса
package uitest

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"el-book/internal/builtin"
	"el-book/sdk/app"
	"el-book/sdk/device/emu"
	"el-book/sdk/gfx"
	"el-book/sdk/storage"
	"el-book/shell"
)

var update = flag.Bool("update", false, "перезаписать эталонные снимки")

// zone — часовой пояс тестов. Он же time.Local: приложения переводят сохранённые моменты
// в местное время, и снимки не должны зависеть от пояса машины, где идут тесты.
var zone = time.FixedZone("MSK", 3*60*60)

func TestMain(m *testing.M) {
	time.Local = zone
	os.Exit(m.Run())
}

// Допуск сравнения: сглаживание на разных процессорах может отличаться на единицы уровня.
const (
	pixelTolerance = 24     // разница уровня, которая считается совпадением
	maxDiffShare   = 0.0005 // доля несовпавших пикселей, которая ещё допустима
)

func TestScripts(t *testing.T) {
	if testing.Short() {
		t.Skip("UI-тесты идут несколько секунд; пропущены в -short")
	}
	t.Parallel()
	scripts, err := filepath.Glob("../../emulator/scripts/*.txt")
	if err != nil || len(scripts) == 0 {
		t.Fatalf("нет скриптов эмулятора: %v", err)
	}
	for _, path := range scripts {
		name := strings.TrimSuffix(filepath.Base(path), ".txt")
		// Скрипты независимы — у каждого свой эмулятор, хранилище и оболочка, — поэтому
		// идут параллельно: почти всё их время — паузы, пока приложение затихнет.
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runScript(t, path, name)
		})
	}
}

func runScript(t *testing.T, path, name string) {
	dev, fx := start(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	err = dev.RunScript(f, func(shot string, img *image.Gray) error {
		return compare(name, shot, img)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, url := range fx.Missed() {
		t.Errorf("запрос без ответа в testdata/http/routes.txt: %s", url)
	}
}

// start запускает оболочку со встроенными приложениями на своём эмуляторе: часы заморожены,
// ответы сети — из testdata/http. Оболочка остановится в конце теста.
func start(t *testing.T) (*emu.Emulator, *fixtures) {
	t.Helper()
	dev := emu.New(emu.Options{AutoWake: true})
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := gfx.DefaultFonts()
	if err != nil {
		t.Fatal(err)
	}
	fx, err := loadFixtures(filepath.Join("testdata", "http"))
	if err != nil {
		t.Fatal(err)
	}
	frozen := time.Date(2026, 10, 19, 14, 5, 0, 0, zone)
	cfg := app.Config{
		Device: dev,
		System: shell.New(shell.Options{Apps: builtin.Apps(), Version: "test", WifiDefault: true}),
		Store:  store,
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Fonts:  fonts,
		Clock:  func() time.Time { return frozen },
		HTTP:   fx,
	}
	done := make(chan int, 1)
	go func() { done <- app.Run(cfg) }()
	t.Cleanup(func() {
		dev.Close()
		<-done
	})
	dev.WaitIdle(200*time.Millisecond, 5*time.Second)
	return dev, fx
}

func compare(script, shot string, got *image.Gray) error {
	golden := filepath.Join("testdata", script, shot+".png")
	if *update {
		return writePNG(golden, got)
	}
	f, err := os.Open(golden)
	if err != nil {
		return fmt.Errorf("нет эталона %s (запустите с -update): %w", golden, err)
	}
	defer f.Close()
	decoded, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("эталон %s: %w", golden, err)
	}
	want := gfx.ToGray(decoded)
	if want.Bounds() != got.Bounds() {
		return fmt.Errorf("снимок %s: размер %v, эталон %v", shot, got.Bounds(), want.Bounds())
	}
	diff := image.NewGray(got.Bounds())
	bad := 0
	for i := range got.Pix {
		d := int(got.Pix[i]) - int(want.Pix[i])
		if d < 0 {
			d = -d
		}
		if d > pixelTolerance {
			bad++
			diff.Pix[i] = 0
		} else {
			diff.Pix[i] = 200 + want.Pix[i]/5
		}
	}
	if share := float64(bad) / float64(len(got.Pix)); share > maxDiffShare {
		out := filepath.Join("..", "..", "build", "uitest", script)
		_ = writePNG(filepath.Join(out, shot+".png"), got)
		_ = writePNG(filepath.Join(out, shot+"-diff.png"), diff)
		return fmt.Errorf("снимок %s отличается от эталона: %d пикселей (%.3f%%); фактический и разница — в %s",
			shot, bad, share*100, out)
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
