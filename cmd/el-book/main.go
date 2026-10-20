// El-book — оболочка на книге. Её запускает scripts/device/launch.sh, когда Nickel уже
// остановлен; по коду выхода скрипт выключает книгу, перезагружает её или возвращает Nickel
// (коды — в sdk/app: ExitNickel, ExitPowerOff, ExitReboot, ExitCrash).
//
//	el-book -root /mnt/onboard/.adds/el-book -log /mnt/onboard/.adds/el-book/logs/el-book.log
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"

	// Корни TLS на случай, если в прошивке книги их нет: используются, только когда
	// системных корней не нашлось.
	_ "golang.org/x/crypto/x509roots/fallback"

	"el-book/internal/builtin"
	"el-book/sdk/app"
	"el-book/sdk/device/kobo"
	"el-book/sdk/gfx"
	"el-book/sdk/storage"
	"el-book/shell"
)

// version подставляется при сборке: -ldflags "-X main.version=…".
var version = "dev"

// logLimit — размер журнала, после которого при запуске он уходит в .1.
const logLimit = 1 << 20

func main() {
	root := flag.String("root", "/mnt/onboard/.adds/el-book", "папка платформы на книге")
	logPath := flag.String("log", "", "файл журнала; пусто — в stderr")
	showVersion := flag.Bool("version", false, "напечатать версию и выйти")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	out, closeLog := openLog(*logPath)
	defer closeLog()
	log := slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo}))
	log.Info("запуск", "версия", version)
	os.Exit(run(*root, log))
}

func run(root string, log *slog.Logger) (code int) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("падение оболочки", "panic", r, "stack", string(debug.Stack()))
			code = app.ExitCrash
		}
	}()
	dev, err := kobo.Open(log, root)
	if err != nil {
		log.Error("устройство", "err", err)
		return app.ExitCrash
	}
	defer dev.Close()
	info := dev.Info()
	log.Info("устройство", "модель", info.Model, "кодовое имя", info.Codename, "прошивка", info.Firmware)
	for _, in := range dev.Inputs() {
		log.Info("ввод", "путь", in.Path, "имя", in.Name, "назначение", in.Kind, "x", in.AbsX, "y", in.AbsY)
	}
	for _, w := range dev.Warnings() {
		log.Warn(w)
	}

	store, err := storage.Open(filepath.Join(root, "data"))
	if err != nil {
		log.Error("данные", "err", err)
		return app.ExitCrash
	}
	fonts, err := gfx.DefaultFonts()
	if err != nil {
		log.Error("шрифты", "err", err)
		return app.ExitCrash
	}
	code = app.Run(app.Config{
		Device: dev,
		// Wi-Fi на книге выключен, пока не пройден спайк его включения: включается в настройках.
		System:    shell.New(shell.Options{Apps: builtin.Apps(), Version: version}),
		Store:     store,
		Log:       log,
		Fonts:     fonts,
		UserAgent: "el-book/" + version,
	})
	log.Info("выход", "код", code)
	return code
}

// openLog открывает журнал на дописывание; слишком большой уходит в .1. Падения рантайма
// Go (паника в чужой горутине, fatal error) тоже пишутся в журнал.
func openLog(path string) (io.Writer, func()) {
	if path == "" {
		return os.Stderr, func() {}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "el-book: журнал:", err)
		return os.Stderr, func() {}
	}
	if st, err := os.Stat(path); err == nil && st.Size() > logLimit {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "el-book: журнал:", err)
		return os.Stderr, func() {}
	}
	_ = debug.SetCrashOutput(f, debug.CrashOptions{})
	return f, func() { f.Close() }
}
