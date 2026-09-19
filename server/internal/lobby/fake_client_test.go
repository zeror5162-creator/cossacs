package lobby

import (
	"bytes"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

type fakeClient struct {
	id     uint32
	addr   string
	sent   []protocol.Packet
	closed bool
}

func newFakeClient(addr string) *fakeClient { return &fakeClient{addr: addr} }

func (c *fakeClient) ID() uint32      { return c.id }
func (c *fakeClient) SetID(id uint32) { c.id = id }
func (c *fakeClient) Addr() string    { return c.addr }
func (c *fakeClient) Close()          { c.closed = true }

func (c *fakeClient) Send(b []byte) {
	p, err := protocol.ReadPacket(bytes.NewReader(b))
	if err != nil {
		panic(err)
	}
	c.sent = append(c.sent, p)
}

// cmds повертає коди отриманих пакетів — зручно для коротких перевірок.
func (c *fakeClient) cmds() []uint16 {
	out := make([]uint16, 0, len(c.sent))
	for _, p := range c.sent {
		out = append(out, p.Cmd)
	}
	return out
}
