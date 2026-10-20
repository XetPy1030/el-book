package kobo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"el-book/sdk/device"
)

// fbDriver — системные вызовы фреймбуфера. На книге — ioctl и mmap над /dev/fb0
// (fb_linux_arm.go), в тестах — подделка, которая ведёт себя как ядро Forma.
type fbDriver interface {
	VarInfo() (varScreenInfo, error)
	// PutVarInfo записывает параметры; ядро возвращает в v то, что установило на самом деле.
	PutVarInfo(v *varScreenInfo) error
	FixInfo() (fixScreenInfo, error)
	// Map отображает память фреймбуфера; повторный вызов заменяет прежнее отображение.
	Map(size int) ([]byte, error)
	SendUpdate(u *mxcfbUpdateDataV2) error
	WaitUpdate(m *mxcfbUpdateMarkerData) error
	Close() error
}

// display — экран e-ink поверх fbDriver. Поворот делает контроллер: фреймбуфер переводится
// в нужную ориентацию, и логические координаты совпадают с координатами памяти.
type display struct {
	drv    fbDriver
	model  *Model
	mu     sync.Mutex
	mem    []byte
	vinfo  varScreenInfo
	finfo  fixScreenInfo
	rot    device.Rotation
	start  device.Rotation // поворот до запуска: возвращается при закрытии для Nickel
	marker uint32
	sent   time.Time // последнее обновление панели: от него сон отсчитывает паузу
	lut32  [256]uint32
}

func newDisplay(drv fbDriver, m *Model) (*display, error) {
	d := &display{drv: drv, model: m}
	for g := range d.lut32 {
		v := uint32(g)
		d.lut32[g] = 0xFF000000 | v<<16 | v<<8 | v
	}
	if err := d.reload(true); err != nil {
		return nil, err
	}
	if r, ok := m.CanonicalFromRead(d.vinfo.Rotate); ok {
		d.rot, d.start = r, r
	}
	return d, nil
}

// reload перечитывает параметры фреймбуфера и заново отображает память, если нужно больше
// прежнего или если remap: после поворота некоторые драйверы привязывают поворот к отображению
// (так делает KOReader).
func (d *display) reload(remap bool) error {
	v, err := d.drv.VarInfo()
	if err != nil {
		return fmt.Errorf("параметры экрана: %w", err)
	}
	f, err := d.drv.FixInfo()
	if err != nil {
		return fmt.Errorf("параметры памяти экрана: %w", err)
	}
	switch v.BitsPerPixel {
	case 8, 16, 32:
	default:
		return fmt.Errorf("глубина цвета %d бит не поддерживается", v.BitsPerPixel)
	}
	need := int(f.LineLength) * int(v.YResVirtual)
	size := int(f.SmemLen)
	if size < need {
		size = need
	}
	if remap || len(d.mem) < need {
		mem, err := d.drv.Map(size)
		if err != nil {
			return fmt.Errorf("отобразить память экрана: %w", err)
		}
		d.mem = mem
	}
	d.vinfo, d.finfo = v, f
	return nil
}

func (d *display) Bounds() image.Rectangle {
	d.mu.Lock()
	defer d.mu.Unlock()
	return image.Rect(0, 0, int(d.vinfo.XRes), int(d.vinfo.YRes))
}

func (d *display) DPI() int { return d.model.DPI }

// describe — параметры фреймбуфера для диагностики.
func (d *display) describe() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, f := d.vinfo, d.finfo
	return fmt.Sprintf("%d×%d (виртуально %d×%d), %d бит, строка %d байт, rotate %d, yoffset %d, память %d КБ",
		v.XRes, v.YRes, v.XResVirtual, v.YResVirtual, v.BitsPerPixel, f.LineLength, v.Rotate, v.YOffset, f.SmemLen/1024)
}

func (d *display) Rotation() device.Rotation {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.rot
}

// SetRotation поворачивает фреймбуфер. Значение пишется с поправкой модели (у Forma нечётные
// повороты инвертируются при записи), затем проверяется чтением; если ядро поставило не то,
// перебираются все значения, пока прочитанное не совпадёт с нужным, — как в FBInk.
func (d *display) SetRotation(r device.Rotation) error {
	r &= 3
	d.mu.Lock()
	defer d.mu.Unlock()
	if r == d.rot {
		return nil
	}
	want := d.model.RotationRead[r]
	v := d.vinfo
	v.Rotate = d.model.WriteRotation(r)
	if err := d.drv.PutVarInfo(&v); err != nil {
		return fmt.Errorf("повернуть экран: %w", err)
	}
	got, err := d.drv.VarInfo()
	if err != nil {
		return fmt.Errorf("повернуть экран: %w", err)
	}
	for try := uint32(0); got.Rotate != want && try < 4; try++ {
		v.Rotate = try
		if err := d.drv.PutVarInfo(&v); err != nil {
			return fmt.Errorf("повернуть экран: %w", err)
		}
		if got, err = d.drv.VarInfo(); err != nil {
			return fmt.Errorf("повернуть экран: %w", err)
		}
	}
	if err := d.reload(true); err != nil {
		return err
	}
	if got.Rotate != want {
		if cur, ok := d.model.CanonicalFromRead(got.Rotate); ok {
			d.rot = cur
		}
		return fmt.Errorf("экран не повернулся: нужно %d, ядро держит %d", want, got.Rotate)
	}
	d.rot = r
	return nil
}

