package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

var ErrTooLarge = errors.New("protocol: body exceeds max packet size")

// ReadPacket читає один кадр. Повертає io.EOF, якщо потік завершився
// рівно на межі пакета.
func ReadPacket(r io.Reader) (Packet, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Packet{}, err
	}
	size := binary.LittleEndian.Uint32(hdr[0:4])
	if size > MaxBodySize {
		return Packet{}, ErrTooLarge
	}
	p := Packet{
		Cmd: binary.LittleEndian.Uint16(hdr[4:6]),
		ID1: binary.LittleEndian.Uint32(hdr[6:10]),
		ID2: binary.LittleEndian.Uint32(hdr[10:14]),
	}
	if size > 0 {
		p.Body = make([]byte, size)
		if _, err := io.ReadFull(r, p.Body); err != nil {
			return Packet{}, err
		}
	}
	return p, nil
}
