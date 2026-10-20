package kobo

import (
	"el-book/sdk/device"
)

// Структуры и номера ioctl фреймбуфера Linux и e-ink-контроллера i.MX (EPDC) Mark 7,
// перенесённые как факты из linux/fb.h и eink/mxcfb-kobo.h проекта FBInk.

// fixScreenInfo — struct fb_fix_screeninfo на 32-битном ARM (unsigned long — 4 байта).
type fixScreenInfo struct {
	ID           [16]byte
	SmemStart    uint32
	SmemLen      uint32
	Type         uint32
	TypeAux      uint32
	Visual       uint32
	XPanStep     uint16
	YPanStep     uint16
	YWrapStep    uint16
	_            uint16
	LineLength   uint32
	MmioStart    uint32
	MmioLen      uint32
	Accel        uint32
	Capabilities uint16
	Reserved     [2]uint16
}

type bitfield struct {
	Offset, Length, MsbRight uint32
}

// varScreenInfo — struct fb_var_screeninfo.
type varScreenInfo struct {
	XRes, YRes               uint32
	XResVirtual, YResVirtual uint32
	XOffset, YOffset         uint32
	BitsPerPixel             uint32
	Grayscale                uint32
	Red, Green, Blue, Transp bitfield
	Nonstd                   uint32
	Activate                 uint32
	Height, Width            uint32
	AccelFlags               uint32
	Pixclock                 uint32
	LeftMargin, RightMargin  uint32
	UpperMargin, LowerMargin uint32
	HsyncLen, VsyncLen       uint32
	Sync, Vmode, Rotate      uint32
	Colorspace               uint32
	Reserved                 [4]uint32
}

type mxcfbRect struct {
	Top, Left, Width, Height uint32
}

type mxcfbAltBufferData struct {
	PhysAddr        uint32
	Width, Height   uint32
	AltUpdateRegion mxcfbRect
}

// mxcfbUpdateDataV2 — struct mxcfb_update_data (v2) для MXCFB_SEND_UPDATE_V2 на Mark 7.
type mxcfbUpdateDataV2 struct {
	UpdateRegion  mxcfbRect
	WaveformMode  uint32
	UpdateMode    uint32
	UpdateMarker  uint32
	Temp          int32
	Flags         uint32
	DitherMode    int32
	QuantBit      int32
	AltBufferData mxcfbAltBufferData
}

// mxcfbUpdateMarkerData — struct mxcfb_update_marker_data для MXCFB_WAIT_FOR_UPDATE_COMPLETE_V3.
type mxcfbUpdateMarkerData struct {
	UpdateMarker  uint32
	CollisionTest uint32
}

// Номера ioctl собраны как _IOC(dir, type, nr, size) из linux/ioctl.h.
const (
	iocWrite = 1
	iocRead  = 2

	fbioGetVScreenInfo = 0x4600
	fbioPutVScreenInfo = 0x4601
	fbioGetFScreenInfo = 0x4602

	sizeofUpdateDataV2 = 72
	sizeofUpdateMarker = 8
	sizeofInputAbsInfo = 24

	mxcfbSendUpdateV2 = iocWrite<<30 | sizeofUpdateDataV2<<16 | 'F'<<8 | 0x2E           // 0x4048462e
	mxcfbWaitUpdateV3 = (iocRead|iocWrite)<<30 | sizeofUpdateMarker<<16 | 'F'<<8 | 0x2F // 0xc008462f

	evIOCGrab     = iocWrite<<30 | 4<<16 | 'E'<<8 | 0x90                 // EVIOCGRAB
	evIOCGNameLen = 256                                                  // буфер имени устройства
	evIOCGName    = iocRead<<30 | evIOCGNameLen<<16 | 'E'<<8 | 0x06      // EVIOCGNAME(256)
	evIOCGAbsBase = iocRead<<30 | sizeofInputAbsInfo<<16 | 'E'<<8 | 0x40 // EVIOCGABS(0); + код оси
)

// Параметры обновления EPDC.
const (
	tempUseAmbient      = 0x1000
	updateModePartial   = 0
	updateModeFull      = 1
	flagForceMonochrome = 0x02
	flagUseRegal        = 0x8000 // только для GLD16, которым платформа не пользуется
	ditherPassthrough   = 0
	waveformModeAuto    = 257
	ntxWaveformDU       = 1
	ntxWaveformGC16     = 2
	ntxWaveformA2       = 4
	ntxWaveformGL16     = 5
	ntxWaveformGLR16    = 6 // REAGL
)

// inputAbsInfo — struct input_absinfo.
type inputAbsInfo struct {
	Value, Minimum, Maximum, Fuzz, Flat, Resolution int32
}

// waveformCode — код режима NTX для waveform платформы.
func waveformCode(w device.Waveform) uint32 {
	switch w {
	case device.WaveformDU:
		return ntxWaveformDU
	case device.WaveformA2:
		return ntxWaveformA2
	case device.WaveformGL16:
		return ntxWaveformGL16
	case device.WaveformREAGL:
		return ntxWaveformGLR16
	case device.WaveformGC16:
		return ntxWaveformGC16
	}
	return waveformModeAuto
}

// buildUpdate заполняет запрос обновления так же, как FBInk для Mark 7: без аппаратного
// дизеринга; A2 — с FORCE_MONOCHROME; REAGL — только частичный. r — область во фреймбуфере.
func buildUpdate(r mxcfbRect, u device.Update, marker uint32) mxcfbUpdateDataV2 {
	wf := waveformCode(u.Waveform)
	mode := uint32(updateModePartial)
	if u.Full && u.Waveform != device.WaveformREAGL {
		mode = updateModeFull
	}
	var flags uint32
	if wf == ntxWaveformA2 {
		flags = flagForceMonochrome
	}
	return mxcfbUpdateDataV2{
		UpdateRegion: r,
		WaveformMode: wf,
		UpdateMode:   mode,
		UpdateMarker: marker,
		Temp:         tempUseAmbient,
		Flags:        flags,
		DitherMode:   ditherPassthrough,
		QuantBit:     0,
	}
}
