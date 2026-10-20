package ink

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

// Двоичный формат штрихов: в несколько раз меньше JSON и читается без разбора текста.
//
//	"INK" версия(1)  число штрихов(uvarint)
//	штрих: кисть(1) тон(1) флаги(1) толщина×16(uvarint) зерно(uvarint) число точек(uvarint)
//	точка: Δx×8(varint) Δy×8(varint) [толщина×100(1), если флаг 1]
//
// Координаты — в восьмых долях пикселя от предыдущей точки штриха.
const (
	codecVersion = 1
	coordScale   = 8
	flagPressure = 1
)

var magic = []byte("INK")

// ErrFormat — данные не похожи на штрихи или испорчены.
var ErrFormat = errors.New("штрихи повреждены")

// Marshal записывает штрихи в двоичный формат.
func Marshal(strokes []*Stroke) []byte {
	size := 8
	for _, s := range strokes {
		size += 12 + len(s.Pts)*5
	}
	b := make([]byte, 0, size)
	b = append(b, magic...)
	b = append(b, codecVersion)
	b = binary.AppendUvarint(b, uint64(len(strokes)))
	for _, s := range strokes {
		flags := byte(0)
		if s.Brush.Dynamic() {
			flags |= flagPressure
		}
		b = append(b, byte(s.Brush), s.Tone, flags)
		b = binary.AppendUvarint(b, uint64(math.Round(float64(s.Width)*16)))
		b = binary.AppendUvarint(b, uint64(s.Seed))
		b = binary.AppendUvarint(b, uint64(len(s.Pts)))
		var px, py int64
		for _, p := range s.Pts {
			x, y := int64(math.Round(float64(p.X)*coordScale)), int64(math.Round(float64(p.Y)*coordScale))
			b = binary.AppendVarint(b, x-px)
			b = binary.AppendVarint(b, y-py)
			px, py = x, y
			if flags&flagPressure != 0 {
				b = append(b, byte(min(max(math.Round(float64(p.P)*100), 0), 255)))
			}
		}
	}
	return b
}

// Unmarshal читает штрихи из двоичного формата.
func Unmarshal(data []byte) ([]*Stroke, error) {
	if len(data) < len(magic)+1 || string(data[:len(magic)]) != string(magic) {
		return nil, ErrFormat
	}
	if v := data[len(magic)]; v > codecVersion {
		return nil, fmt.Errorf("%w: формат версии %d новее этой программы", ErrFormat, v)
	}
	r := reader{b: data[len(magic)+1:]}
	n := r.uvarint()
	if n > uint64(len(r.b)) {
		return nil, ErrFormat
	}
	out := make([]*Stroke, 0, n)
	for i := uint64(0); i < n && r.err == nil; i++ {
		s := &Stroke{Brush: Brush(r.byte()), Tone: r.byte()}
		flags := r.byte()
		s.Width = float32(r.uvarint()) / 16
		s.Seed = uint32(r.uvarint())
		np := r.uvarint()
		if np > uint64(len(r.b)) {
			return nil, ErrFormat
		}
		s.Pts = make([]Point, 0, np)
		var px, py int64
		for j := uint64(0); j < np && r.err == nil; j++ {
			px += r.varint()
			py += r.varint()
			p := Point{X: float32(px) / coordScale, Y: float32(py) / coordScale, P: 1}
			if flags&flagPressure != 0 {
				p.P = float32(r.byte()) / 100
			}
			s.Pts = append(s.Pts, p)
		}
		out = append(out, s)
	}
	if r.err != nil {
		return nil, r.err
	}
	return out, nil
}

// reader читает поля и запоминает первую ошибку.
type reader struct {
	b   []byte
	err error
}

func (r *reader) byte() byte {
	if r.err != nil || len(r.b) == 0 {
		r.err = ErrFormat
		return 0
	}
	v := r.b[0]
	r.b = r.b[1:]
	return v
}

func (r *reader) uvarint() uint64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Uvarint(r.b)
	if n <= 0 {
		r.err = ErrFormat
		return 0
	}
	r.b = r.b[n:]
	return v
}

func (r *reader) varint() int64 {
	if r.err != nil {
		return 0
	}
	v, n := binary.Varint(r.b)
	if n <= 0 {
		r.err = ErrFormat
		return 0
	}
	r.b = r.b[n:]
	return v
}
