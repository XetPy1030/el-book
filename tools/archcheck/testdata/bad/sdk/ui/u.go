package ui

import (
	"unsafe"

	"example/bad/apps/x"
)

var X = unsafe.Sizeof(x.Y)
