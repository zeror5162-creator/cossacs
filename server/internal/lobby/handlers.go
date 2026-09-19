// Обробники команд лобі-протоколу.
//
// Логіка й формати перенесені з cossacks3-lan-server/src/Lobby.cpp,
// (c) 2018 Ereb @ habrahabr.ru, ліцензія MIT.
package lobby

import (
	"log/slog"
	"strings"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

const allowedNickChars = "abcdefghijklmnopqrstuvwxyz" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789()+-_.[]"

// NormalizeNick повторює обмеження оригінального клієнта:
// довжина 4..16, дозволені лише allowedNickChars, решта → '_'.
func NormalizeNick(name string) string {
	b := []byte(name)
	for len(b) < 4 {
		b = append(b, '_')
	}
	if len(b) > 16 {
		b = b[:16]
	}
	for i, ch := range b {
		if !strings.ContainsRune(allowedNickChars, rune(ch)) {
			b[i] = '_'
		}
	}
	return string(b)
}

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

	// логін і профіль
	case 0x19a:
		s.handleLogin(c, p)
	case 0x1a8:
		out := p
		out.Body = append(append([]byte{}, p.Body...), 1)
		out.Cmd, out.ID1, out.ID2 = 0x1a9, 0, 0
		s.send(out, id, toSource)
	case 0x1ad:
		pl, ok := s.players[id]
		if !ok {
			return
		}
		w := protocol.NewWriter()
		w.String(pl.Ver1, protocol.LenByte)
		w.String(pl.Ver2, protocol.LenByte)
		w.Int(0)
		s.send(w.Packet(0x1ae, 0, pl.ID), id, toSource)
	case 0x1b3:
		pl, ok := s.players[id]
		if !ok {
			return
		}
		r := protocol.NewReader(p.Body)
		r.String(protocol.LenByte)
		r.String(protocol.LenByte)
		r.String(protocol.LenByte)
		props := r.String(protocol.LenByte)
		if r.Err() == nil {
			pl.Props = props
		}
	case 0x192:
		r := protocol.NewReader(p.Body)
		infoID := r.Int()
		if r.Err() != nil {
			return
		}
		target, ok := s.players[infoID]
		if !ok {
			return
		}
		w := protocol.NewWriter()
		w.Int(infoID)
		w.Byte(target.Status)
		w.String(target.Name, protocol.LenByte)
		w.Byte(0)
		for i := 0; i < 5; i++ {
			w.Int(0)
		}
		w.String(target.Props, protocol.LenByte)
		s.send(w.Packet(0x193, infoID, p.ID1), id, toSource)

	// без відповіді (як в оригіналі)
	case 0x198, 0x1b7:

	default:
		slog.Debug("unknown packet", "server", s.Name, "cmd", p.Cmd, "id", id)
	}
}

func (s *Server) handleLogin(c Client, p protocol.Packet) {
	id := c.ID()
	if _, exists := s.players[id]; exists {
		return // повторний логін в одному з'єднанні ігнорується
	}

	r := protocol.NewReader(p.Body)
	ver1 := r.String(protocol.LenByte)
	ver2 := r.String(protocol.LenByte)
	r.Skip(int(r.Byte())) // email
	r.Skip(int(r.Byte())) // пароль
	name := r.String(protocol.LenByte)
	if r.Err() != nil {
		slog.Warn("malformed login", "server", s.Name, "addr", c.Addr())
		c.Close()
		return
	}

	pl := NewPlayer(id, NormalizeNick(name), ver1, ver2)
	s.players[id] = pl

	// 0x19b: стан лобі для того, хто щойно зайшов
	w := protocol.NewWriter()
	w.Byte(0)
	w.String(pl.Name, protocol.LenByte)
	w.Byte(0)
	for i := 0; i < 5; i++ {
		w.Int(0)
	}
	w.String(pl.Props, protocol.LenByte)
	for _, pid := range s.sortedPlayerIDs() {
		other := s.players[pid]
		w.Int(pid)
		w.Byte(other.Status)
		w.String(other.Name, protocol.LenByte)
		w.Byte(0)
		w.String(other.Props, protocol.LenByte)
		for i := 0; i < 7; i++ {
			w.Int(0)
		}
	}
	w.Int(0)
	roomIDs := s.sortedRoomIDs()
	for i := len(roomIDs) - 1; i >= 0; i-- {
		room := s.rooms[roomIDs[i]]
		if room.Hidden {
			continue
		}
		w.Int(room.HostID)
		w.Int(8)
		w.String(room.Desc, protocol.LenByte)
		w.String(room.Info, protocol.LenByte)
		w.Int(0)
		w.Short(0)
		w.Int(uint32(len(room.Players)))
		for j := len(room.Players) - 1; j >= 0; j-- {
			w.Int(room.Players[j])
		}
	}
	w.Int(0)
	s.send(w.Packet(0x19b, id, id), id, toSource)

	// 0x1a6: сповіщення всім, включно з новим гравцем
	n := protocol.NewWriter()
	n.String(pl.Name, protocol.LenByte)
	n.Byte(0)
	n.String(pl.Props, protocol.LenByte)
	n.Byte(pl.Status)
	s.send(n.Packet(0x1a6, id, 0), id, toEveryone)

	slog.Info("login", "server", s.Name, "id", id, "nick", pl.Name, "addr", c.Addr())
}
