package lobby

// DefaultProps — рядок властивостей клієнта, який оригінальний сервер
// віддає для щойно створеного гравця (Player.hpp).
const DefaultProps = "pur|0|dlc|0|ram|4|sic|0|si1|0|si2|0|si3|0|snc||sn1||sn2||sn3|"

// Статуси гравця (Player.hpp).
const (
	StatusLobby    uint8 = 0x01
	StatusInRoom   uint8 = 0x03
	StatusRoomHost uint8 = 0x05
	StatusInGame   uint8 = 0x0b
	StatusGameHost uint8 = 0x0f
)

type Player struct {
	ID     uint32
	Name   string
	Ver1   string
	Ver2   string
	Props  string
	Status uint8
	Room   *Room
}

func NewPlayer(id uint32, name, ver1, ver2 string) *Player {
	return &Player{ID: id, Name: name, Ver1: ver1, Ver2: ver2,
		Props: DefaultProps, Status: StatusLobby}
}

func (p *Player) JoinRoom(r *Room) {
	if r.HostID == p.ID {
		p.Status = StatusRoomHost
	} else {
		p.Status = StatusInRoom
	}
	p.Room = r
	r.AddPlayer(p.ID)
}

func (p *Player) LeaveRoom() {
	p.Status = StatusLobby
	if p.Room != nil {
		p.Room.RemovePlayer(p.ID)
		p.Room = nil
	}
}

type Room struct {
	HostID  uint32
	Desc    string
	Info    string
	Players []uint32
	Hidden  bool
}

func NewRoom(hostID uint32, desc string) *Room {
	return &Room{HostID: hostID, Desc: desc, Info: "0",
		Players: make([]uint32, 0, 8)}
}

func (r *Room) AddPlayer(id uint32) { r.Players = append(r.Players, id) }

func (r *Room) RemovePlayer(id uint32) {
	out := r.Players[:0]
	for _, p := range r.Players {
		if p != id {
			out = append(out, p)
		}
	}
	r.Players = out
}
