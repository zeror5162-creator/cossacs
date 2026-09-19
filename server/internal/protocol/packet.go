// Package protocol кодує й декодує пакети лобі-протоколу Cossacks 3.
//
// Формат перенесено з cossacks3-lan-server (c) 2018 Ereb, MIT.
// Заголовок 14 байт little-endian: size uint32 (довжина тіла), cmd uint16,
// id1 uint32, id2 uint32. Далі size байтів тіла.
package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	HeaderSize  = 14
	MaxBodySize = 0x100000
)

// LengthType — скільки байтів займає префікс довжини рядка.
type LengthType int

const (
	LenByte LengthType = iota
	LenShort
	LenInt
)

var ErrOverrun = errors.New("protocol: read past end of body")

type Packet struct {
	Cmd  uint16
	ID1  uint32
	ID2  uint32
	Body []byte
}

func Encode(p Packet) []byte {
	out := make([]byte, HeaderSize+len(p.Body))
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(p.Body)))
	binary.LittleEndian.PutUint16(out[4:6], p.Cmd)
	binary.LittleEndian.PutUint32(out[6:10], p.ID1)
	binary.LittleEndian.PutUint32(out[10:14], p.ID2)
	copy(out[HeaderSize:], p.Body)
	return out
}

type Reader struct {
	body []byte
	pos  int
	err  error
}

func NewReader(body []byte) *Reader { return &Reader{body: body} }

func (r *Reader) Err() error { return r.err }

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.body) {
		r.err = ErrOverrun
		return nil
	}
	b := r.body[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *Reader) Byte() uint8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *Reader) Short() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (r *Reader) Int() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *Reader) Skip(n int) { r.take(n) }

func (r *Reader) String(lt LengthType) string {
	var n int
	switch lt {
	case LenByte:
		n = int(r.Byte())
	case LenShort:
		n = int(r.Short())
	case LenInt:
		n = int(r.Int())
	}
	b := r.take(n)
	if b == nil {
		return ""
	}
	return string(b)
}

type Writer struct{ buf []byte }

func NewWriter() *Writer { return &Writer{} }

func (w *Writer) Byte(b uint8) { w.buf = append(w.buf, b) }

func (w *Writer) Short(v uint16) {
	w.buf = binary.LittleEndian.AppendUint16(w.buf, v)
}

func (w *Writer) Int(v uint32) {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, v)
}

func (w *Writer) Raw(b []byte) { w.buf = append(w.buf, b...) }

func (w *Writer) String(s string, lt LengthType) {
	switch lt {
	case LenByte:
		w.Byte(uint8(len(s)))
	case LenShort:
		w.Short(uint16(len(s)))
	case LenInt:
		w.Int(uint32(len(s)))
	}
	w.buf = append(w.buf, s...)
}

func (w *Writer) Len() int { return len(w.buf) }

// PatchInt перезаписує 4 байти за зсувом off уже записаним значенням.
// Потрібно для 0x1bd, де довжина блоку пишеться на початку, а відома в кінці.
func (w *Writer) PatchInt(off int, v uint32) {
	binary.LittleEndian.PutUint32(w.buf[off:off+4], v)
}

func (w *Writer) Packet(cmd uint16, id1, id2 uint32) Packet {
	return Packet{Cmd: cmd, ID1: id1, ID2: id2, Body: w.buf}
}
