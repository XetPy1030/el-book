package kobo

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fbDev — фреймбуфер /dev/fb0: ioctl и mmap.
type fbDev struct {
	fd  int
	mem []byte
}

func openFB(path string) (*fbDev, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("открыть %s: %w", path, err)
	}
	return &fbDev{fd: fd}, nil
}

// ioctl с указателем на структуру; arg держит объект живым на время вызова.
func ioctlPtr(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

func (f *fbDev) VarInfo() (v varScreenInfo, err error) {
	err = ioctlPtr(uintptr(f.fd), fbioGetVScreenInfo, unsafe.Pointer(&v))
	return v, err
}

func (f *fbDev) PutVarInfo(v *varScreenInfo) error {
	return ioctlPtr(uintptr(f.fd), fbioPutVScreenInfo, unsafe.Pointer(v))
}

func (f *fbDev) FixInfo() (v fixScreenInfo, err error) {
	err = ioctlPtr(uintptr(f.fd), fbioGetFScreenInfo, unsafe.Pointer(&v))
	return v, err
}

func (f *fbDev) Map(size int) ([]byte, error) {
	if f.mem != nil {
		_ = unix.Munmap(f.mem)
		f.mem = nil
	}
	mem, err := unix.Mmap(f.fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	f.mem = mem
	return mem, nil
}

func (f *fbDev) SendUpdate(u *mxcfbUpdateDataV2) error {
	return ioctlPtr(uintptr(f.fd), mxcfbSendUpdateV2, unsafe.Pointer(u))
}

func (f *fbDev) WaitUpdate(m *mxcfbUpdateMarkerData) error {
	return ioctlPtr(uintptr(f.fd), mxcfbWaitUpdateV3, unsafe.Pointer(m))
}

func (f *fbDev) Close() error {
	if f.mem != nil {
		_ = unix.Munmap(f.mem)
		f.mem = nil
	}
	return unix.Close(f.fd)
}
