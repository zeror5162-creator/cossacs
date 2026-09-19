// Package paritytest проганяє однаковий сценарій проти двох реалізацій
// сервера й збирає отримані пакети для побайтового порівняння.
//
// Сценарій навмисно строго послідовний: кожна дія завершується очікуванням
// пакета-підтвердження, перш ніж почнеться наступна. Інакше порівняння
// побайтово не має сенсу — обидва сервери розсилають сповіщення всім
// під'єднаним сокетам, тож те, чи встиг сервер зареєструвати щойно
// прийняте з'єднання до розсилки, змінює кількість пакетів у клієнта.
package paritytest

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

// settle — пауза, за яку сервер встигає розіслати наслідки однієї дії тим,
// від кого ми не чекаємо явного підтвердження.
const settle = 150 * time.Millisecond

// ackTimeout — скільки чекаємо на пакет-підтвердження.
const ackTimeout = 5 * time.Second

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

func (c *client) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.recv)
}

func (c *client) send(p protocol.Packet) {
	c.conn.Write(protocol.Encode(p))
	time.Sleep(settle) // даємо серверу розіслати наслідки
}

// waitFor чекає, доки клієнт отримає пакет із командою cmd, починаючи з
// індексу from. Повертає помилку по таймауту, щоб сценарій падав із
// зрозумілим повідомленням, а не мовчки розходився з еталоном.
func (c *client) waitFor(cmd uint16, from int) error {
	deadline := time.Now().Add(ackTimeout)
	for {
		c.mu.Lock()
		for _, p := range c.recv[min(from, len(c.recv)):] {
			if p.Cmd == cmd {
				c.mu.Unlock()
				return nil
			}
		}
		c.mu.Unlock()
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: no %#x within %s", c.name, cmd, ackTimeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (c *client) login(nick string) error {
	from := c.count()
	w := protocol.NewWriter()
	w.String("1.0.0.7", protocol.LenByte)
	w.String("2.0.7", protocol.LenByte)
	w.String("a@b.c", protocol.LenByte)
	w.String("", protocol.LenByte)
	w.String(nick, protocol.LenByte)
	c.conn.Write(protocol.Encode(w.Packet(0x19a, 0, 0)))
	if err := c.waitFor(0x19b, from); err != nil {
		return err
	}
	time.Sleep(settle) // 0x1a6 услід за 0x19b має дійти до всіх
	return nil
}

// join під'єднує клієнта й одразу логінить його. Наступний клієнт
// з'являється лише після того, як попередній повністю зайшов у лобі.
func join(addr, name, nick string) (*client, error) {
	c, err := dial(addr, name)
	if err != nil {
		return nil, err
	}
	if err := c.login(nick); err != nil {
		c.conn.Close()
		return nil, err
	}
	return c, nil
}

// Run виконує сценарій: три гравці заходять, створюють кімнату, грають,
// хост виходить під час гри (передача ролі), решта роз'єднується.
func Run(addr string) (map[string][]protocol.Packet, error) {
	host, err := join(addr, "host", "Host")
	if err != nil {
		return nil, err
	}
	defer host.conn.Close()
	g1, err := join(addr, "guest1", "Guest1")
	if err != nil {
		return nil, err
	}
	defer g1.conn.Close()
	g2, err := join(addr, "guest2", "Guest2")
	if err != nil {
		return nil, err
	}
	defer g2.conn.Close()

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

	// Роз'єднуємо по одному: якщо закрити два сокети разом, порядок, у якому
	// сервер помітить розриви, стає випадковим, і сповіщення 0x1a7 приходять
	// у різному порядку на різних прогонах.
	host.conn.Close()
	time.Sleep(settle)
	g1.conn.Close()
	time.Sleep(settle)
	g2.conn.Close()
	time.Sleep(settle)

	return map[string][]protocol.Packet{
		"host": host.packets(), "guest1": g1.packets(), "guest2": g2.packets(),
	}, nil
}

func (c *client) myID() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.id
}