// Present копирует область в память фреймбуфера и отправляет обновление контроллеру.
func (d *display) Present(src *image.Gray, u device.Update) (device.Marker, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	bounds := image.Rect(0, 0, int(d.vinfo.XRes), int(d.vinfo.YRes))
	if src.Rect != bounds {
		return 0, fmt.Errorf("кадр %v не совпадает с экраном %v", src.Rect, bounds)
	}
	r := minUpdateRect(u.Rect.Intersect(bounds), bounds)
	if r.Empty() {
		return 0, nil
	}
	d.blit(src, r)
	d.marker++
	if d.marker == 0 { // 0 у контроллера — «без маркера»
		d.marker = 1
	}
	req := buildUpdate(mxcfbRect{
		Top: uint32(r.Min.Y), Left: uint32(r.Min.X),
		Width: uint32(r.Dx()), Height: uint32(r.Dy()),
	}, u, d.marker)
	if err := d.drv.SendUpdate(&req); err != nil {
		return 0, fmt.Errorf("обновить экран %v %v: %w", r, u.Waveform, err)
	}
	d.sent = time.Now()
	return device.Marker(d.marker), nil
}

// lastUpdate — когда панель обновлялась последний раз.
func (d *display) lastUpdate() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.sent
}

// minUpdateRect растягивает область до 2×2: обновления шириной или высотой в пиксель
// роняют некоторые ядра (замечено KOReader и FBInk).
func minUpdateRect(r, bounds image.Rectangle) image.Rectangle {
	if r.Empty() {
		return r
	}
	if r.Dx() < 2 {
		r.Max.X = r.Min.X + 2
	}
	if r.Dy() < 2 {
		r.Max.Y = r.Min.Y + 2
	}
	if r.Max.X > bounds.Max.X {
		r = r.Add(image.Pt(bounds.Max.X-r.Max.X, 0))
	}
	if r.Max.Y > bounds.Max.Y {
		r = r.Add(image.Pt(0, bounds.Max.Y-r.Max.Y))
	}
	return r.Intersect(bounds)
}

// Wait ждёт окончания обновления. Маркер, которого контроллер уже не помнит (EINVAL),
// означает, что обновление давно закончилось.
func (d *display) Wait(m device.Marker) error {
	if m == 0 {
		return nil
	}
	req := mxcfbUpdateMarkerData{UpdateMarker: uint32(m)}
	err := d.drv.WaitUpdate(&req)
	if err == nil || errors.Is(err, syscall.EINVAL) {
		return nil
	}
	return fmt.Errorf("ждать обновления %d: %w", m, err)
}

// blit переводит оттенки серого в формат фреймбуфера. Строки берутся с учётом yoffset:
// контроллер сам прибавляет смещение панорамирования к области обновления.
func (d *display) blit(src *image.Gray, r image.Rectangle) {
	bpp := int(d.vinfo.BitsPerPixel) / 8
	line := int(d.finfo.LineLength)
	yoff := int(d.vinfo.YOffset)
	xoff := int(d.vinfo.XOffset)
	inverted := d.vinfo.Grayscale == 2
	for y := r.Min.Y; y < r.Max.Y; y++ {
		s := src.Pix[(y-src.Rect.Min.Y)*src.Stride+(r.Min.X-src.Rect.Min.X):][:r.Dx()]
		o := (y+yoff)*line + (r.Min.X+xoff)*bpp
		dst := d.mem[o : o+r.Dx()*bpp]
		switch bpp {
		case 4:
			blit32(dst, s, &d.lut32)
		case 2:
			blit16(dst, s)
		default:
			if inverted {
				for i, g := range s {
					dst[i] = ^g
				}
			} else {
				copy(dst, s)
			}
		}
	}
}

// blit32 пишет пиксели BGRA; по выровненной памяти — словами по 4 байта.
func blit32(dst, s []byte, lut *[256]uint32) {
	if len(s) == 0 {
		return
	}
	if uintptr(unsafe.Pointer(&dst[0]))%4 == 0 {
		w := unsafe.Slice((*uint32)(unsafe.Pointer(&dst[0])), len(s))
		for i, g := range s {
			w[i] = lut[g]
		}
		return
	}
	for i, g := range s {
		binary.LittleEndian.PutUint32(dst[4*i:], lut[g])
	}
}

func blit16(dst, s []byte) {
	for i, g := range s {
		v := uint16(g>>3)<<11 | uint16(g>>2)<<5 | uint16(g>>3)
		dst[2*i] = byte(v)
		dst[2*i+1] = byte(v >> 8)
	}
}

// Close возвращает поворот, с которым экран достался от Nickel (картинка на панели
// при этом не меняется), и освобождает фреймбуфер.
func (d *display) Close() error {
	_ = d.SetRotation(d.start)
	return d.drv.Close()
}
