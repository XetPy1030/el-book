package main

import (
	"os"

	"example/bad/sdk/device/kobo"
)

func main() {
	_ = kobo.X
	_ = os.WriteFile("a", nil, 0o644)
}
