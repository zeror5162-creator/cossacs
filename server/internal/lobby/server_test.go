package lobby

import (
	"testing"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

func packet(cmd uint16, id1, id2 uint32, body []byte) protocol.Packet {
	return protocol.Packet{Cmd: cmd, ID1: id1, ID2: id2, Body: body}
}

func login(t *testing.T, s *Server, c *fakeClient, nick string) {
	t.Helper()
	s.Connect(c)
	w := protocol.NewWriter()
	w.String("1.0.0.7", protocol.LenByte)
	w.String("2.0.7", protocol.LenByte)
	w.String("mail@example.com", protocol.LenByte)
	w.String("", protocol.LenByte)
	w.String(nick, protocol.LenByte)
	s.Handle(c, w.Packet(0x19a, c.ID(), 0))
}

func loginAll(t *testing.T, s *Server, cs ...*fakeClient) {
	t.Helper()
	for i, c := range cs {
		login(t, s, c, string(rune('A'+i))+"player")
	}
}

func createRoom(t *testing.T, s *Server, c *fakeClient) {
	t.Helper()
	w := protocol.NewWriter()
	w.Int(8)
	w.Byte(0)
	w.String(`"room"\t""\t008C7`, protocol.LenByte)
	w.String("0", protocol.LenByte)
	w.Int(42)
	w.Short(0)
	s.Handle(c, w.Packet(0x19c, c.ID(), 0))
}

func joinRoom(t *testing.T, s *Server, c *fakeClient, hostID uint32) {
	t.Helper()
	w := protocol.NewWriter()
	w.Int(hostID)
	s.Handle(c, w.Packet(0x19e, c.ID(), 0))
}

func TestConnectAssignsIncrementingIDs(t *testing.T) {
	s := New("test")
	a, b := newFakeClient("1.1.1.1"), newFakeClient("2.2.2.2")
	s.Connect(a)
	s.Connect(b)
	if a.ID() != 1 || b.ID() != 2 {
		t.Fatalf("ids = %d, %d; want 1, 2", a.ID(), b.ID())
	}
}

func TestSendPropagateInRoomFromHostGoesToOthers(t *testing.T) {
	s := New("test")
	host, guest, other := newFakeClient("h"), newFakeClient("g"), newFakeClient("o")
	loginAll(t, s, host, guest, other)
	createRoom(t, s, host)
	joinRoom(t, s, guest, host.ID())

	host.sent, guest.sent, other.sent = nil, nil, nil
	s.Handle(host, packet(0x4b0, host.ID(), 0, []byte{9, 9}))

	if len(guest.sent) != 1 || guest.sent[0].Cmd != 0x4b0 {
		t.Fatalf("guest got %v, want one 0x4b0", guest.cmds())
	}
	if len(host.sent) != 0 {
		t.Fatalf("host got %v, want nothing", host.cmds())
	}
	if len(other.sent) != 0 {
		t.Fatalf("player outside room got %v, want nothing", other.cmds())
	}
}

func TestSendPropagateInRoomFromGuestGoesToHostOnly(t *testing.T) {
	s := New("test")
	host, g1, g2 := newFakeClient("h"), newFakeClient("g1"), newFakeClient("g2")
	loginAll(t, s, host, g1, g2)
	createRoom(t, s, host)
	joinRoom(t, s, g1, host.ID())
	joinRoom(t, s, g2, host.ID())

	host.sent, g1.sent, g2.sent = nil, nil, nil
	s.Handle(g1, packet(0x4b0, g1.ID(), 0, []byte{1}))

	if len(host.sent) != 1 {
		t.Fatalf("host got %v, want one packet", host.cmds())
	}
	if len(g2.sent) != 0 {
		t.Fatalf("other guest got %v, want nothing", g2.cmds())
	}
}
