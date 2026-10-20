package x

import (
	"os"

	"example/bad/sdk/device/emu"
	"example/bad/sdk/ui"
	"example/bad/shell"
)

var _ = emu.X
var _ = ui.X
var _ = shell.X

func run() {
	go func() {}()
	_ = os.WriteFile("a", nil, 0o644)
	_, _ = os.OpenFile("a", os.O_RDONLY, 0)
	_, _ = os.OpenFile("a", os.O_RDWR, 0)
	_, _ = os.ReadFile("a")
}
