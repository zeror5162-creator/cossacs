package lobby

import "testing"

func TestDisconnectNotifiesEveryoneWith1a7(t *testing.T) {
	s := New("test")
	a, b := newFakeClient("a"), newFakeClient("b")
	loginAll(t, s, a, b)
	a.sent, b.sent = nil, nil

	s.Disconnect(a)

	if len(b.sent) != 1 || b.sent[0].Cmd != 0x1a7 || b.sent[0].ID1 != a.ID() {
		t.Fatalf("remaining client got %v", b.sent)
	}
	if len(a.sent) != 0 {
		t.Fatalf("leaving client got %v, want nothing", a.cmds())
	}
	if got := s.Snapshot().Players; got != 1 {
		t.Fatalf("players = %d, want 1", got)
	}
}

func TestDisconnectOfRoomHostCleansRoomUp(t *testing.T) {
	s := New("test")
	host, guest := newFakeClient("h"), newFakeClient("g")
	loginAll(t, s, host, guest)
	createRoom(t, s, host)
	joinRoom(t, s, guest, host.ID())
	host.sent, guest.sent = nil, nil

	s.Disconnect(host)

	if !hasCmd(guest.cmds(), 0x1a1) {
		t.Fatalf("guest got %v, want 0x1a1 before 0x1a7", guest.cmds())
	}
	if !hasCmd(guest.cmds(), 0x1a7) {
		t.Fatalf("guest got %v, want 0x1a7", guest.cmds())
	}
	if got := s.Snapshot().Rooms; got != 0 {
		t.Fatalf("rooms = %d, want 0", got)
	}
	if pl := s.playerForTest(guest.ID()); pl.Room != nil {
		t.Fatal("guest is still linked to a deleted room")
	}
}

func TestDisconnectBeforeLoginIsSilent(t *testing.T) {
	s := New("test")
	a, b := newFakeClient("a"), newFakeClient("b")
	loginAll(t, s, a)
	s.Connect(b)
	a.sent = nil

	s.Disconnect(b)

	if len(a.sent) != 0 {
		t.Fatalf("got %v, want nothing", a.cmds())
	}
}
