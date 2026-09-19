// Обробники команд лобі-протоколу.
//
// Логіка й формати перенесені з cossacks3-lan-server/src/Lobby.cpp,
// (c) 2018 Ereb @ habrahabr.ru, ліцензія MIT.
package lobby

import (
	"log/slog"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

// Handle обробляє один вхідний пакет від клієнта.
func (s *Server) Handle(c Client, p protocol.Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handle(c, p)
}

// forward пересилає вхідний пакет без змін тіла, можливо з іншим cmd.
func (s *Server) forward(p protocol.Packet, srcID uint32, cmd uint16, target sendTo) {
	out := p
	out.Cmd = cmd
	s.send(out, srcID, target)
}

func (s *Server) handle(c Client, p protocol.Packet) {
	id := c.ID()

	if _, ok := s.players[id]; !ok {
		switch p.Cmd {
		case 0x19a, 0x1a8, 0x198:
		default:
			slog.Debug("packet before login", "server", s.Name, "cmd", p.Cmd, "id", id)
			return
		}
	}

	switch p.Cmd {
	// ігрові дані й завантаження
	case 0x4b0, 0x456:
		s.forward(p, id, p.Cmd, toPropagateInRoom)
	case 0x032, 0x457, 0x461:
		s.forward(p, id, p.Cmd, toEveryoneInRoomButSource)
	case 0x460, 0x064, 0x065:
		s.forward(p, id, p.Cmd, toRoomHost)
	case 0x066:
		s.forward(p, id, p.Cmd, toSource)

	// статуси й властивості
	case 0x1ab:
		s.forward(p, id, 0x1ac, toEveryone)
	case 0x0c8:
		s.forward(p, id, p.Cmd, toEveryoneButSource)
	case 0x0c9:
		s.forward(p, id, p.Cmd, toID2)
	case 0x1af:
		s.forward(p, id, p.Cmd, toEveryone)
	case 0x1bb:
		s.forward(p, id, 0x1bc, toEveryoneInRoom)

	// чат
	case 0x194:
		s.forward(p, id, 0x195, toEveryoneInRoom)
	case 0x196:
		switch {
		case p.ID2 == 0:
			s.forward(p, id, 0x197, toEveryone)
		case p.ID1 == p.ID2:
			s.forward(p, id, 0x197, toSource)
		default:
			s.forward(p, id, 0x197, toSource)
			s.forward(p, id, 0x197, toID2)
		}

	// без відповіді (як в оригіналі)
	case 0x198, 0x1b7:

	default:
		slog.Debug("unknown packet", "server", s.Name, "cmd", p.Cmd, "id", id)
	}
}
