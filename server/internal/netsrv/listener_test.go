package netsrv

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/zeror5162-creator/cossacs/server/internal/lobby"
	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

func dialAndLogin(t *testing.T, addr, nick string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	w := protocol.NewWriter()
	w.String("1.0.0.7", protocol.LenByte)
	w.String("2.0.7", protocol.LenByte)
	w.String("a@b.c", protocol.LenByte)
	w.String("", protocol.LenByte)
	w.String(nick, protocol.LenByte)
	if _, err := conn.Write(protocol.Encode(w.Packet(0x19a, 0, 0))); err != nil {
		t.Fatalf("write: %v", err)
	}
	return conn
}

func TestLoginOverRealSocketReturns19b(t *testing.T) {
	lb := lobby.New("test")
	l, err := Listen(context.Background(), "127.0.0.1:0", lb, Options{})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	conn := dialAndLogin(t, l.Addr(), "Alpha")
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	p, err := protocol.ReadPacket(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if p.Cmd != 0x19b {
		t.Fatalf("cmd = %#x, want 0x19b", p.Cmd)
	}
}

func TestOversizedPacketClosesConnectionAndServerSurvives(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb, Options{})
	defer l.Close()

	bad, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// заголовок із розміром 0x100001
	bad.Write([]byte{0x01, 0x00, 0x10, 0x00, 0xb0, 0x04, 0, 0, 0, 0, 0, 0, 0, 0})
	bad.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := protocol.ReadPacket(bad); err == nil {
		t.Fatal("connection stayed open after oversized packet")
	}
	bad.Close()

	// сервер продовжує працювати
	good := dialAndLogin(t, l.Addr(), "Beta")
	defer good.Close()
	good.SetReadDeadline(time.Now().Add(2 * time.Second))
	if p, err := protocol.ReadPacket(good); err != nil || p.Cmd != 0x19b {
		t.Fatalf("second client: p = %+v, err = %v", p, err)
	}
}

func TestConnectionsPerIPAreLimited(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb, Options{MaxConnPerIP: 2})
	defer l.Close()

	first := dialAndLogin(t, l.Addr(), "One")
	defer first.Close()
	second := dialAndLogin(t, l.Addr(), "Two")
	defer second.Close()

	third, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer third.Close()
	third.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := third.Read(buf); err == nil {
		t.Fatal("third connection from same IP was accepted")
	}

	// наявні з'єднання не постраждали
	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	if p, err := protocol.ReadPacket(first); err != nil || p.Cmd != 0x19b {
		t.Fatalf("first connection broken: p = %+v, err = %v", p, err)
	}
}

func TestIdleConnectionIsClosedAfterLoginTimeout(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb,
		Options{LoginTimeout: 200 * time.Millisecond})
	defer l.Close()

	conn, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("idle connection was not closed")
	}
}
