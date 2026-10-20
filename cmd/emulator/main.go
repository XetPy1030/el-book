// Emulator запускает оболочку на эмулированной книге: в браузере (по умолчанию) или по
// скрипту без браузера, сохраняя снимки экрана.
//
//	go run ./cmd/emulator                       → http://localhost:8740
//	go run ./cmd/emulator -script s.txt -out d  → снимки d/<имя>.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"el-book/internal/builtin"
	"el-book/sdk/app"
	"el-book/sdk/device/emu"
	"el-book/sdk/gfx"
	"el-book/sdk/storage"
	"el-book/shell"
)

var version = "dev"

func main() {
	addr := flag.String("addr", "localhost:8740", "адрес веб-интерфейса")
	data := flag.String("data", "build/emu-data", "папка данных эмулированной книги")
	fresh := flag.Bool("fresh", false, "начать с пустыми данными")
	script := flag.String("script", "", "скрипт действий; без браузера")
	out := flag.String("out", "build/shots", "куда сохранять снимки скрипта")
	timing := flag.Bool("timing", false, "ждать обновления панели столько, сколько на книге")
	landscape := flag.Bool("landscape", false, "начать в альбомной ориентации")
	frozen := flag.String("clock", "", "заморозить часы, например 2026-10-19T14:05 (для снимков)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if *fresh {
		os.RemoveAll(*data)
	}
	store, err := storage.Open(*data)
	if err != nil {
		fail(err)
	}
	fonts, err := gfx.DefaultFonts()
	if err != nil {
		fail(err)
	}
	dev := emu.New(emu.Options{SimulateTiming: *timing, AutoWake: *script != ""})
	if *landscape {
		dev.Display().SetRotation(1)
	}
	clock := time.Now
	if *frozen != "" {
		t, err := time.ParseInLocation("2006-01-02T15:04", *frozen, time.Local)
		if err != nil {
			fail(fmt.Errorf("-clock: %w", err))
		}
		clock = func() time.Time { return t }
	}
	cfg := app.Config{
		Device: dev,
		System: shell.New(shell.Options{Apps: builtin.Apps(), Version: version, WifiDefault: true}),
		Store:  store, Log: log, Fonts: fonts, Clock: clock,
		UserAgent: "el-book/" + version + " (эмулятор)",
	}

	if *script != "" {
		go runScript(dev, *script, *out)
	} else {
		go func() {
			log.Info("эмулятор запущен", "адрес", "http://"+*addr)
			if err := http.ListenAndServe(*addr, dev.Handler()); err != nil {
				fail(err)
			}
		}()
	}
	code := app.Run(cfg)
	log.Info("оболочка завершилась", "код", code)
	os.Exit(code)
}

func runScript(dev *emu.Emulator, path, out string) {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := os.MkdirAll(out, 0o755); err != nil {
		fail(err)
	}
	dev.WaitIdle(200*time.Millisecond, 5*time.Second) // первый кадр
	err = dev.RunScript(f, func(name string, img *image.Gray) error {
		p := filepath.Join(out, name+".png")
		w, err := os.Create(p)
		if err != nil {
			return err
		}
		defer w.Close()
		fmt.Println(p)
		return png.Encode(w, img)
	})
	if err != nil {
		fail(err)
	}
	os.Exit(0)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "emulator:", err)
	os.Exit(1)
}
