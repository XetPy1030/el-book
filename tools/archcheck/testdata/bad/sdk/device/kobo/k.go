package kobo

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

var _ = unsafe.Sizeof(unix.Stat_t{})
