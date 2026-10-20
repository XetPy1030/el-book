//go:build !(linux && arm)

package kobo

import (
	"errors"
	"log/slog"
)

// InputInfo — что найдено среди устройств ввода; нужно диагностике.
type InputInfo struct {
	Path, Name, Kind string
	AbsX, AbsY       [2]int32
}

// Open на компьютере недоступна: системная часть слоя Kobo собирается только для книги.
func Open(*slog.Logger, string) (*Device, error) {
	return nil, errors.New("устройство Kobo доступно только на книге (linux/arm)")
}
