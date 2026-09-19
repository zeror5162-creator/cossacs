package lobby

import (
	"testing"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

func TestNormalizeNick(t *testing.T) {
	cases := map[string]string{
		"ab":    "ab__",
		"Taras": "Taras",
		// кирилиця: 5 символів = 10 байтів, кожен байт → '_'
		"верхи":             "__________",
		"0123456789abcdefg": "0123456789abcdef",
		"a b":               "a_b_",
	}
	for in, want := range cases {
		if got := NormalizeNick(in); got != want {
			t.Errorf("NormalizeNick(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoginAnswersWith19bAndBroadcasts1a6(t *testing.T) {
	s := New("test")
	first, second := newFakeClient("1"), newFakeClient("2")
	login(t, s, first, "Alpha")
	first.sent = nil
	login(t, s, second, "Beta")

	if len(second.sent) < 2 || second.sent[0].Cmd != 0x19b {
		t.Fatalf("second got %v, want 0x19b first", second.cmds())
	}
	if second.sent[0].ID1 != second.ID() || second.sent[0].ID2 != second.ID() {
		t.Fatalf("0x19b ids = %d/%d", second.sent[0].ID1, second.sent[0].ID2)
	}
	if len(first.sent) != 1 || first.sent[0].Cmd != 0x1a6 {
		t.Fatalf("first got %v, want one 0x1a6", first.cmds())
	}

	r := protocol.NewReader(second.sent[0].Body)
	r.Byte()
	if nick := r.String(protocol.LenByte); nick != "Beta" {
		t.Fatalf("nick in 0x19b = %q", nick)
	}
}

func TestLobbyListInResponseIsSortedByIDAndIncludesSelf(t *testing.T) {
	s := New("test")
	a, b, c := newFakeClient("a"), newFakeClient("b"), newFakeClient("c")
	login(t, s, a, "Alpha")
	login(t, s, b, "Beta")
	login(t, s, c, "Gamma")

	r := protocol.NewReader(c.sent[0].Body)
	r.Byte()
	r.String(protocol.LenByte) // власний нік
	r.Byte()
	for i := 0; i < 5; i++ {
		r.Int()
	}
	r.String(protocol.LenByte) // власні props

	var ids []uint32
	for {
		id := r.Int()
		if id == 0 {
			break
		}
		r.Byte()
		r.String(protocol.LenByte)
		r.Byte()
		r.String(protocol.LenByte)
		for i := 0; i < 7; i++ {
			r.Int()
		}
		ids = append(ids, id)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Fatalf("player ids = %v, want [1 2 3]", ids)
	}
	if r.Err() != nil {
		t.Fatalf("reader error: %v", r.Err())
	}
}

func TestCommandBeforeLoginIsIgnored(t *testing.T) {
	s := New("test")
	c := newFakeClient("x")
	s.Connect(c)

	s.Handle(c, packet(0x1ad, 0, 0, []byte{5, '2', '.', '0', '.', '7'}))
	createRoom(t, s, c)

	if len(c.sent) != 0 {
		t.Fatalf("got %v, want nothing before login", c.cmds())
	}
	if got := s.Snapshot().Rooms; got != 0 {
		t.Fatalf("rooms = %d, want 0", got)
	}
}

func TestVersionCheckAnswers1ae(t *testing.T) {
	s := New("test")
	c := newFakeClient("x")
	login(t, s, c, "Alpha")
	c.sent = nil

	s.Handle(c, packet(0x1ad, 0, 0, []byte{5, '2', '.', '0', '.', '7'}))

	if len(c.sent) != 1 || c.sent[0].Cmd != 0x1ae || c.sent[0].ID2 != c.ID() {
		t.Fatalf("got %v", c.sent)
	}
	r := protocol.NewReader(c.sent[0].Body)
	if v1 := r.String(protocol.LenByte); v1 != "1.0.0.7" {
		t.Fatalf("ver1 = %q", v1)
	}
	if v2 := r.String(protocol.LenByte); v2 != "2.0.7" {
		t.Fatalf("ver2 = %q", v2)
	}
}
