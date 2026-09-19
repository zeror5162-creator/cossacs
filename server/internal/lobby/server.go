package lobby

import (
	"log/slog"
	"sort"
	"sync"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

// Client — з'єднання з гравцем з точки зору лобі.
type Client interface {
	ID() uint32
	SetID(uint32)
	Addr() string
	Send([]byte)
	Close()
}

type sendTo int

const (
	toSource sendTo = iota
	toID2
	toEveryone
	toEveryoneButSource
	toRoomHost
	toEveryoneInRoom
	toEveryoneInRoomButSource
	toPropagateInRoom
)

type Server struct {
	Name string

	mu      sync.Mutex
	clients map[uint32]Client
	players map[uint32]*Player
	rooms   map[uint32]*Room
	lastID  uint32
}

func New(name string) *Server {
	return &Server{
		Name:    name,
		clients: make(map[uint32]Client),
		players: make(map[uint32]*Player),
		rooms:   make(map[uint32]*Room),
	}
}

func (s *Server) Connect(c Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastID++
	c.SetID(s.lastID)
	s.clients[s.lastID] = c
	slog.Info("client connected", "server", s.Name, "id", s.lastID, "addr", c.Addr())
}

// Disconnect прибирає клієнта з лобі: спершу з реєстру (щоб не слати
// пакети мертвому сокету), потім з кімнати, потім сповіщає решту.
func (s *Server) Disconnect(c Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := c.ID()
	delete(s.clients, id)
	slog.Info("client disconnected", "server", s.Name, "id", id, "addr", c.Addr())

	pl, ok := s.players[id]
	if !ok {
		return // розрив до логіну
	}
	if pl.Room != nil {
		s.leaveRoom(c, protocol.Packet{Cmd: 0x1a0, ID1: id})
	}
	delete(s.players, id)

	s.send(protocol.Packet{Cmd: 0x1a7, ID1: id}, id, toEveryone)
}

// sortedClientIDs повертає id клієнтів за зростанням — як std::map в оригіналі.
func (s *Server) sortedClientIDs() []uint32 {
	ids := make([]uint32, 0, len(s.clients))
	for id := range s.clients {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (s *Server) sortedPlayerIDs() []uint32 {
	ids := make([]uint32, 0, len(s.players))
	for id := range s.players {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (s *Server) sortedRoomIDs() []uint32 {
	ids := make([]uint32, 0, len(s.rooms))
	for id := range s.rooms {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// send кладе вже закодований пакет у черги потрібних клієнтів.
// Викликається під уже взятим s.mu.
func (s *Server) send(p protocol.Packet, srcID uint32, target sendTo) {
	raw := protocol.Encode(p)

	deliver := func(id uint32) {
		if c, ok := s.clients[id]; ok {
			c.Send(raw)
		}
	}

	switch target {
	case toSource:
		deliver(srcID)
	case toID2:
		deliver(p.ID2)
	case toEveryone:
		for _, id := range s.sortedClientIDs() {
			deliver(id)
		}
	case toEveryoneButSource:
		for _, id := range s.sortedClientIDs() {
			if id != srcID {
				deliver(id)
			}
		}
	default:
		pl, ok := s.players[srcID]
		if !ok || pl.Room == nil {
			return
		}
		room := pl.Room
		switch target {
		case toRoomHost:
			deliver(room.HostID)
		case toEveryoneInRoom:
			for _, id := range room.Players {
				deliver(id)
			}
		case toEveryoneInRoomButSource:
			for _, id := range room.Players {
				if id != srcID {
					deliver(id)
				}
			}
		case toPropagateInRoom:
			if srcID == room.HostID {
				for _, id := range room.Players {
					if id != srcID {
						deliver(id)
					}
				}
			} else {
				deliver(room.HostID)
			}
		}
	}
}

// Snapshot — миттєвий стан сервера для моніторингу й тестів.
type Snapshot struct {
	Name    string
	Clients int
	Players int
	Rooms   int
}

func (s *Server) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{Name: s.Name, Clients: len(s.clients),
		Players: len(s.players), Rooms: len(s.rooms)}
}

// playerForTest повертає гравця за id. Використовується лише тестами.
func (s *Server) playerForTest(id uint32) *Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.players[id]
}

// HasPlayer каже, чи завершив клієнт логін.
func (s *Server) HasPlayer(id uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.players[id]
	return ok
}
