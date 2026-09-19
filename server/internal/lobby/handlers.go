// Обробники команд лобі-протоколу.
//
// Логіка й формати перенесені з cossacks3-lan-server/src/Lobby.cpp,
// (c) 2018 Ereb @ habrahabr.ru, ліцензія MIT.
package lobby

import (
	"log/slog"
	"strconv"
	"strings"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

// maxTextPacket — стеля для чат-пакетів (0x194/0x196).
const maxTextPacket = 1024

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
	case 0x194, 0x196:
		// Рядок у цих пакетах має 1-байтний префікс довжини, тож усе,
		// більше за maxTextPacket, справжній клієнт надіслати не може.
		// Без обмеження один гравець розсилає всім по мегабайту.
		if len(p.Body) > maxTextPacket {
			slog.Warn("oversized text packet dropped", "server", s.Name,
				"cmd", p.Cmd, "id", id, "size", len(p.Body))
			return
		}
		if p.Cmd == 0x194 {
			s.forward(p, id, 0x195, toEveryoneInRoom)
			return
		}
		switch {
		case p.ID2 == 0:
			s.forward(p, id, 0x197, toEveryone)
		case p.ID1 == p.ID2:
			s.forward(p, id, 0x197, toSource)
		default:
			s.forward(p, id, 0x197, toSource)
			s.forward(p, id, 0x197, toID2)
		}

	// кімнати
	case 0x19c:
		r := protocol.NewReader(p.Body)
		r.Skip(5)
		desc := r.String(protocol.LenByte)
		info := r.String(protocol.LenByte)
		magic := r.Int()
		if r.Err() != nil {
			return
		}
		pl := s.players[id]
		// Після 0x1b5 (кік) зв'язок із кімнатою лишається — і в оригіналі теж.
		// Знімаємо його тут, інакше гравець назавжди втратить змогу грати.
		pl.LeaveRoom()
		if _, taken := s.rooms[id]; taken {
			return // кімната з таким хостом уже є
		}
		room := NewRoom(id, desc)
		s.rooms[id] = room
		pl.JoinRoom(room)

		w := protocol.NewWriter()
		w.Byte(7)
		w.Int(8)
		w.String(desc, protocol.LenByte)
		w.String(info, protocol.LenByte)
		w.Int(magic)
		w.Short(0)
		s.send(w.Packet(0x19d, p.ID1, 0), id, toEveryone)

	case 0x19e:
		r := protocol.NewReader(p.Body)
		hostID := r.Int()
		if r.Err() != nil {
			return
		}
		room, ok := s.rooms[hostID]
		if !ok {
			return
		}
		pl := s.players[id]
		if pl.Room == room {
			return // вже в цій кімнаті
		}
		pl.LeaveRoom() // знімаємо залишковий зв'язок після кіку
		pl.JoinRoom(room)

		w := protocol.NewWriter()
		w.Int(hostID)
		w.Byte(pl.Status)
		s.send(w.Packet(0x19f, p.ID1, 0), id, toEveryone)

	case 0x1a0:
		s.leaveRoom(c, p)

	case 0x1a2:
		pl := s.players[id]
		if pl.Room == nil {
			return
		}
		room := pl.Room
		room.Hidden = true
		w := protocol.NewWriter()
		w.Int(uint32(len(room.Players)))
		for i := len(room.Players) - 1; i >= 0; i-- {
			pid := room.Players[i]
			other := s.players[pid]
			if pid == id {
				other.Status = StatusGameHost
			} else {
				other.Status = StatusInGame
			}
			w.Int(pid)
			w.Byte(other.Status)
		}
		s.send(w.Packet(0x1a3, p.ID1, 0), id, toEveryone)

	case 0x1aa:
		r := protocol.NewReader(p.Body)
		desc := r.String(protocol.LenByte)
		info := r.String(protocol.LenByte)
		if r.Err() != nil {
			return
		}
		pl := s.players[id]
		if pl.Room == nil {
			return
		}
		room := pl.Room
		room.Info = info

		w := protocol.NewWriter()
		w.Int(8)
		w.String(desc, protocol.LenByte)
		w.String(info, protocol.LenByte)
		w.Int(0)
		w.Short(0)
		w.Int(uint32(len(room.Players)))
		for i := len(room.Players) - 1; i >= 0; i-- {
			pid := room.Players[i]
			w.Int(pid)
			w.Byte(s.players[pid].Status)
		}
		s.send(w.Packet(0x1a5, p.ID1, 0), id, toEveryone)

	case 0x1b5:
		r := protocol.NewReader(p.Body)
		kickID := r.Int()
		if r.Err() != nil {
			return
		}
		s.forward(p, id, 0x1b6, toEveryone)

		w := protocol.NewWriter()
		w.Byte(0)
		w.Int(1)
		w.Int(kickID)
		w.Byte(1)
		s.send(w.Packet(0x1a1, kickID, 0), id, toEveryone)

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

// leaveRoom обробляє 0x1a0 і використовується при розриві з'єднання.
func (s *Server) leaveRoom(c Client, p protocol.Packet) {
	id := c.ID()
	pl, ok := s.players[id]
	if !ok || pl.Room == nil {
		// вихід хоста провокує 0x1a0 від інших гравців — їх ігноруємо
		return
	}
	room := pl.Room
	roomID := room.HostID
	players := append([]uint32(nil), room.Players...)
	status := pl.Status

	hostLeaving := status == StatusRoomHost || status == StatusGameHost
	transferNeeded := status == StatusGameHost && len(players) > 1
	newHostID := players[len(players)-1]

	w := protocol.NewWriter()
	if hostLeaving {
		w.Byte(1)
		w.Int(uint32(len(players)))
		for _, pid := range players {
			other := s.players[pid]
			other.LeaveRoom()
			w.Int(pid)
			w.Byte(other.Status)
		}
	} else {
		pl.LeaveRoom()
		w.Byte(0)
		w.Int(1)
		w.Int(pl.ID)
		w.Byte(pl.Status)
	}
	s.send(w.Packet(0x1a1, p.ID1, 0), id, toEveryone)

	if transferNeeded {
		s.sendHostTransfer(id, room, players, newHostID)
	}
	if hostLeaving {
		delete(s.rooms, roomID)
	}
}

// sendHostTransfer шле 0x1bd новому хосту й 0x1be решті гравців.
func (s *Server) sendHostTransfer(srcID uint32, room *Room, players []uint32, newHostID uint32) {
	w := protocol.NewWriter()
	w.Int(0) // місце під довжину блоку, перезапишемо в кінці
	w.Int(0)
	w.Int(1)
	w.Byte(0)
	w.Int(6)

	pair := func(k, v string) {
		w.String(k, protocol.LenInt)
		w.String(v, protocol.LenInt)
		w.Int(0)
	}
	pair("gamename", room.Desc)
	pair("mapname", room.Info)
	pair("master", strconv.FormatUint(uint64(newHostID), 10))
	pair("session", "1337")
	pair("clients", strconv.Itoa(len(players)-1))

	w.String("clientslist", protocol.LenInt)
	w.Int(1)
	w.Byte(0)
	w.Int(uint32(len(players) - 1))
	for _, pid := range players[1:] {
		w.String("*", protocol.LenInt)
		w.String(strconv.FormatUint(uint64(pid), 10), protocol.LenInt)
	}
	w.Int(0)

	// перший int тіла = довжина решти тіла
	w.PatchInt(0, uint32(w.Len()-4))
	s.send(w.Packet(0x1bd, newHostID, newHostID), srcID, toID2)

	for _, pid := range players[1:] {
		if pid == newHostID {
			continue
		}
		s.send(protocol.Packet{Cmd: 0x1be, ID1: newHostID, ID2: pid}, srcID, toID2)
	}
}
