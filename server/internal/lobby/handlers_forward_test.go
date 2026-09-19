package lobby

import "testing"

func TestRoomChatIsForwardedAsUnchangedBodyWithNewCmd(t *testing.T) {
	s := New("test")
	host, guest := newFakeClient("h"), newFakeClient("g")
	loginAll(t, s, host, guest)
	createRoom(t, s, host)
	joinRoom(t, s, guest, host.ID())

	host.sent, guest.sent = nil, nil
	body := []byte{4, '0', '|', 'h', 'i'}
	s.Handle(guest, packet(0x194, guest.ID(), 0, body))

	for _, c := range []*fakeClient{host, guest} {
		if len(c.sent) != 1 || c.sent[0].Cmd != 0x195 {
			t.Fatalf("%s got %v, want one 0x195", c.Addr(), c.cmds())
		}
		if string(c.sent[0].Body) != string(body) {
			t.Fatalf("body = % x, want % x", c.sent[0].Body, body)
		}
	}
}

func TestPrivateLobbyMessageGoesToSenderAndTarget(t *testing.T) {
	s := New("test")
	a, b, c := newFakeClient("a"), newFakeClient("b"), newFakeClient("c")
	loginAll(t, s, a, b, c)
	a.sent, b.sent, c.sent = nil, nil, nil

	s.Handle(a, packet(0x196, a.ID(), b.ID(), []byte{2, 'h', 'i'}))

	if len(a.sent) != 1 || a.sent[0].Cmd != 0x197 {
		t.Fatalf("sender got %v", a.cmds())
	}
	if len(b.sent) != 1 || b.sent[0].Cmd != 0x197 {
		t.Fatalf("target got %v", b.cmds())
	}
	if len(c.sent) != 0 {
		t.Fatalf("third party got %v, want nothing", c.cmds())
	}
}

func TestPlayerStatusIsBroadcastAs1ac(t *testing.T) {
	s := New("test")
	a, b := newFakeClient("a"), newFakeClient("b")
	loginAll(t, s, a, b)
	a.sent, b.sent = nil, nil

	s.Handle(a, packet(0x1ab, a.ID(), 0, []byte{0x0b}))

	for _, c := range []*fakeClient{a, b} {
		if len(c.sent) != 1 || c.sent[0].Cmd != 0x1ac || c.sent[0].Body[0] != 0x0b {
			t.Fatalf("%s got %v", c.Addr(), c.cmds())
		}
	}
}
