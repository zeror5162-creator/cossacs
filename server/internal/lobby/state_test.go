package lobby

import "testing"

func TestNewPlayerDefaults(t *testing.T) {
	p := NewPlayer(7, "Taras", "1.0.0.7", "2.0.7")
	if p.Status != 0x01 {
		t.Fatalf("Status = %#x, want 0x01", p.Status)
	}
	if p.Props != DefaultProps {
		t.Fatalf("Props = %q", p.Props)
	}
}

func TestJoinRoomHostGetsStatus5(t *testing.T) {
	host := NewPlayer(3, "Host", "1.0.0.7", "2.0.7")
	room := NewRoom(3, `"game"\t""\t008C7`)
	host.JoinRoom(room)
	if host.Status != 0x05 {
		t.Fatalf("host status = %#x, want 0x05", host.Status)
	}
	if len(room.Players) != 1 || room.Players[0] != 3 {
		t.Fatalf("room players = %v", room.Players)
	}
	if room.Info != "0" {
		t.Fatalf("Info = %q, want \"0\"", room.Info)
	}
}

func TestJoinRoomGuestGetsStatus3AndKeepsOrder(t *testing.T) {
	room := NewRoom(3, "d")
	host := NewPlayer(3, "Host", "", "")
	guest := NewPlayer(4, "Guest", "", "")
	host.JoinRoom(room)
	guest.JoinRoom(room)
	if guest.Status != 0x03 {
		t.Fatalf("guest status = %#x, want 0x03", guest.Status)
	}
	if got := room.Players; len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("players = %v, want [3 4]", got)
	}
}

func TestLeaveRoomResetsStatusAndRemovesID(t *testing.T) {
	room := NewRoom(3, "d")
	host := NewPlayer(3, "Host", "", "")
	guest := NewPlayer(4, "Guest", "", "")
	host.JoinRoom(room)
	guest.JoinRoom(room)
	guest.LeaveRoom()
	if guest.Status != 0x01 || guest.Room != nil {
		t.Fatalf("guest = %+v", guest)
	}
	if got := room.Players; len(got) != 1 || got[0] != 3 {
		t.Fatalf("players = %v, want [3]", got)
	}
}
