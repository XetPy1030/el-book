package kobo

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"el-book/sdk/device"
)

// InputInfo — что найдено среди устройств ввода; нужно диагностике.
type InputInfo struct {
	Path, Name, Kind string
	AbsX, AbsY       [2]int32 // минимум и максимум осей multitouch
}

// Open открывает книгу: модель по PRODUCT и /mnt/onboard/.kobo/version, фреймбуфер
// в портретной ориентации, все устройства ввода, подсветку, батарею, FIFO udev и Wi-Fi.
// dir — папка платформы: в ней скрипт wifi.sh и сети, добавленные в оболочке.
// В log слой пишет то, что выясняется по ходу работы: попытки сна, отказы ядра.
func Open(log *slog.Logger, dir string) (*Device, error) {
	number, firmware := "", ""
	if b, err := os.ReadFile("/mnt/onboard/.kobo/version"); err == nil {
		number, firmware = parseVersion(string(b))
	}
	m, err := Detect(strings.TrimSpace(os.Getenv("PRODUCT")), number)
	if err != nil {
		return nil, err
	}
	fb, err := openFB("/dev/fb0")
	if err != nil {
		return nil, err
	}
	disp, err := newDisplay(fb, m)
	if err != nil {
		fb.Close()
		return nil, err
	}
	if err := disp.SetRotation(device.Portrait); err != nil {
		disp.Close()
		return nil, err
	}
	d := newDevice(m, device.Info{Model: m.Name, Codename: m.Codename, Firmware: firmware}, disp, sysfs{root: "/"}, log)
	inputs, err := openInputs()
	if err != nil {
		d.Close()
		return nil, err
	}
	for _, in := range inputs {
		d.addCloser(in.f)
		d.inputs = append(d.inputs, InputInfo{
			Path: in.Path, Name: in.Name, Kind: in.Kind.String(),
			AbsX: [2]int32{in.AbsX.Minimum, in.AbsX.Maximum},
			AbsY: [2]int32{in.AbsY.Minimum, in.AbsY.Maximum},
		})
		handle := d.keysHandler()
		if in.Kind == inputTouch {
			rawW, rawH := touchScale(m, in.AbsX, in.AbsY)
			handle = d.touchHandler(rawW, rawH)
		}
		f := in.f
		d.run(func() { d.readEvents(f, handle) })
	}
	if err := d.watchHardwareStatus(hardwareStatusFIFO); err != nil {
		// Без FIFO не будет событий USB; книга работает и так.
		d.warnings = append(d.warnings, fmt.Sprintf("события USB недоступны: %v", err))
	}
	d.openWifi(dir, log)
	return d, nil
}

// openWifi включает управление Wi-Fi, если скрипт запуска передал модуль и интерфейс,
// а рядом с оболочкой лежит wifi.sh. Сам Wi-Fi при этом не включается.
func (d *Device) openWifi(dir string, log *slog.Logger) {
	module, iface := os.Getenv("WIFI_MODULE"), os.Getenv("INTERFACE")
	switch {
	case module == "" || iface == "":
		d.warnings = append(d.warnings, "Wi-Fi недоступен: скрипт запуска не передал WIFI_MODULE и INTERFACE")
		return
	case dir == "":
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "wifi.sh")); err != nil {
		d.warnings = append(d.warnings, fmt.Sprintf("Wi-Fi недоступен: %v", err))
		return
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	d.wifi = newWifi(dir, iface, d.trySend, log)
	d.addCloser(d.wifi)
}
