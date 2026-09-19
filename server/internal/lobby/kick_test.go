package lobby

import "testing"

// Гравець, якого викинули з гри, має змогти створити власну кімнату:
// оригінал не знімає з нього зв'язок із кімнатою, тож без підчистки
// наступний 0x19c мовчки нічого не робить.
func TestKickedPlayerCanCreateRoomAfterwards(t *testing.T) {
	s := New("test")
	host, victim := newFakeClient("h"), newFakeClient("v")
	loginAll(t, s, host, victim)
	createRoom(t, s, host)
	joinRoom(t, s, victim, host.ID())

	w := packet(0x1b5, host.ID(), 0, []byte{byte(victim.ID()), 0, 0, 0})
	s.Handle(host, w)

	host.sent, victim.sent = nil, nil
	createRoom(t, s, victim)

	if len(victim.sent) == 0 || victim.sent[0].Cmd != 0x19d {
		t.Fatalf("kicked player got %v, want 0x19d", victim.cmds())
	}
	if got := s.Snapshot().Rooms; got != 2 {
		t.Fatalf("rooms = %d, want 2", got)
	}
	if pl := s.playerForTest(victim.ID()); pl.Room == nil || pl.Room.HostID != victim.ID() {
		t.Fatalf("kicked player is not the host of the new room: %+v", pl.Room)
	}
	// у старій кімнаті його вже нема
	if old := s.playerForTest(host.ID()).Room; len(old.Players) != 1 {
		t.Fatalf("old room players = %v, want just the host", old.Players)
	}
}

// Те саме для входу в чужу кімнату після кіку.
func TestKickedPlayerCanJoinAnotherRoom(t *testing.T) {
	s := New("test")
	host, victim, other := newFakeClient("h"), newFakeClient("v"), newFakeClient("o")
	loginAll(t, s, host, victim, other)
	createRoom(t, s, host)
	joinRoom(t, s, victim, host.ID())
	s.Handle(host, packet(0x1b5, host.ID(), 0, []byte{byte(victim.ID()), 0, 0, 0}))
	createRoom(t, s, other)

	victim.sent = nil
	joinRoom(t, s, victim, other.ID())

	if len(victim.sent) == 0 || victim.sent[0].Cmd != 0x19f {
		t.Fatalf("kicked player got %v, want 0x19f", victim.cmds())
	}
	if pl := s.playerForTest(victim.ID()); pl.Room == nil || pl.Room.HostID != other.ID() {
		t.Fatalf("kicked player did not join the other room")
	}
}
