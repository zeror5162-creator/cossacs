package lobby

import (
	"testing"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

func TestCreateRoomNotifiesEveryoneWith19d(t *testing.T) {
	s := New("test")
	host, other := newFakeClient("h"), newFakeClient("o")
	loginAll(t, s, host, other)
	host.sent, other.sent = nil, nil

	createRoom(t, s, host)

	for _, c := range []*fakeClient{host, other} {
		if len(c.sent) != 1 || c.sent[0].Cmd != 0x19d {
			t.Fatalf("%s got %v, want one 0x19d", c.Addr(), c.cmds())
		}
	}
	r := protocol.NewReader(host.sent[0].Body)
	if b := r.Byte(); b != 7 {
		t.Fatalf("first byte = %d, want 7", b)
	}
	if v := r.Int(); v != 8 {
		t.Fatalf("int = %d, want 8", v)
	}
	if d := r.String(protocol.LenByte); d != `"room"\t""\t008C7` {
		t.Fatalf("desc = %q", d)
	}
}

func TestJoinNonexistentRoomIsIgnored(t *testing.T) {
	s := New("test")
	c := newFakeClient("c")
	loginAll(t, s, c)
	c.sent = nil

	joinRoom(t, s, c, 999)

	if len(c.sent) != 0 {
		t.Fatalf("got %v, want nothing", c.cmds())
	}
	if pl := s.playerForTest(c.ID()); pl.Room != nil {
		t.Fatal("player joined a room that does not exist")
	}
}

func TestStartGameHidesRoomFromNewcomers(t *testing.T) {
	s := New("test")
	host, guest := newFakeClient("h"), newFakeClient("g")
	loginAll(t, s, host, guest)
	createRoom(t, s, host)
	joinRoom(t, s, guest, host.ID())
	s.Handle(host, packet(0x1a2, host.ID(), 0, nil))

	newcomer := newFakeClient("n")
	login(t, s, newcomer, "Newbie")

	r := protocol.NewReader(newcomer.sent[0].Body)
	skipLobbyHeader(t, r)
	if r.Int() != 0 {
		t.Fatal("expected zero rooms for newcomer")
	}
}

func TestHostLeavingDuringGameTransfersHost(t *testing.T) {
	s := New("test")
	host, g1, g2 := newFakeClient("h"), newFakeClient("g1"), newFakeClient("g2")
	loginAll(t, s, host, g1, g2)
	createRoom(t, s, host)
	joinRoom(t, s, g1, host.ID())
	joinRoom(t, s, g2, host.ID())
	s.Handle(host, packet(0x1a2, host.ID(), 0, nil))
	host.sent, g1.sent, g2.sent = nil, nil, nil

	s.Handle(host, packet(0x1a0, host.ID(), 0, nil))

	newHost, others := g2, []*fakeClient{g1}
	if !hasCmd(newHost.cmds(), 0x1bd) {
		t.Fatalf("new host got %v, want 0x1bd", newHost.cmds())
	}
	for _, c := range others {
		if !hasCmd(c.cmds(), 0x1be) {
			t.Fatalf("%s got %v, want 0x1be", c.Addr(), c.cmds())
		}
	}
	if got := s.Snapshot().Rooms; got != 0 {
		t.Fatalf("rooms = %d, want 0 (old room deleted)", got)
	}
}

func hasCmd(cmds []uint16, want uint16) bool {
	for _, c := range cmds {
		if c == want {
			return true
		}
	}
	return false
}

// skipLobbyHeader перемотує 0x19b до списку кімнат.
func skipLobbyHeader(t *testing.T, r *protocol.Reader) {
	t.Helper()
	r.Byte()
	r.String(protocol.LenByte)
	r.Byte()
	for i := 0; i < 5; i++ {
		r.Int()
	}
	r.String(protocol.LenByte)
	for {
		if r.Int() == 0 {
			return
		}
		r.Byte()
		r.String(protocol.LenByte)
		r.Byte()
		r.String(protocol.LenByte)
		for i := 0; i < 7; i++ {
			r.Int()
		}
	}
}
