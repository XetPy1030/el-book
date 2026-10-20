package hello

import (
	"bytes"
	"image"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"el-book/sdk/app"
	"el-book/sdk/device/emu"
	"el-book/sdk/gfx"
	"el-book/sdk/storage"
	"el-book/shell"
)

// Требование С-8: минимальное приложение — один файл короче пятидесяти строк.
func TestFitsInFiftyLines(t *testing.T) {
	src, err := os.ReadFile("hello.go")
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(src, []byte("\n")); n >= 50 {
		t.Fatalf("hello.go — %d строк; образец должен оставаться меньше пятидесяти", n)
	}
}

// Образец работает на платформе: плитка на домашнем экране, нажатие, счёт сохранён.
func TestRunsInEmulator(t *testing.T) {
	dev := emu.New(emu.Options{AutoWake: true})
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := gfx.DefaultFonts()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		done <- app.Run(app.Config{
			Device: dev,
			System: shell.New(shell.Options{Apps: []app.App{&App{}}, Version: "test"}),
			Store:  store,
			Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
			Fonts:  fonts,
		})
	}()
	defer func() {
		dev.Close()
		<-done
	}()
	dev.WaitIdle(200*time.Millisecond, 5*time.Second)
	script := "tap 260 850\nwait 300\ntap 720 1013\ntap 720 1013\nwait 300\n" // плитка, два нажатия
	if err := dev.RunScript(strings.NewReader(script), func(string, *image.Gray) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var count int
	if _, err := store.App("hello").LoadSettings(&count); err != nil || count != 2 {
		t.Fatalf("сохранённый счёт %d, %v; нужно 2", count, err)
	}
}
