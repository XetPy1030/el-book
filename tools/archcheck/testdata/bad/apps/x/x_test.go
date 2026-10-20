package x

import (
	"os"
	"testing"

	"example/bad/sdk/device/emu"
)

func TestX(t *testing.T) {
	go func() {}()
	_ = os.WriteFile("a", nil, 0o644)
	_ = emu.X
}
