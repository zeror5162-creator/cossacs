package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeHeaderIsLittleEndian(t *testing.T) {
	got := Encode(Packet{Cmd: 0x19b, ID1: 2, ID2: 2, Body: []byte{0xAA}})
	want := []byte{
		0x01, 0x00, 0x00, 0x00, // size = 1
		0x9b, 0x01, // cmd
		0x02, 0x00, 0x00, 0x00, // id1
		0x02, 0x00, 0x00, 0x00, // id2
		0xAA,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode() = % x, want % x", got, want)
	}
}

func TestWriterStringLengthPrefixes(t *testing.T) {
	w := NewWriter()
	w.String("ab", LenByte)
	w.String("cd", LenShort)
	w.String("ef", LenInt)
	got := w.Packet(0x1a6, 1, 0).Body
	want := []byte{
		0x02, 'a', 'b',
		0x02, 0x00, 'c', 'd',
		0x02, 0x00, 0x00, 0x00, 'e', 'f',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("body = % x, want % x", got, want)
	}
}

func TestReaderRoundTrip(t *testing.T) {
	w := NewWriter()
	w.Int(8)
	w.String(`"room"\t""\t008C7`, LenByte)
	w.Byte(7)
	r := NewReader(w.Packet(0x19c, 1, 0).Body)
	if got := r.Int(); got != 8 {
		t.Fatalf("Int() = %d, want 8", got)
	}
	if got := r.String(LenByte); got != `"room"\t""\t008C7` {
		t.Fatalf("String() = %q", got)
	}
	if got := r.Byte(); got != 7 {
		t.Fatalf("Byte() = %d, want 7", got)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestReaderOverrunSetsError(t *testing.T) {
	r := NewReader([]byte{0x05, 'a'}) // каже 5 байтів, а є 1
	if got := r.String(LenByte); got != "" {
		t.Fatalf("String() = %q, want empty", got)
	}
	if r.Err() == nil {
		t.Fatal("Err() = nil, want overrun error")
	}
}
