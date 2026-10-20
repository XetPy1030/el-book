package shell

import "C"

import (
	files "os"

	"example/bad/sdk/device/kobo"
)

var X = kobo.X

func save() { _ = files.Rename("a", "b") }
