package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadPacketParsesTwoPacketsInARow(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Encode(Packet{Cmd: 0x196, ID1: 3, Body: []byte("hi")}))
	buf.Write(Encode(Packet{Cmd: 0x1a0, ID1: 3}))

	p1, err := ReadPacket(&buf)
	if err != nil || p1.Cmd != 0x196 || string(p1.Body) != "hi" {
		t.Fatalf("first = %+v, err = %v", p1, err)
	}
	p2, err := ReadPacket(&buf)
	if err != nil || p2.Cmd != 0x1a0 || len(p2.Body) != 0 {
		t.Fatalf("second = %+v, err = %v", p2, err)
	}
	if _, err := ReadPacket(&buf); !errors.Is(err, io.EOF) {
		t.Fatalf("third err = %v, want EOF", err)
	}
}

func TestReadPacketRejectsOversizedBody(t *testing.T) {
	hdr := Encode(Packet{Cmd: 0x4b0})
	// підміняємо size на MaxBodySize+1
	hdr[0], hdr[1], hdr[2], hdr[3] = 0x01, 0x00, 0x10, 0x00
	if _, err := ReadPacket(bytes.NewReader(hdr)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestReadPacketTruncatedBodyIsError(t *testing.T) {
	raw := Encode(Packet{Cmd: 0x4b0, Body: []byte{1, 2, 3, 4}})
	if _, err := ReadPacket(bytes.NewReader(raw[:HeaderSize+2])); err == nil {
		t.Fatal("err = nil, want unexpected EOF")
	}
}
