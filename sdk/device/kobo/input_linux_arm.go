package kobo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"unsafe"

	"golang.org/x/sys/unix"
)

// inputDev — открытое устройство /dev/input/event*.
type inputDev struct {
	Path       string
	Name       string
	Kind       inputKind
	AbsX, AbsY inputAbsInfo
	f          *os.File
}

// openInputs открывает все устройства ввода, кроме сырого акселерометра, и захватывает их
// (EVIOCGRAB), чтобы события не уходили другим процессам. Ioctl идут через SyscallConn:
// File.Fd перевёл бы дескриптор в блокирующий режим, и Close перестал бы прерывать чтение.
func openInputs() ([]*inputDev, error) {
	paths, _ := filepath.Glob("/dev/input/event*")
	sort.Strings(paths)
	var out []*inputDev
	for _, p := range paths {
		f, err := os.OpenFile(p, os.O_RDONLY, 0)
		if err != nil {
			continue
		}
		rc, err := f.SyscallConn()
		if err != nil {
			f.Close()
			continue
		}
		d := &inputDev{Path: p, f: f}
		var name [evIOCGNameLen]byte
		var hasX, hasY bool
		_ = rc.Control(func(fd uintptr) {
			if ioctlPtr(fd, evIOCGName, unsafe.Pointer(&name[0])) == nil {
				if i := bytes.IndexByte(name[:], 0); i >= 0 {
					d.Name = string(name[:i])
				}
			}
			hasX = ioctlPtr(fd, evIOCGAbsBase+absMTPositionX, unsafe.Pointer(&d.AbsX)) == nil && d.AbsX.Maximum > 0
			hasY = ioctlPtr(fd, evIOCGAbsBase+absMTPositionY, unsafe.Pointer(&d.AbsY)) == nil && d.AbsY.Maximum > 0
		})
		d.Kind = classifyInput(d.Name, hasX && hasY)
		if d.Kind == inputSkip {
			f.Close()
			continue
		}
		_ = rc.Control(func(fd uintptr) {
			_, _, _ = unix.Syscall(unix.SYS_IOCTL, fd, evIOCGrab, 1)
		})
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("нет устройств ввода в /dev/input")
	}
	return out, nil
}
