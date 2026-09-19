// Package paritytest проганяє однаковий сценарій проти двох реалізацій
// сервера й збирає отримані пакети для побайтового порівняння.
package paritytest

import (
	"net"
	"sync"
	"time"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

type client struct {
	name string
	conn net.Conn

	mu   sync.Mutex
	recv []protocol.Packet
	id   uint32 // видається сервером у відповіді 0x19b
}

func dial(addr, name string) (*client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	c := &client{name: name, conn: conn}
	go func() {
		for {
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			p, err := protocol.ReadPacket(conn)
			if err != nil {
				return
			}
			c.mu.Lock()
			c.recv = append(c.recv, p)
			if p.Cmd == 0x19b {
				c.id = p.ID1 // сервер повідомляє клієнту його id
			}
			c.mu.Unlock()
		}
	}()
	return c, nil
}

func (c *client) packets() []protocol.Packet {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]protocol.Packet, len(c.recv))
	copy(out, c.recv)
	return out
}

func (c *client) send(p protocol.Packet) {
	c.conn.Write(protocol.Encode(p))
	time.Sleep(120 * time.Millisecond) // даємо серверу розіслати наслідки
}

func (c *client) myID() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id
}

func (c *client) login(nick string) {
	w := protocol.NewWriter()
	w.String("1.0.0.7", protocol.LenByte)
	w.String("2.0.7", protocol.LenByte)
	w.String("a@b.c", protocol.LenByte)
	w.String("", protocol.LenByte)
	w.String(nick, protocol.LenByte)
	c.send(w.Packet(0x19a, 0, 0))
}

// Run виконує сценарій: три гравці заходять, створюють кімнату, грають,
// хост виходить під час гри (передача ролі), решта роз'єднується.
func Run(addr string) (map[string][]protocol.Packet, error) {
	host, err := dial(addr, "host")
	if err != nil {
		return nil, err
	}
	g1, err := dial(addr, "guest1")
	if err != nil {
		return nil, err
	}
	g2, err := dial(addr, "guest2")
	if err != nil {
		return nil, err
	}

	host.login("Host")
	g1.login("Guest1")
	g2.login("Guest2")

	hostID := host.myID()

	w := protocol.NewWriter()
	w.Int(8)
	w.Byte(0)
	w.String(`"parity"\t""\t008C7`, protocol.LenByte)
	w.String("0", protocol.LenByte)
	w.Int(42)
	w.Short(0)
	host.send(w.Packet(0x19c, hostID, 0))

	for _, g := range []*client{g1, g2} {
		jw := protocol.NewWriter()
		jw.Int(hostID)
		g.send(jw.Packet(0x19e, g.myID(), 0))
	}

	uw := protocol.NewWriter()
	uw.String(`"parity"\t""\t008C7`, protocol.LenByte)
	uw.String("1|3|0|5|0|0", protocol.LenByte)
	uw.Raw(make([]byte, 6))
	host.send(uw.Packet(0x1aa, hostID, 0))

	cw := protocol.NewWriter()
	cw.String("0|hello", protocol.LenByte)
	g1.send(cw.Packet(0x194, g1.myID(), 0))

	host.send(protocol.Packet{Cmd: 0x1a2, ID1: hostID})
	host.send(protocol.Packet{Cmd: 0x4b0, ID1: hostID, Body: []byte{1, 2, 3, 4}})
	g1.send(protocol.Packet{Cmd: 0x460, ID1: g1.myID()})
	host.send(protocol.Packet{Cmd: 0x1a0, ID1: hostID}) // хост виходить під час гри

	time.Sleep(300 * time.Millisecond)
	host.conn.Close()
	g1.conn.Close()
	time.Sleep(300 * time.Millisecond)
	g2.conn.Close()
	time.Sleep(200 * time.Millisecond)

	return map[string][]protocol.Packet{
		"host": host.packets(), "guest1": g1.packets(), "guest2": g2.packets(),
	}, nil
}
