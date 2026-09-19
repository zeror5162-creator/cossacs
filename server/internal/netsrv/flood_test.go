package netsrv

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/zeror5162-creator/cossacs/server/internal/lobby"
	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

// Клієнт, який не читає свій сокет, не має тягнути за собою інших:
// черга надсилань обмежена байтами, а не кількістю пакетів.
func TestSlowClientDoesNotDropReadingClient(t *testing.T) {
	lb := lobby.New("test")
	l, err := Listen(context.Background(), "127.0.0.1:0", lb,
		Options{SendQueueBytes: 64 * 1024})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	reader := dialAndLogin(t, l.Addr(), "Reader")
	defer reader.Close()
	stalled := dialAndLogin(t, l.Addr(), "Stalled")
	defer stalled.Close()
	sender := dialAndLogin(t, l.Addr(), "Sender")
	defer sender.Close()

	// читач постійно вичитує все, що йому шлють
	readErr := make(chan error, 1)
	go func() {
		for {
			reader.SetReadDeadline(time.Now().Add(5 * time.Second))
			if _, err := protocol.ReadPacket(reader); err != nil {
				readErr <- err
				return
			}
		}
	}()

	// відправник шле багато великих повідомлень у лобі
	body := make([]byte, 0, 60*1024)
	body = append(body, 0xff)
	body = append(body, make([]byte, 60*1024-1)...)
	for i := 0; i < 200; i++ {
		if _, err := sender.Write(protocol.Encode(
			protocol.Packet{Cmd: 0x196, ID1: 3, Body: body})); err != nil {
			break
		}
	}

	select {
	case err := <-readErr:
		t.Fatalf("reading client was disconnected: %v", err)
	case <-time.After(700 * time.Millisecond):
	}
	_ = stalled
}

// Повідомлення в лобі більше за будь-яке можливе від справжнього клієнта
// (префікс довжини рядка — 1 байт) не пересилається нікому.
func TestOversizedLobbyMessageIsDropped(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb, Options{})
	defer l.Close()

	victim := dialAndLogin(t, l.Addr(), "Victim")
	defer victim.Close()
	sender := dialAndLogin(t, l.Addr(), "Sender")
	defer sender.Close()

	drain(t, victim) // 0x19b + 0x1a6 від власного й чужого логіну

	sender.Write(protocol.Encode(protocol.Packet{
		Cmd: 0x196, ID1: 2, Body: make([]byte, 100*1024)}))

	// нормальний чат після нього має дійти — і бути першим, що прийшло
	sender.Write(protocol.Encode(protocol.Packet{
		Cmd: 0x196, ID1: 2, Body: []byte{2, 'h', 'i'}}))

	victim.SetReadDeadline(time.Now().Add(2 * time.Second))
	p, err := protocol.ReadPacket(victim)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if p.Cmd != 0x197 {
		t.Fatalf("cmd = %#x, want 0x197", p.Cmd)
	}
	if len(p.Body) != 3 {
		t.Fatalf("body len = %d, want 3 (oversized message was forwarded)", len(p.Body))
	}
}

// Пакет, дозволений до логіну, не має знімати таймаут логіну.
func TestPreLoginPacketDoesNotDisableLoginTimeout(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb,
		Options{LoginTimeout: 300 * time.Millisecond})
	defer l.Close()

	conn, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 0x1b7 сервер приймає до логіну, але гравцем це не робить
	conn.Write(protocol.Encode(protocol.Packet{Cmd: 0x1b7, Body: make([]byte, 8)}))

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("connection stayed open after login timeout")
	}
}

func drain(t *testing.T, conn net.Conn) {
	t.Helper()
	for {
		conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
		if _, err := protocol.ReadPacket(conn); err != nil {
			return
		}
	}
}
