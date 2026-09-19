# Етап 1: лобі-сервер на Go — план реалізації

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Повна заміна C++ сервера `cossacks3-lan-server` на Go-сервіс `c3hub`, який обслуговує кілька ігрових портів одним процесом і поводиться з клієнтом Cossacks 3 байт у байт як оригінал.

**Architecture:** Три шари. `protocol` кодує й декодує пакети (14-байтний заголовок LE + тіло), без мережі. `lobby` тримає стан одного сервера (клієнти, гравці, кімнати) і обробляє команди — перенесення `Lobby.cpp` зі збереженням порядку й вмісту байтів. `netsrv` приймає TCP-з'єднання й перетворює їх на `lobby.Client`. `cmd/c3hub` читає TOML-конфіг і піднімає по одному `lobby.Server` на порт. Захист від регресій — тести паритету: один сценарій проганяється проти C++ сервера й проти Go-сервера, байти порівнюються.

**Tech Stack:** Go 1.23+, стандартна бібліотека, `github.com/BurntSushi/toml`. Без cgo. Крос-компіляція `GOOS=linux GOARCH=arm64`. C++ оригінал для тестів паритету збирається в GitHub Actions (`g++ -DASIO_STANDALONE`, `libasio-dev`).

**Spec:** [docs/superpowers/specs/2026-09-20-cossacks3-multiplayer-design.md](../specs/2026-09-20-cossacks3-multiplayer-design.md)

**Reference:** оригінальні C++ вихідники лежать локально в `reference/cpp-src/` (у git не входять, копія з `oracle:~/cossacks3-lan-server/src`). Ключовий файл — `Lobby.cpp`.

## Global Constraints

- Go 1.23+, `CGO_ENABLED=0`, збірка для `linux/arm64`.
- Жодних змін у клієнті гри. Усі відповіді, крім логіну, — **байт у байт як в оригіналі**.
- Усі цілі числа в протоколі — **little-endian**. Рядки — з префіксом довжини 1, 2 або 4 байти.
- `HeaderSize = 14`, `MaxBodySize = 0x100000` (1 МіБ). Пакет із більшим тілом → з'єднання закривається.
- Ліцензія MIT з обов'язковою згадкою автора оригіналу (Ereb, 2018) у `LICENSE` і в заголовку `internal/lobby/handlers.go`.
- Секрети (токени, паролі, БД) не потрапляють у репозиторій: він публічний.
- Етап 1 **не** містить акаунтів, банів, SQLite, HTTP і Telegram — це етап 2.
- Жодних навантажувальних тестів на VPS: хост ділиться з Minecraft-сервером.

## Review Focus

Перевірки, яких вимагає специфікація, але які не випливають із жодного «щасливого» сценарію. Кожна прив'язана до задачі, що володіє кодом:

1. **Обрізаний або брехливий пакет** — тіло коротше, ніж каже заголовок, або рядок, довжина якого виходить за межі тіла. Очікувано: з'єднання закривається, сервер працює далі. (Задачі 1, 9)
2. **Команда для гравця, який не залогінений** — клієнт шле `0x19c` або `0x1ad` до `0x19a`. Очікувано: пакет ігнорується, паніки нема. (Задача 6)
3. **Посилання на неіснуючий id** — вхід у кімнату, якої вже нема, запит інфо про гравця, що вийшов. Очікувано: пакет ігнорується, інші гравці не страждають. (Задача 7)
4. **Вихід хоста під час гри** — роль хоста має перейти до наступного гравця, кімната видаляється, решта отримує `0x1be`. (Задача 7)
5. **Обрив з'єднання без `0x1a0`** — гравець, який був у кімнаті, просто зник. Очікувано: кімната прибирається так само, як при штатному виході, і всі отримують `0x1a7`. (Задача 8)

---

## Файлова структура

| Файл | Відповідальність |
|---|---|
| `server/go.mod` | модуль `github.com/zeror5162-creator/cossacs/server` |
| `server/internal/protocol/packet.go` | `Packet`, `Reader`, `Writer`, `Encode` |
| `server/internal/protocol/frame.go` | читання кадру з `io.Reader`, перевірки розміру |
| `server/internal/protocol/packet_test.go` | тести кодування |
| `server/internal/lobby/state.go` | `Player`, `Room` і операції над ними |
| `server/internal/lobby/server.go` | `Server`, `Client`, реєстр клієнтів, `send()` |
| `server/internal/lobby/handlers.go` | обробники команд (порт `Lobby.cpp`) |
| `server/internal/lobby/*_test.go` | юніт-тести стану, маршрутизації, обробників |
| `server/internal/netsrv/listener.go` | TCP-слухач, сесія, черга надсилань |
| `server/internal/config/config.go` | TOML-конфіг |
| `server/cmd/c3hub/main.go` | запуск, логування, коректне завершення |
| `server/internal/paritytest/` | сценарії й раннер тестів паритету |
| `deploy/c3hub.service`, `deploy/config.example.toml`, `deploy/deploy.sh` | розгортання |
| `.github/workflows/ci.yml` | `go test` + паритет проти C++ |

---

### Task 1: Кодування пакетів

**Files:**
- Create: `server/go.mod`, `server/internal/protocol/packet.go`
- Test: `server/internal/protocol/packet_test.go`

**Interfaces:**
- Consumes: —
- Produces: `protocol.Packet{Cmd uint16; ID1, ID2 uint32; Body []byte}`; `protocol.NewReader(body []byte) *Reader` з методами `Byte() uint8`, `Short() uint16`, `Int() uint32`, `String(LengthType) string`, `Skip(int)`, `Err() error`; `protocol.NewWriter() *Writer` з методами `Byte(uint8)`, `Short(uint16)`, `Int(uint32)`, `String(string, LengthType)`, `Raw([]byte)`, `Packet(cmd uint16, id1, id2 uint32) Packet`; `protocol.Encode(Packet) []byte`; константи `HeaderSize = 14`, `MaxBodySize = 0x100000`, `LenByte`, `LenShort`, `LenInt`.

- [ ] **Step 1: Створити модуль**

```bash
mkdir -p server/internal/protocol && cd server && go mod init github.com/zeror5162-creator/cossacs/server
```

- [ ] **Step 2: Написати тест, що падає**

`server/internal/protocol/packet_test.go`:

```go
package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeHeaderIsLittleEndian(t *testing.T) {
	got := Encode(Packet{Cmd: 0x19b, ID1: 2, ID2: 2, Body: []byte{0xAA}})
	want := []byte{
		0x01, 0x00, 0x00, 0x00, // size = 1
		0x9b, 0x01, // cmd
		0x02, 0x00, 0x00, 0x00, // id1
		0x02, 0x00, 0x00, 0x00, // id2
		0xAA,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode() = % x, want % x", got, want)
	}
}

func TestWriterStringLengthPrefixes(t *testing.T) {
	w := NewWriter()
	w.String("ab", LenByte)
	w.String("cd", LenShort)
	w.String("ef", LenInt)
	got := w.Packet(0x1a6, 1, 0).Body
	want := []byte{
		0x02, 'a', 'b',
		0x02, 0x00, 'c', 'd',
		0x02, 0x00, 0x00, 0x00, 'e', 'f',
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("body = % x, want % x", got, want)
	}
}

func TestReaderRoundTrip(t *testing.T) {
	w := NewWriter()
	w.Int(8)
	w.String(`"room"\t""\t008C7`, LenByte)
	w.Byte(7)
	r := NewReader(w.Packet(0x19c, 1, 0).Body)
	if got := r.Int(); got != 8 {
		t.Fatalf("Int() = %d, want 8", got)
	}
	if got := r.String(LenByte); got != `"room"\t""\t008C7` {
		t.Fatalf("String() = %q", got)
	}
	if got := r.Byte(); got != 7 {
		t.Fatalf("Byte() = %d, want 7", got)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestReaderOverrunSetsError(t *testing.T) {
	r := NewReader([]byte{0x05, 'a'}) // каже 5 байтів, а є 1
	if got := r.String(LenByte); got != "" {
		t.Fatalf("String() = %q, want empty", got)
	}
	if r.Err() == nil {
		t.Fatal("Err() = nil, want overrun error")
	}
}
```

- [ ] **Step 3: Переконатися, що тест падає**

Run: `cd server && go test ./internal/protocol/ -v`
Expected: FAIL — `undefined: Encode`, `undefined: NewWriter`.

- [ ] **Step 4: Реалізувати `packet.go`**

```go
// Package protocol кодує й декодує пакети лобі-протоколу Cossacks 3.
//
// Формат перенесено з cossacks3-lan-server (c) 2018 Ereb, MIT.
// Заголовок 14 байт little-endian: size uint32 (довжина тіла), cmd uint16,
// id1 uint32, id2 uint32. Далі size байтів тіла.
package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	HeaderSize  = 14
	MaxBodySize = 0x100000
)

// LengthType — скільки байтів займає префікс довжини рядка.
type LengthType int

const (
	LenByte LengthType = iota
	LenShort
	LenInt
)

var ErrOverrun = errors.New("protocol: read past end of body")

type Packet struct {
	Cmd  uint16
	ID1  uint32
	ID2  uint32
	Body []byte
}

func Encode(p Packet) []byte {
	out := make([]byte, HeaderSize+len(p.Body))
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(p.Body)))
	binary.LittleEndian.PutUint16(out[4:6], p.Cmd)
	binary.LittleEndian.PutUint32(out[6:10], p.ID1)
	binary.LittleEndian.PutUint32(out[10:14], p.ID2)
	copy(out[HeaderSize:], p.Body)
	return out
}

type Reader struct {
	body []byte
	pos  int
	err  error
}

func NewReader(body []byte) *Reader { return &Reader{body: body} }

func (r *Reader) Err() error { return r.err }

func (r *Reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.pos+n > len(r.body) {
		r.err = ErrOverrun
		return nil
	}
	b := r.body[r.pos : r.pos+n]
	r.pos += n
	return b
}

func (r *Reader) Byte() uint8 {
	b := r.take(1)
	if b == nil {
		return 0
	}
	return b[0]
}

func (r *Reader) Short() uint16 {
	b := r.take(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}

func (r *Reader) Int() uint32 {
	b := r.take(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (r *Reader) Skip(n int) { r.take(n) }

func (r *Reader) String(lt LengthType) string {
	var n int
	switch lt {
	case LenByte:
		n = int(r.Byte())
	case LenShort:
		n = int(r.Short())
	case LenInt:
		n = int(r.Int())
	}
	b := r.take(n)
	if b == nil {
		return ""
	}
	return string(b)
}

type Writer struct{ buf []byte }

func NewWriter() *Writer { return &Writer{} }

func (w *Writer) Byte(b uint8) { w.buf = append(w.buf, b) }

func (w *Writer) Short(v uint16) {
	w.buf = binary.LittleEndian.AppendUint16(w.buf, v)
}

func (w *Writer) Int(v uint32) {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, v)
}

func (w *Writer) Raw(b []byte) { w.buf = append(w.buf, b...) }

func (w *Writer) String(s string, lt LengthType) {
	switch lt {
	case LenByte:
		w.Byte(uint8(len(s)))
	case LenShort:
		w.Short(uint16(len(s)))
	case LenInt:
		w.Int(uint32(len(s)))
	}
	w.buf = append(w.buf, s...)
}

func (w *Writer) Len() int { return len(w.buf) }

// PatchInt перезаписує 4 байти за зсувом off уже записаним значенням.
// Потрібно для 0x1bd, де довжина блоку пишеться на початку, а відома в кінці.
func (w *Writer) PatchInt(off int, v uint32) {
	binary.LittleEndian.PutUint32(w.buf[off:off+4], v)
}

func (w *Writer) Packet(cmd uint16, id1, id2 uint32) Packet {
	return Packet{Cmd: cmd, ID1: id1, ID2: id2, Body: w.buf}
}
```

- [ ] **Step 5: Тести проходять**

Run: `cd server && go test ./internal/protocol/ -v`
Expected: PASS (4 тести).

- [ ] **Step 6: Коміт**

```bash
git add server/go.mod server/internal/protocol
git commit -m "feat(protocol): packet encoding and reader/writer"
```

---

### Task 2: Читання кадрів із потоку

**Files:**
- Create: `server/internal/protocol/frame.go`
- Test: `server/internal/protocol/frame_test.go`

**Interfaces:**
- Consumes: `Packet`, `HeaderSize`, `MaxBodySize` із задачі 1.
- Produces: `protocol.ReadPacket(r io.Reader) (Packet, error)`; `protocol.ErrTooLarge`.

- [ ] **Step 1: Тест, що падає**

`server/internal/protocol/frame_test.go`:

```go
package protocol

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadPacketParsesTwoPacketsInARow(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(Encode(Packet{Cmd: 0x196, ID1: 3, Body: []byte("hi")}))
	buf.Write(Encode(Packet{Cmd: 0x1a0, ID1: 3}))

	p1, err := ReadPacket(&buf)
	if err != nil || p1.Cmd != 0x196 || string(p1.Body) != "hi" {
		t.Fatalf("first = %+v, err = %v", p1, err)
	}
	p2, err := ReadPacket(&buf)
	if err != nil || p2.Cmd != 0x1a0 || len(p2.Body) != 0 {
		t.Fatalf("second = %+v, err = %v", p2, err)
	}
	if _, err := ReadPacket(&buf); !errors.Is(err, io.EOF) {
		t.Fatalf("third err = %v, want EOF", err)
	}
}

func TestReadPacketRejectsOversizedBody(t *testing.T) {
	hdr := Encode(Packet{Cmd: 0x4b0})
	// підміняємо size на MaxBodySize+1
	hdr[0], hdr[1], hdr[2], hdr[3] = 0x01, 0x00, 0x10, 0x00
	if _, err := ReadPacket(bytes.NewReader(hdr)); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestReadPacketTruncatedBodyIsError(t *testing.T) {
	raw := Encode(Packet{Cmd: 0x4b0, Body: []byte{1, 2, 3, 4}})
	if _, err := ReadPacket(bytes.NewReader(raw[:HeaderSize+2])); err == nil {
		t.Fatal("err = nil, want unexpected EOF")
	}
}
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/protocol/ -run ReadPacket -v`
Expected: FAIL — `undefined: ReadPacket`.

- [ ] **Step 3: Реалізація**

`server/internal/protocol/frame.go`:

```go
package protocol

import (
	"encoding/binary"
	"errors"
	"io"
)

var ErrTooLarge = errors.New("protocol: body exceeds max packet size")

// ReadPacket читає один кадр. Повертає io.EOF, якщо потік завершився
// рівно на межі пакета.
func ReadPacket(r io.Reader) (Packet, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Packet{}, err
	}
	size := binary.LittleEndian.Uint32(hdr[0:4])
	if size > MaxBodySize {
		return Packet{}, ErrTooLarge
	}
	p := Packet{
		Cmd: binary.LittleEndian.Uint16(hdr[4:6]),
		ID1: binary.LittleEndian.Uint32(hdr[6:10]),
		ID2: binary.LittleEndian.Uint32(hdr[10:14]),
	}
	if size > 0 {
		p.Body = make([]byte, size)
		if _, err := io.ReadFull(r, p.Body); err != nil {
			return Packet{}, err
		}
	}
	return p, nil
}
```

- [ ] **Step 4: Тести проходять**

Run: `cd server && go test ./internal/protocol/ -v`
Expected: PASS (7 тестів).

- [ ] **Step 5: Коміт**

```bash
git add server/internal/protocol
git commit -m "feat(protocol): frame reader with size limits"
```

---

### Task 3: Стан гравців і кімнат

**Files:**
- Create: `server/internal/lobby/state.go`
- Test: `server/internal/lobby/state_test.go`

**Interfaces:**
- Consumes: —
- Produces: `lobby.Player{ID uint32; Name, Ver1, Ver2, Props string; Status uint8; Room *Room}`, метод `(*Player) JoinRoom(*Room)`, `(*Player) LeaveRoom()`; `lobby.Room{HostID uint32; Desc, Info string; Players []uint32; Hidden bool}`, методи `(*Room) AddPlayer(uint32)`, `(*Room) RemovePlayer(uint32)`; `lobby.NewPlayer(id uint32, name, ver1, ver2 string) *Player`; `lobby.NewRoom(hostID uint32, desc string) *Room`; `lobby.DefaultProps`.

Довідка (`reference/cpp-src/Player.hpp`, `Room.hpp`): статуси `0x01` у лобі, `0x03` учасник кімнати, `0x05` хост кімнати, `0x0b` учасник у грі, `0x0f` хост у грі. Нова кімната має `Info = "0"`.

- [ ] **Step 1: Тест, що падає**

`server/internal/lobby/state_test.go`:

```go
package lobby

import "testing"

func TestNewPlayerDefaults(t *testing.T) {
	p := NewPlayer(7, "Taras", "1.0.0.7", "2.0.7")
	if p.Status != 0x01 {
		t.Fatalf("Status = %#x, want 0x01", p.Status)
	}
	if p.Props != DefaultProps {
		t.Fatalf("Props = %q", p.Props)
	}
}

func TestJoinRoomHostGetsStatus5(t *testing.T) {
	host := NewPlayer(3, "Host", "1.0.0.7", "2.0.7")
	room := NewRoom(3, `"game"\t""\t008C7`)
	host.JoinRoom(room)
	if host.Status != 0x05 {
		t.Fatalf("host status = %#x, want 0x05", host.Status)
	}
	if len(room.Players) != 1 || room.Players[0] != 3 {
		t.Fatalf("room players = %v", room.Players)
	}
	if room.Info != "0" {
		t.Fatalf("Info = %q, want \"0\"", room.Info)
	}
}

func TestJoinRoomGuestGetsStatus3AndKeepsOrder(t *testing.T) {
	room := NewRoom(3, "d")
	host := NewPlayer(3, "Host", "", "")
	guest := NewPlayer(4, "Guest", "", "")
	host.JoinRoom(room)
	guest.JoinRoom(room)
	if guest.Status != 0x03 {
		t.Fatalf("guest status = %#x, want 0x03", guest.Status)
	}
	if got := room.Players; len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("players = %v, want [3 4]", got)
	}
}

func TestLeaveRoomResetsStatusAndRemovesID(t *testing.T) {
	room := NewRoom(3, "d")
	host := NewPlayer(3, "Host", "", "")
	guest := NewPlayer(4, "Guest", "", "")
	host.JoinRoom(room)
	guest.JoinRoom(room)
	guest.LeaveRoom()
	if guest.Status != 0x01 || guest.Room != nil {
		t.Fatalf("guest = %+v", guest)
	}
	if got := room.Players; len(got) != 1 || got[0] != 3 {
		t.Fatalf("players = %v, want [3]", got)
	}
}
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/lobby/ -v`
Expected: FAIL — `undefined: NewPlayer`.

- [ ] **Step 3: Реалізація**

`server/internal/lobby/state.go`:

```go
package lobby

// DefaultProps — рядок властивостей клієнта, який оригінальний сервер
// віддає для щойно створеного гравця (Player.hpp).
const DefaultProps = "pur|0|dlc|0|ram|4|sic|0|si1|0|si2|0|si3|0|snc||sn1||sn2||sn3|"

// Статуси гравця (Player.hpp).
const (
	StatusLobby     uint8 = 0x01
	StatusInRoom    uint8 = 0x03
	StatusRoomHost  uint8 = 0x05
	StatusInGame    uint8 = 0x0b
	StatusGameHost  uint8 = 0x0f
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
```

- [ ] **Step 4: Тести проходять**

Run: `cd server && go test ./internal/lobby/ -v`
Expected: PASS (4 тести).

- [ ] **Step 5: Коміт**

```bash
git add server/internal/lobby
git commit -m "feat(lobby): player and room state"
```

---

### Task 4: Реєстр клієнтів і маршрутизація

**Files:**
- Create: `server/internal/lobby/server.go`, `server/internal/lobby/fake_client_test.go`
- Test: `server/internal/lobby/server_test.go`

**Interfaces:**
- Consumes: `Player`, `Room` (задача 3), `protocol.Packet`, `protocol.Encode`.
- Produces: інтерфейс `lobby.Client{ID() uint32; SetID(uint32); Addr() string; Send([]byte); Close()}`; `lobby.Server` з методами `Connect(Client)`, `Disconnect(Client)`, `Handle(Client, protocol.Packet)`, `Snapshot() Snapshot`; `lobby.New(name string) *Server`; тестовий хелпер `newFakeClient(addr string) *fakeClient` з полем `sent []protocol.Packet`.

Довідка (`Lobby.cpp`, `Lobby::send`): цілі надсилання — `Source`, `Id2`, `Everyone`, `EveryoneButSource`, `RoomHost`, `EveryoneInRoom`, `EveryoneInRoomButSource`, `PropagateInRoom`. `PropagateInRoom`: якщо джерело — хост кімнати, пакет іде всім у кімнаті, крім джерела; інакше — тільки хосту.

- [ ] **Step 1: Тестовий клієнт**

`server/internal/lobby/fake_client_test.go`:

```go
package lobby

import "github.com/zeror5162-creator/cossacs/server/internal/protocol"

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
	p, err := protocol.ReadPacket(bytesReader(b))
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
```

Додати в той самий файл хелпер:

```go
import "bytes"

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
```

- [ ] **Step 2: Тест маршрутизації, що падає**

`server/internal/lobby/server_test.go`:

```go
package lobby

import "testing"

func TestConnectAssignsIncrementingIDs(t *testing.T) {
	s := New("test")
	a, b := newFakeClient("1.1.1.1"), newFakeClient("2.2.2.2")
	s.Connect(a)
	s.Connect(b)
	if a.ID() != 1 || b.ID() != 2 {
		t.Fatalf("ids = %d, %d; want 1, 2", a.ID(), b.ID())
	}
}

func TestSendPropagateInRoomFromHostGoesToOthers(t *testing.T) {
	s := New("test")
	host, guest, other := newFakeClient("h"), newFakeClient("g"), newFakeClient("o")
	loginAll(t, s, host, guest, other)
	createRoom(t, s, host)
	joinRoom(t, s, guest, host.ID())

	host.sent, guest.sent, other.sent = nil, nil, nil
	s.Handle(host, packet(0x4b0, host.ID(), 0, []byte{9, 9}))

	if len(guest.sent) != 1 || guest.sent[0].Cmd != 0x4b0 {
		t.Fatalf("guest got %v, want one 0x4b0", guest.cmds())
	}
	if len(host.sent) != 0 {
		t.Fatalf("host got %v, want nothing", host.cmds())
	}
	if len(other.sent) != 0 {
		t.Fatalf("player outside room got %v, want nothing", other.cmds())
	}
}

func TestSendPropagateInRoomFromGuestGoesToHostOnly(t *testing.T) {
	s := New("test")
	host, g1, g2 := newFakeClient("h"), newFakeClient("g1"), newFakeClient("g2")
	loginAll(t, s, host, g1, g2)
	createRoom(t, s, host)
	joinRoom(t, s, g1, host.ID())
	joinRoom(t, s, g2, host.ID())

	host.sent, g1.sent, g2.sent = nil, nil, nil
	s.Handle(g1, packet(0x4b0, g1.ID(), 0, []byte{1}))

	if len(host.sent) != 1 {
		t.Fatalf("host got %v, want one packet", host.cmds())
	}
	if len(g2.sent) != 0 {
		t.Fatalf("other guest got %v, want nothing", g2.cmds())
	}
}
```

Хелпери сценарію в тому ж файлі (використовуються далі всіма тестами обробників):

```go
import "github.com/zeror5162-creator/cossacs/server/internal/protocol"

func packet(cmd uint16, id1, id2 uint32, body []byte) protocol.Packet {
	return protocol.Packet{Cmd: cmd, ID1: id1, ID2: id2, Body: body}
}

func login(t *testing.T, s *Server, c *fakeClient, nick string) {
	t.Helper()
	s.Connect(c)
	w := protocol.NewWriter()
	w.String("1.0.0.7", protocol.LenByte)
	w.String("2.0.7", protocol.LenByte)
	w.String("mail@example.com", protocol.LenByte)
	w.String("", protocol.LenByte)
	w.String(nick, protocol.LenByte)
	s.Handle(c, w.Packet(0x19a, c.ID(), 0))
}

func loginAll(t *testing.T, s *Server, cs ...*fakeClient) {
	t.Helper()
	for i, c := range cs {
		login(t, s, c, string(rune('A'+i))+"player")
	}
}

func createRoom(t *testing.T, s *Server, c *fakeClient) {
	t.Helper()
	w := protocol.NewWriter()
	w.Int(8)
	w.Byte(0)
	w.String(`"room"\t""\t008C7`, protocol.LenByte)
	w.String("0", protocol.LenByte)
	w.Int(42)
	w.Short(0)
	s.Handle(c, w.Packet(0x19c, c.ID(), 0))
}

func joinRoom(t *testing.T, s *Server, c *fakeClient, hostID uint32) {
	t.Helper()
	w := protocol.NewWriter()
	w.Int(hostID)
	s.Handle(c, w.Packet(0x19e, c.ID(), 0))
}
```

- [ ] **Step 3: Перевірити падіння**

Run: `cd server && go test ./internal/lobby/ -run Send -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 4: Реалізувати `server.go`**

```go
package lobby

import (
	"log/slog"
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
		for id := range s.clients {
			deliver(id)
		}
	case toEveryoneButSource:
		for id := range s.clients {
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
```

**Важливо про порядок обходу:** `toEveryone` і `toEveryoneButSource` в оригіналі йдуть по `std::map`, тобто **за зростанням id**. Мапа Go порядку не гарантує, тому тут і в усіх інших місцях, де оригінал ітерує по мапі, треба обходити **відсортовані id**. Додати в `server.go`:

```go
import "sort"

// sortedIDs повертає id клієнтів за зростанням — як std::map в оригіналі.
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
```

і використати `s.sortedClientIDs()` у гілках `toEveryone` та `toEveryoneButSource` замість `for id := range s.clients`.

- [ ] **Step 5: Тимчасова заглушка `Handle`**

Щоб задача компілювалася до появи обробників, додати в `server.go`:

```go
// Handle обробляє один вхідний пакет. Обробники — у handlers.go.
func (s *Server) Handle(c Client, p protocol.Packet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handle(c, p)
}
```

- [ ] **Step 6: Коміт**

Тести цієї задачі проходять лише після задач 5–7, тому комітимо без них:

```bash
git add server/internal/lobby
git commit -m "feat(lobby): client registry and send targeting"
```

---

### Task 5: Обробники пересилання

**Files:**
- Create: `server/internal/lobby/handlers.go`
- Test: `server/internal/lobby/handlers_forward_test.go`

**Interfaces:**
- Consumes: `Server.send`, `sendTo` (задача 4).
- Produces: метод `(*Server) handle(Client, protocol.Packet)` із гілками команд пересилання.

Відповідність (з `Lobby.cpp`, вхідний пакет пересилається без змін тіла):

| Вхід | Вихідний cmd | Ціль |
|---|---|---|
| `0x4b0` | `0x4b0` | `toPropagateInRoom` |
| `0x032` | `0x032` | `toEveryoneInRoomButSource` |
| `0x456` | `0x456` | `toPropagateInRoom` |
| `0x457` | `0x457` | `toEveryoneInRoomButSource` |
| `0x460` | `0x460` | `toRoomHost` |
| `0x461` | `0x461` | `toEveryoneInRoomButSource` |
| `0x064` | `0x064` | `toRoomHost` |
| `0x065` | `0x065` | `toRoomHost` |
| `0x066` | `0x066` | `toSource` |
| `0x1ab` | `0x1ac` | `toEveryone` |
| `0x0c8` | `0x0c8` | `toEveryoneButSource` |
| `0x0c9` | `0x0c9` | `toID2` |
| `0x1af` | `0x1af` | `toEveryone` |
| `0x1bb` | `0x1bc` | `toEveryoneInRoom` |
| `0x194` | `0x195` | `toEveryoneInRoom` |
| `0x1b7`, `0x198`, `0x1b3`(відповідь) | — | нічого не надсилається |

`0x196` (повідомлення в лобі) → `0x197`: якщо `id2 == 0` — `toEveryone`; якщо `id1 == id2` — `toSource`; інакше — `toSource` **і** `toID2` (два надсилання).

- [ ] **Step 1: Тест, що падає**

`server/internal/lobby/handlers_forward_test.go`:

```go
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
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/lobby/ -run Forward -v`
Expected: FAIL — `undefined: (*Server).handle` або пакети не надходять.

- [ ] **Step 3: Реалізувати каркас і гілки пересилання**

`server/internal/lobby/handlers.go`:

```go
// Обробники команд лобі-протоколу.
//
// Логіка й формати перенесені з cossacks3-lan-server/src/Lobby.cpp,
// (c) 2018 Ereb @ habrahabr.ru, ліцензія MIT.
package lobby

import (
	"log/slog"
	"strconv"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

// forward пересилає вхідний пакет без змін тіла, можливо з іншим cmd.
func (s *Server) forward(p protocol.Packet, srcID uint32, cmd uint16, target sendTo) {
	out := p
	out.Cmd = cmd
	s.send(out, srcID, target)
}

func (s *Server) handle(c Client, p protocol.Packet) {
	id := c.ID()

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
```

- [ ] **Step 4: Тести проходять**

Run: `cd server && go test ./internal/lobby/ -run "Forward|Chat|Status" -v`
Expected: PASS.

- [ ] **Step 5: Коміт**

```bash
git add server/internal/lobby
git commit -m "feat(lobby): forwarding handlers"
```

---

### Task 6: Логін, профіль, версія

**Files:**
- Modify: `server/internal/lobby/handlers.go`
- Create: `server/internal/lobby/handlers_login_test.go`

**Interfaces:**
- Consumes: `handle` (задача 5), `NewPlayer` (задача 3).
- Produces: гілки `0x19a`, `0x1a8`, `0x1ad`, `0x1b3`, `0x192`; функція `lobby.NormalizeNick(string) string`.

Правила з `Lobby.cpp`:
- **`0x19a`**: читає `ver1`, `ver2`, пропускає email і пароль (`Skip(int(Byte()))`), читає `game key` як нік. Нік нормалізується: менше 4 символів — доповнення `_` до 4; більше 16 — обрізання до 16; символи поза `abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789()+-_.[]` → `_`. Порядок в оригіналі: спершу довжина, потім заміна символів.
- Відповідь `0x19b` (id1 = id2 = id клієнта), тіло: `byte 0`, `string nick`, `byte 0`, 5 × `int 0`, `string props`; далі для кожного гравця **в порядку зростання id** (включно з новим): `int id`, `byte status`, `string name`, `byte 0`, `string props`, 7 × `int 0`; далі `int 0`; далі кімнати **у зворотному порядку id**, пропускаючи `Hidden`: `int hostID`, `int 8`, `string desc`, `string info`, `int 0`, `short 0`, `int len(players)`, далі id гравців кімнати у **зворотному** порядку; наприкінці `int 0`.
- Потім `0x1a6` (id1 = id клієнта, id2 = 0) усім: `string name`, `byte 0`, `string props`, `byte status`.
- **`0x1a8`**: тіло без змін + `byte 1`, cmd `0x1a9`, id1 = id2 = 0, тільки джерелу.
- **`0x1ad`**: відповідь `0x1ae`, id1 = 0, id2 = id гравця, тіло: `string ver1`, `string ver2`, `int 0`. **Тіло дописується до вхідного** (в оригіналі `write_string` продовжує з позиції після читання заголовка, тобто тіло відповіді = порожнє + три записи; вхідне тіло не зберігається).
- **`0x1b3`**: читає 3 рядки, четвертий — нові `props`, зберігає їх. Відповіді нема.
- **`0x192`**: читає `int` id запитаного гравця; відповідь `0x193`, id1 = запитаний id, id2 = id1 вхідного пакета, тільки джерелу; тіло: `int infoID`, `byte status`, `string name`, `byte 0`, 5 × `int 0`, `string props`.

- [ ] **Step 1: Тести, що падають**

`server/internal/lobby/handlers_login_test.go`:

```go
package lobby

import (
	"testing"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

func TestNormalizeNick(t *testing.T) {
	cases := map[string]string{
		"ab":                "ab__",
		"Taras":             "Taras",
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
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/lobby/ -run "Nick|Login|Lobby|Version|BeforeLogin" -v`
Expected: FAIL — `undefined: NormalizeNick`, `undefined: Snapshot`.

- [ ] **Step 3: Реалізувати**

Додати в `handlers.go` нормалізацію ніку:

```go
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
```

(додати `"strings"` в імпорти)

Додати гілки в `handle` **перед** `default`:

```go
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
```

Крім того, на початку `handle` додати перевірку «залогінений», яка захищає всі гілки, крім логіну й email-форми:

```go
	if _, ok := s.players[id]; !ok {
		switch p.Cmd {
		case 0x19a, 0x1a8, 0x198:
		default:
			slog.Debug("packet before login", "server", s.Name, "cmd", p.Cmd, "id", id)
			return
		}
	}
```

Обробник логіну (новий метод у `handlers.go`):

```go
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
```

Додати в `server.go` знімок стану (знадобиться і тут, і на етапі 2):

```go
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
```

- [ ] **Step 4: Тести проходять**

Run: `cd server && go test ./internal/lobby/ -v`
Expected: PASS (усі тести задач 3–6).

- [ ] **Step 5: Коміт**

```bash
git add server/internal/lobby
git commit -m "feat(lobby): login, profile and version handlers"
```

---

### Task 7: Кімнати, старт гри, передача ролі хоста

**Files:**
- Modify: `server/internal/lobby/handlers.go`
- Create: `server/internal/lobby/handlers_rooms_test.go`

**Interfaces:**
- Consumes: `handle`, `handleLogin`, стан із задач 3–6.
- Produces: гілки `0x19c`, `0x19e`, `0x1a0`, `0x1a2`, `0x1aa`, `0x1b5`; метод `(*Server) leaveRoom(c Client, p protocol.Packet)`.

Формати (`Lobby.cpp`):
- **`0x19c` створення кімнати**: пропустити 5 байтів, прочитати `desc`, `info`, `magic int`. Створити кімнату з ключем = id хоста, `JoinRoom`. Сповіщення `0x19d` усім, id1 = `p.ID1`, тіло: `byte 7`, `int 8`, `string desc`, `string info`, `int magic`, `short 0`.
- **`0x19e` вхід у кімнату**: `int hostID`. Сповіщення `0x19f` усім, id1 = `p.ID1`, тіло: `int hostID`, `byte status` гравця після входу.
- **`0x1a0` вихід**: якщо гравець не в кімнаті — нічого. `roomHostLeaving = status == 0x05 || status == 0x0f`; `hostTransferNeeded = status == 0x0f && len(players) > 1`; `newHostID = players[len-1]`. Якщо виходить хост — усі гравці кімнати виходять, тіло `0x1a1`: `byte 1`, `int len(players)`, далі для кожного `int id`, `byte status` (після `LeaveRoom`). Інакше виходить лише гравець: `byte 0`, `int 1`, `int id`, `byte status`. `0x1a1` надсилається всім.
  Якщо потрібна передача ролі — `0x1bd` новому хосту (id1 = id2 = newHostID) з блоком ключ-значення (див. код нижче) і `0x1be` кожному іншому гравцю кімнати, крім нового хоста (id1 = newHostID, id2 = id адресата). Кімната видаляється, якщо виходив хост.
- **`0x1a2` старт гри**: кімната ховається, статуси: хост `0x0f`, решта `0x0b`. Сповіщення `0x1a3` усім, тіло: `int len(players)`, далі **у зворотному порядку** `int id`, `byte status`.
- **`0x1aa` оновлення кімнати**: читає `desc` і `info`, зберігає `info`. Сповіщення `0x1a5` усім, тіло: `int 8`, `string desc`, `string info`, `int 0`, `short 0`, `int len(players)`, далі **у зворотному порядку** `int id`, `byte status`.
- **`0x1b5` кік із гри**: `int kickID`; переслати як `0x1b6` усім; потім `0x1a1` усім з id1 = kickID, тіло: `byte 0`, `int 1`, `int kickID`, `byte 1`.

- [ ] **Step 1: Тести, що падають**

`server/internal/lobby/handlers_rooms_test.go`:

```go
package lobby

import (
	"testing"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

func TestCreateRoomNotifiesEveryoneWith19d(t *testing.T) {
	s := New("test")
	host, other := newFakeClient("h"), newFakeClient("o")
	loginAll(t, s, host, other)
	host.sent, other.sent = nil, nil

	createRoom(t, s, host)

	for _, c := range []*fakeClient{host, other} {
		if len(c.sent) != 1 || c.sent[0].Cmd != 0x19d {
			t.Fatalf("%s got %v, want one 0x19d", c.Addr(), c.cmds())
		}
	}
	r := protocol.NewReader(host.sent[0].Body)
	if b := r.Byte(); b != 7 {
		t.Fatalf("first byte = %d, want 7", b)
	}
	if v := r.Int(); v != 8 {
		t.Fatalf("int = %d, want 8", v)
	}
	if d := r.String(protocol.LenByte); d != `"room"\t""\t008C7` {
		t.Fatalf("desc = %q", d)
	}
}

func TestJoinNonexistentRoomIsIgnored(t *testing.T) {
	s := New("test")
	c := newFakeClient("c")
	loginAll(t, s, c)
	c.sent = nil

	joinRoom(t, s, c, 999)

	if len(c.sent) != 0 {
		t.Fatalf("got %v, want nothing", c.cmds())
	}
	if pl := s.playerForTest(c.ID()); pl.Room != nil {
		t.Fatal("player joined a room that does not exist")
	}
}

func TestStartGameHidesRoomFromNewcomers(t *testing.T) {
	s := New("test")
	host, guest := newFakeClient("h"), newFakeClient("g")
	loginAll(t, s, host, guest)
	createRoom(t, s, host)
	joinRoom(t, s, guest, host.ID())
	s.Handle(host, packet(0x1a2, host.ID(), 0, nil))

	newcomer := newFakeClient("n")
	login(t, s, newcomer, "Newbie")

	r := protocol.NewReader(newcomer.sent[0].Body)
	skipLobbyHeader(t, r)
	if r.Int() != 0 {
		t.Fatal("expected zero rooms for newcomer")
	}
}

func TestHostLeavingDuringGameTransfersHost(t *testing.T) {
	s := New("test")
	host, g1, g2 := newFakeClient("h"), newFakeClient("g1"), newFakeClient("g2")
	loginAll(t, s, host, g1, g2)
	createRoom(t, s, host)
	joinRoom(t, s, g1, host.ID())
	joinRoom(t, s, g2, host.ID())
	s.Handle(host, packet(0x1a2, host.ID(), 0, nil))
	host.sent, g1.sent, g2.sent = nil, nil, nil

	s.Handle(host, packet(0x1a0, host.ID(), 0, nil))

	newHost, others := g2, []*fakeClient{g1}
	if !hasCmd(newHost.cmds(), 0x1bd) {
		t.Fatalf("new host got %v, want 0x1bd", newHost.cmds())
	}
	for _, c := range others {
		if !hasCmd(c.cmds(), 0x1be) {
			t.Fatalf("%s got %v, want 0x1be", c.Addr(), c.cmds())
		}
	}
	if got := s.Snapshot().Rooms; got != 0 {
		t.Fatalf("rooms = %d, want 0 (old room deleted)", got)
	}
}

func hasCmd(cmds []uint16, want uint16) bool {
	for _, c := range cmds {
		if c == want {
			return true
		}
	}
	return false
}

// skipLobbyHeader перемотує 0x19b до списку кімнат.
func skipLobbyHeader(t *testing.T, r *protocol.Reader) {
	t.Helper()
	r.Byte()
	r.String(protocol.LenByte)
	r.Byte()
	for i := 0; i < 5; i++ {
		r.Int()
	}
	r.String(protocol.LenByte)
	for {
		if r.Int() == 0 {
			return
		}
		r.Byte()
		r.String(protocol.LenByte)
		r.Byte()
		r.String(protocol.LenByte)
		for i := 0; i < 7; i++ {
			r.Int()
		}
	}
}
```

Додати тестовий доступ до стану в `server.go`:

```go
// playerForTest повертає гравця за id. Використовується лише тестами.
func (s *Server) playerForTest(id uint32) *Player {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.players[id]
}
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/lobby/ -run "Room|Game|Host" -v`
Expected: FAIL — кімнати не створюються, `0x19d` не надходить.

- [ ] **Step 3: Реалізувати гілки кімнат**

Додати в `handle` перед `default`:

```go
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
		if pl.Room != nil {
			return // вже в кімнаті
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
		if pl.Room != nil {
			return
		}
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
```

Окремий метод виходу з кімнати:

```go
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
```

- [ ] **Step 4: Тести проходять**

Run: `cd server && go test ./internal/lobby/ -v`
Expected: PASS.

- [ ] **Step 5: Коміт**

```bash
git add server/internal/lobby
git commit -m "feat(lobby): room lifecycle, game start and host transfer"
```

---

### Task 8: Від'єднання

**Files:**
- Modify: `server/internal/lobby/server.go`
- Create: `server/internal/lobby/disconnect_test.go`

**Interfaces:**
- Consumes: `leaveRoom` (задача 7).
- Produces: `(*Server) Disconnect(Client)`.

Порядок в оригіналі (`Lobby::disconnect`): спершу клієнт видаляється з реєстру (щоб не надсилати йому нічого), потім, якщо гравець був у кімнаті, виконується та сама логіка, що й для `0x1a0`, потім гравець видаляється, і всім іде `0x1a7` (id1 = id, тіло порожнє).

- [ ] **Step 1: Тест, що падає**

`server/internal/lobby/disconnect_test.go`:

```go
package lobby

import "testing"

func TestDisconnectNotifiesEveryoneWith1a7(t *testing.T) {
	s := New("test")
	a, b := newFakeClient("a"), newFakeClient("b")
	loginAll(t, s, a, b)
	a.sent, b.sent = nil, nil

	s.Disconnect(a)

	if len(b.sent) != 1 || b.sent[0].Cmd != 0x1a7 || b.sent[0].ID1 != a.ID() {
		t.Fatalf("remaining client got %v", b.sent)
	}
	if len(a.sent) != 0 {
		t.Fatalf("leaving client got %v, want nothing", a.cmds())
	}
	if got := s.Snapshot().Players; got != 1 {
		t.Fatalf("players = %d, want 1", got)
	}
}

func TestDisconnectOfRoomHostCleansRoomUp(t *testing.T) {
	s := New("test")
	host, guest := newFakeClient("h"), newFakeClient("g")
	loginAll(t, s, host, guest)
	createRoom(t, s, host)
	joinRoom(t, s, guest, host.ID())
	host.sent, guest.sent = nil, nil

	s.Disconnect(host)

	if !hasCmd(guest.cmds(), 0x1a1) {
		t.Fatalf("guest got %v, want 0x1a1 before 0x1a7", guest.cmds())
	}
	if !hasCmd(guest.cmds(), 0x1a7) {
		t.Fatalf("guest got %v, want 0x1a7", guest.cmds())
	}
	if got := s.Snapshot().Rooms; got != 0 {
		t.Fatalf("rooms = %d, want 0", got)
	}
	if pl := s.playerForTest(guest.ID()); pl.Room != nil {
		t.Fatal("guest is still linked to a deleted room")
	}
}

func TestDisconnectBeforeLoginIsSilent(t *testing.T) {
	s := New("test")
	a, b := newFakeClient("a"), newFakeClient("b")
	loginAll(t, s, a)
	s.Connect(b)
	a.sent = nil

	s.Disconnect(b)

	if len(a.sent) != 0 {
		t.Fatalf("got %v, want nothing", a.cmds())
	}
}
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/lobby/ -run Disconnect -v`
Expected: FAIL — `undefined: (*Server).Disconnect`.

- [ ] **Step 3: Реалізація**

Додати в `server.go`:

```go
func (s *Server) Disconnect(c Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := c.ID()
	delete(s.clients, id) // спершу, щоб не слати пакети мертвому сокету
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
```

- [ ] **Step 4: Тести проходять**

Run: `cd server && go test ./internal/lobby/ -v`
Expected: PASS.

- [ ] **Step 5: Коміт**

```bash
git add server/internal/lobby
git commit -m "feat(lobby): disconnect cleanup and notification"
```

---

### Task 9: TCP-слухач і сесії

**Files:**
- Create: `server/internal/netsrv/listener.go`
- Test: `server/internal/netsrv/listener_test.go`

**Interfaces:**
- Consumes: `lobby.Server`, `protocol.ReadPacket`, `protocol.Encode`.
- Produces: `netsrv.Listen(ctx context.Context, addr string, lb *lobby.Server, opts Options) (*Listener, error)`; `netsrv.Options{LoginTimeout time.Duration; SendQueue int; MaxConnPerIP int}`; методи `(*Listener) Addr() string`, `(*Listener) Close() error`.

Вимоги:
- одна горутина на читання, одна на запис, буферизований канал надсилань (за замовчуванням 64); переповнення черги → з'єднання закривається (повільний клієнт не блокує лобі);
- `MaxConnPerIP` (за замовчуванням 4, `-1` = без обмеження) — нове з'єднання з IP, який вичерпав ліміт, одразу закривається до будь-якого читання;
- паніка в обробці пакета не валить процес: `recover()` у горутині з'єднання, з'єднання закривається, помилка логується;
- `LoginTimeout` (за замовчуванням 30 с) — дедлайн читання до першого успішного пакета; після цього дедлайн читання знімається;
- закриття `Listener` розриває всі активні з'єднання.

- [ ] **Step 1: Тест, що падає**

`server/internal/netsrv/listener_test.go`:

```go
package netsrv

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/zeror5162-creator/cossacs/server/internal/lobby"
	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

func dialAndLogin(t *testing.T, addr, nick string) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	w := protocol.NewWriter()
	w.String("1.0.0.7", protocol.LenByte)
	w.String("2.0.7", protocol.LenByte)
	w.String("a@b.c", protocol.LenByte)
	w.String("", protocol.LenByte)
	w.String(nick, protocol.LenByte)
	if _, err := conn.Write(protocol.Encode(w.Packet(0x19a, 0, 0))); err != nil {
		t.Fatalf("write: %v", err)
	}
	return conn
}

func TestLoginOverRealSocketReturns19b(t *testing.T) {
	lb := lobby.New("test")
	l, err := Listen(context.Background(), "127.0.0.1:0", lb, Options{})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	conn := dialAndLogin(t, l.Addr(), "Alpha")
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	p, err := protocol.ReadPacket(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if p.Cmd != 0x19b {
		t.Fatalf("cmd = %#x, want 0x19b", p.Cmd)
	}
}

func TestOversizedPacketClosesConnectionAndServerSurvives(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb, Options{})
	defer l.Close()

	bad, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// заголовок із розміром 0x100001
	bad.Write([]byte{0x01, 0x00, 0x10, 0x00, 0xb0, 0x04, 0, 0, 0, 0, 0, 0, 0, 0})
	bad.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := protocol.ReadPacket(bad); err == nil {
		t.Fatal("connection stayed open after oversized packet")
	}
	bad.Close()

	// сервер продовжує працювати
	good := dialAndLogin(t, l.Addr(), "Beta")
	defer good.Close()
	good.SetReadDeadline(time.Now().Add(2 * time.Second))
	if p, err := protocol.ReadPacket(good); err != nil || p.Cmd != 0x19b {
		t.Fatalf("second client: p = %+v, err = %v", p, err)
	}
}

func TestConnectionsPerIPAreLimited(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb, Options{MaxConnPerIP: 2})
	defer l.Close()

	first := dialAndLogin(t, l.Addr(), "One")
	defer first.Close()
	second := dialAndLogin(t, l.Addr(), "Two")
	defer second.Close()

	third, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer third.Close()
	third.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := third.Read(buf); err == nil {
		t.Fatal("third connection from same IP was accepted")
	}

	// наявні з'єднання не постраждали
	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	if p, err := protocol.ReadPacket(first); err != nil || p.Cmd != 0x19b {
		t.Fatalf("first connection broken: p = %+v, err = %v", p, err)
	}
}

func TestIdleConnectionIsClosedAfterLoginTimeout(t *testing.T) {
	lb := lobby.New("test")
	l, _ := Listen(context.Background(), "127.0.0.1:0", lb,
		Options{LoginTimeout: 200 * time.Millisecond})
	defer l.Close()

	conn, err := net.Dial("tcp", l.Addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("idle connection was not closed")
	}
}
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/netsrv/ -v`
Expected: FAIL — `undefined: Listen`.

- [ ] **Step 3: Реалізація**

`server/internal/netsrv/listener.go`:

```go
// Package netsrv приймає TCP-з'єднання й подає їх у лобі.
package netsrv

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/zeror5162-creator/cossacs/server/internal/lobby"
	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

type Options struct {
	LoginTimeout time.Duration // 0 → 30s
	SendQueue    int           // 0 → 64
	MaxConnPerIP int           // 0 → 4; -1 → без обмеження
}

type Listener struct {
	ln    net.Listener
	lobby *lobby.Server
	opts  Options

	mu      sync.Mutex
	conns   map[*session]struct{}
	perIP   map[string]int
}

func Listen(ctx context.Context, addr string, lb *lobby.Server, opts Options) (*Listener, error) {
	if opts.LoginTimeout == 0 {
		opts.LoginTimeout = 30 * time.Second
	}
	if opts.SendQueue == 0 {
		opts.SendQueue = 64
	}
	if opts.MaxConnPerIP == 0 {
		opts.MaxConnPerIP = 4
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	l := &Listener{ln: ln, lobby: lb, opts: opts,
		conns: make(map[*session]struct{}), perIP: make(map[string]int)}
	go l.acceptLoop(ctx)
	return l, nil
}

func (l *Listener) Addr() string { return l.ln.Addr().String() }

func (l *Listener) Close() error {
	err := l.ln.Close()
	l.mu.Lock()
	for s := range l.conns {
		s.Close()
	}
	l.mu.Unlock()
	return err
}

func (l *Listener) acceptLoop(ctx context.Context) {
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			return
		}
		s := newSession(conn, l.lobby, l.opts)

		l.mu.Lock()
		if l.opts.MaxConnPerIP > 0 && l.perIP[s.addr] >= l.opts.MaxConnPerIP {
			l.mu.Unlock()
			slog.Warn("connection limit per ip", "addr", s.addr,
				"limit", l.opts.MaxConnPerIP)
			conn.Close()
			continue
		}
		l.perIP[s.addr]++
		l.conns[s] = struct{}{}
		l.mu.Unlock()

		go func() {
			s.run()
			l.mu.Lock()
			delete(l.conns, s)
			if l.perIP[s.addr] <= 1 {
				delete(l.perIP, s.addr)
			} else {
				l.perIP[s.addr]--
			}
			l.mu.Unlock()
		}()
		_ = ctx
	}
}

type session struct {
	conn  net.Conn
	lobby *lobby.Server
	opts  Options

	id       uint32
	addr     string
	out      chan []byte
	closeOne sync.Once
	done     chan struct{}
}

func newSession(conn net.Conn, lb *lobby.Server, opts Options) *session {
	host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	return &session{
		conn: conn, lobby: lb, opts: opts, addr: host,
		out:  make(chan []byte, opts.SendQueue),
		done: make(chan struct{}),
	}
}

func (s *session) ID() uint32      { return s.id }
func (s *session) SetID(id uint32) { s.id = id }
func (s *session) Addr() string    { return s.addr }

// Send не блокує лобі: якщо клієнт не встигає читати, з'єднання рветься.
func (s *session) Send(b []byte) {
	select {
	case s.out <- b:
	default:
		slog.Warn("send queue overflow", "addr", s.addr, "id", s.id)
		s.Close()
	}
}

func (s *session) Close() {
	s.closeOne.Do(func() {
		close(s.done)
		s.conn.Close()
	})
}

func (s *session) run() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic in session", "addr", s.addr, "id", s.id, "panic", r)
		}
		s.Close()
		s.lobby.Disconnect(s)
	}()

	s.lobby.Connect(s)
	go s.writeLoop()

	deadline := time.Now().Add(s.opts.LoginTimeout)
	_ = s.conn.SetReadDeadline(deadline)
	authed := false

	for {
		p, err := protocol.ReadPacket(s.conn)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				slog.Debug("read error", "addr", s.addr, "id", s.id, "err", err)
			}
			return
		}
		if !authed {
			authed = true
			_ = s.conn.SetReadDeadline(time.Time{})
		}
		s.lobby.Handle(s, p)
		select {
		case <-s.done:
			return
		default:
		}
	}
}

func (s *session) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case b := <-s.out:
			if _, err := s.conn.Write(b); err != nil {
				slog.Debug("write error", "addr", s.addr, "id", s.id, "err", err)
				s.Close()
				return
			}
		}
	}
}
```

- [ ] **Step 4: Тести проходять**

Run: `cd server && go test ./... -v`
Expected: PASS у всіх пакетах.

- [ ] **Step 5: Коміт**

```bash
git add server/internal/netsrv
git commit -m "feat(netsrv): tcp listener with sessions and limits"
```

---

### Task 10: Конфіг і запуск процесу

**Files:**
- Create: `server/internal/config/config.go`, `server/cmd/c3hub/main.go`, `deploy/config.example.toml`
- Test: `server/internal/config/config_test.go`

**Interfaces:**
- Consumes: `netsrv.Listen`, `lobby.New`.
- Produces: `config.Config{DataDir string; Servers []ServerConfig}`, `config.ServerConfig{Name string; Port int; MaxConnPerIP int; Description string}`, `config.Load(path string) (Config, error)`.

- [ ] **Step 1: Тест, що падає**

`server/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadParsesServers(t *testing.T) {
	cfg, err := Load(writeTemp(t, `
data_dir = "/var/lib/c3hub"

[[server]]
name = "Основний"
port = 31523

[[server]]
name = "Турнір"
port = 31524
max_conn_per_ip = 2
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("servers = %d, want 2", len(cfg.Servers))
	}
	if cfg.Servers[0].MaxConnPerIP != 4 {
		t.Fatalf("default MaxConnPerIP = %d, want 4", cfg.Servers[0].MaxConnPerIP)
	}
	if cfg.Servers[1].MaxConnPerIP != 2 {
		t.Fatalf("explicit MaxConnPerIP = %d, want 2", cfg.Servers[1].MaxConnPerIP)
	}
}

func TestLoadRejectsDuplicatePorts(t *testing.T) {
	_, err := Load(writeTemp(t, `
[[server]]
name = "A"
port = 31523

[[server]]
name = "B"
port = 31523
`))
	if err == nil {
		t.Fatal("err = nil, want duplicate port error")
	}
}

func TestLoadRejectsEmptyServerList(t *testing.T) {
	if _, err := Load(writeTemp(t, `data_dir = "/tmp"`)); err == nil {
		t.Fatal("err = nil, want error for config without servers")
	}
}
```

- [ ] **Step 2: Перевірити падіння**

Run: `cd server && go test ./internal/config/ -v`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Реалізація**

```bash
cd server && go get github.com/BurntSushi/toml@latest
```

`server/internal/config/config.go`:

```go
// Package config читає TOML-конфіг c3hub.
package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type ServerConfig struct {
	Name         string `toml:"name"`
	Port         int    `toml:"port"`
	MaxConnPerIP int    `toml:"max_conn_per_ip"`
	Description  string `toml:"description"`
}

type Config struct {
	DataDir string         `toml:"data_dir"`
	Servers []ServerConfig `toml:"server"`
}

func Load(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if len(cfg.Servers) == 0 {
		return Config{}, fmt.Errorf("config has no [[server]] entries")
	}
	seen := make(map[int]string, len(cfg.Servers))
	for i := range cfg.Servers {
		s := &cfg.Servers[i]
		if s.Name == "" {
			return Config{}, fmt.Errorf("server #%d has no name", i+1)
		}
		if s.Port < 1 || s.Port > 65535 {
			return Config{}, fmt.Errorf("server %q has invalid port %d", s.Name, s.Port)
		}
		if prev, dup := seen[s.Port]; dup {
			return Config{}, fmt.Errorf("port %d used by both %q and %q", s.Port, prev, s.Name)
		}
		seen[s.Port] = s.Name
		if s.MaxConnPerIP == 0 {
			s.MaxConnPerIP = 4
		}
	}
	return cfg, nil
}
```

`server/cmd/c3hub/main.go`:

```go
// c3hub — лобі-сервер Cossacks 3: один процес, кілька ігрових портів.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/zeror5162-creator/cossacs/server/internal/config"
	"github.com/zeror5162-creator/cossacs/server/internal/lobby"
	"github.com/zeror5162-creator/cossacs/server/internal/netsrv"
)

func main() {
	cfgPath := flag.String("config", "/etc/c3hub/config.toml", "path to config file")
	debug := flag.Bool("debug", false, "verbose logging")
	flag.Parse()

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr,
		&slog.HandlerOptions{Level: level})))

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	var listeners []*netsrv.Listener
	for _, sc := range cfg.Servers {
		lb := lobby.New(sc.Name)
		addr := fmt.Sprintf(":%d", sc.Port)
		l, err := netsrv.Listen(ctx, addr, lb,
			netsrv.Options{MaxConnPerIP: sc.MaxConnPerIP})
		if err != nil {
			slog.Error("listen", "server", sc.Name, "port", sc.Port, "err", err)
			os.Exit(1)
		}
		listeners = append(listeners, l)
		slog.Info("lobby started", "server", sc.Name, "addr", addr)
	}

	<-ctx.Done()
	slog.Info("shutting down")
	for _, l := range listeners {
		l.Close()
	}
}
```

`deploy/config.example.toml`:

```toml
data_dir = "/var/lib/c3hub"

[[server]]
name = "Основний"
port = 31523
max_conn_per_ip = 4
description = "Головний сервер спільноти"

[[server]]
name = "Тестовий"
port = 31524
max_conn_per_ip = 4
description = "Для перевірок перед оновленням"
```

- [ ] **Step 4: Тести й ручний запуск**

Run: `cd server && go test ./... && go run ./cmd/c3hub -config ../deploy/config.example.toml -debug`
Expected: тести PASS; процес пише `lobby started` для двох портів і не завершується (зупинити Ctrl+C).

- [ ] **Step 5: Коміт**

```bash
git add server deploy/config.example.toml
git commit -m "feat(c3hub): config loading and multi-port startup"
```

---

### Task 11: Тести паритету з C++ сервером

**Files:**
- Create: `server/internal/paritytest/scenario.go`, `server/internal/paritytest/parity_test.go`, `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `protocol`, запущений сервер на TCP-адресі.
- Produces: `paritytest.Run(t *testing.T, addr string) map[string][]protocol.Packet` — проганяє сценарій і повертає отримані пакети по клієнтах.

Ідея: сценарій — послідовність дій трьох клієнтів із фіксованими паузами. Пакети, які отримав кожен клієнт, нормалізуються (обнуляються поля, що містять випадкові або часозалежні значення) і порівнюються між C++ і Go.

- [ ] **Step 1: Написати сценарій і раннер**

`server/internal/paritytest/scenario.go`:

```go
// Package paritytest проганяє однаковий сценарій проти двох реалізацій
// сервера й збирає отримані пакети для побайтового порівняння.
package paritytest

import (
	"net"
	"time"

	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

type client struct {
	name string
	conn net.Conn
	recv []protocol.Packet
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
			c.recv = append(c.recv, p)
		}
	}()
	return c, nil
}

func (c *client) send(p protocol.Packet) {
	c.conn.Write(protocol.Encode(p))
	time.Sleep(120 * time.Millisecond) // даємо серверу розіслати наслідки
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

	w := protocol.NewWriter()
	w.Int(8)
	w.Byte(0)
	w.String(`"parity"\t""\t008C7`, protocol.LenByte)
	w.String("0", protocol.LenByte)
	w.Int(42)
	w.Short(0)
	host.send(w.Packet(0x19c, 1, 0))

	for _, g := range []*client{g1, g2} {
		jw := protocol.NewWriter()
		jw.Int(1) // id хоста = 1
		g.send(jw.Packet(0x19e, 0, 0))
	}

	uw := protocol.NewWriter()
	uw.String(`"parity"\t""\t008C7`, protocol.LenByte)
	uw.String("1|3|0|5|0|0", protocol.LenByte)
	uw.Raw(make([]byte, 6))
	host.send(uw.Packet(0x1aa, 1, 0))

	cw := protocol.NewWriter()
	cw.String("0|hello", protocol.LenByte)
	g1.send(cw.Packet(0x194, 2, 0))

	host.send(protocol.Packet{Cmd: 0x1a2, ID1: 1})
	host.send(protocol.Packet{Cmd: 0x4b0, ID1: 1, Body: []byte{1, 2, 3, 4}})
	g1.send(protocol.Packet{Cmd: 0x460, ID1: 2})
	host.send(protocol.Packet{Cmd: 0x1a0, ID1: 1}) // хост виходить під час гри

	time.Sleep(300 * time.Millisecond)
	host.conn.Close()
	g1.conn.Close()
	time.Sleep(300 * time.Millisecond)
	g2.conn.Close()
	time.Sleep(200 * time.Millisecond)

	return map[string][]protocol.Packet{
		"host": host.recv, "guest1": g1.recv, "guest2": g2.recv,
	}, nil
}
```

- [ ] **Step 2: Тест порівняння**

`server/internal/paritytest/parity_test.go`:

```go
package paritytest

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/zeror5162-creator/cossacs/server/internal/lobby"
	"github.com/zeror5162-creator/cossacs/server/internal/netsrv"
	"github.com/zeror5162-creator/cossacs/server/internal/protocol"
)

// TestParityWithReferenceServer порівнює наш сервер з оригінальним C++.
// Адреса оригіналу береться зі змінної C3_REFERENCE_ADDR; без неї тест
// пропускається (локально оригіналу зазвичай нема).
func TestParityWithReferenceServer(t *testing.T) {
	refAddr := os.Getenv("C3_REFERENCE_ADDR")
	if refAddr == "" {
		t.Skip("C3_REFERENCE_ADDR is not set")
	}

	lb := lobby.New("parity")
	l, err := netsrv.Listen(context.Background(), "127.0.0.1:0", lb, netsrv.Options{})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	refPackets, err := Run(refAddr)
	if err != nil {
		t.Fatalf("reference run: %v", err)
	}
	gotPackets, err := Run(l.Addr())
	if err != nil {
		t.Fatalf("go run: %v", err)
	}

	for _, name := range []string{"host", "guest1", "guest2"} {
		want, got := refPackets[name], gotPackets[name]
		if len(want) != len(got) {
			t.Errorf("%s: got %d packets, want %d\n got: %s\nwant: %s",
				name, len(got), len(want), dump(got), dump(want))
			continue
		}
		for i := range want {
			w, g := want[i], got[i]
			if w.Cmd != g.Cmd || w.ID1 != g.ID1 || w.ID2 != g.ID2 {
				t.Errorf("%s packet %d: header %#x/%d/%d, want %#x/%d/%d",
					name, i, g.Cmd, g.ID1, g.ID2, w.Cmd, w.ID1, w.ID2)
				continue
			}
			if string(w.Body) != string(g.Body) {
				t.Errorf("%s packet %d (cmd %#x):\n got % x\nwant % x",
					name, i, w.Cmd, g.Body, w.Body)
			}
		}
	}
}

func dump(ps []protocol.Packet) string {
	out := ""
	for _, p := range ps {
		out += fmt.Sprintf("%#x ", p.Cmd)
	}
	return out
}
```

- [ ] **Step 3: Запустити локально без оригіналу**

Run: `cd server && go test ./internal/paritytest/ -v`
Expected: SKIP — `C3_REFERENCE_ADDR is not set`.

- [ ] **Step 4: CI з реальним порівнянням**

`.github/workflows/ci.yml`:

```yaml
name: CI

on:
  push:
  pull_request:

jobs:
  test:
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Unit tests
        working-directory: server
        run: go test ./... -count=1

      - name: Build reference C++ server
        run: |
          sudo apt-get update
          sudo apt-get install -y g++ libasio-dev
          git clone --depth 1 https://github.com/ereb-thanatos/cossacks3-lan-server.git /tmp/ref
          g++ /tmp/ref/src/*.cpp -DNDEBUG -DASIO_STANDALONE -O2 -pthread -o /tmp/ref-server

      - name: Run reference server
        run: |
          /tmp/ref-server &
          sleep 2

      - name: Parity test
        working-directory: server
        env:
          C3_REFERENCE_ADDR: 127.0.0.1:31523
        run: go test ./internal/paritytest/ -v -count=1
```

- [ ] **Step 5: Прогнати CI і виправити розбіжності**

```bash
git add server/internal/paritytest .github/workflows/ci.yml
git commit -m "test: parity harness against reference C++ server"
git push
```

Відкрити вкладку Actions. **Кожна розбіжність у виводі — баг у нашому порту, не в тесті.** Виправляти `handlers.go`, доки паритет не стане чистим. Типові місця: порядок обходу мап, зворотний порядок списків, зайвий або відсутній нульовий байт.

---

### Task 12: Розгортання на тестовому порту

**Files:**
- Create: `deploy/c3hub.service`, `deploy/deploy.sh`

**Interfaces:**
- Consumes: зібраний бінарник `c3hub`.
- Produces: працюючий `c3hub.service` на VPS, порт 31524.

Пам'ятати з `cossacks3-server.md`: хост спільний із Minecraft; `pkill -f cossacks` через SSH вбиває саму сесію; довгі команди запускати через `setsid nohup`; порт треба відкрити і в iptables, і в Security List OCI (друге робить користувач).

- [ ] **Step 1: systemd-юніт**

`deploy/c3hub.service`:

```ini
[Unit]
Description=c3hub - Cossacks 3 lobby server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=c3hub
Group=c3hub
WorkingDirectory=/opt/c3hub
ExecStart=/opt/c3hub/c3hub -config /etc/c3hub/config.toml
Restart=always
RestartSec=5
MemoryMax=256M
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/c3hub

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 2: Скрипт деплою**

`deploy/deploy.sh`:

```bash
#!/usr/bin/env bash
# Збирає c3hub під ARM64 і заливає на VPS. Запускати з кореня репозиторію.
set -euo pipefail

HOST="${1:-oracle}"

echo "==> build"
( cd server && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../c3hub ./cmd/c3hub )

echo "==> upload"
scp c3hub "$HOST:/tmp/c3hub"
scp deploy/c3hub.service "$HOST:/tmp/c3hub.service"

echo "==> install"
ssh "$HOST" 'sudo install -m 755 /tmp/c3hub /opt/c3hub/c3hub \
  && sudo install -m 644 /tmp/c3hub.service /etc/systemd/system/c3hub.service \
  && sudo systemctl daemon-reload \
  && sudo systemctl restart c3hub \
  && sleep 2 && systemctl is-active c3hub'

rm -f c3hub
echo "==> done"
```

- [ ] **Step 3: Підготувати сервер (одноразово)**

```bash
ssh oracle 'sudo useradd --system --no-create-home --shell /usr/sbin/nologin c3hub || true; \
  sudo mkdir -p /opt/c3hub /etc/c3hub /var/lib/c3hub; \
  sudo chown c3hub:c3hub /var/lib/c3hub'
```

Залити конфіг **лише з тестовим портом 31524** (порт 31523 поки за старим сервером):

```bash
scp deploy/config.example.toml oracle:/tmp/config.toml
ssh oracle 'sudo install -m 640 -o root -g c3hub /tmp/config.toml /etc/c3hub/config.toml'
```

Перед заливкою відредагувати файл так, щоб у ньому лишився один `[[server]]` з `port = 31524`.

- [ ] **Step 4: Відкрити порт**

iptables (перед фінальним `REJECT`; номер правила подивитися через `sudo iptables -L INPUT --line-numbers`):

```bash
ssh oracle 'sudo iptables -I INPUT 6 -p tcp --dport 31524 -m state --state NEW -j ACCEPT && sudo netfilter-persistent save'
```

**Попросити користувача** додати ingress-правило в Security List OCI: TCP, порт 31524, джерело `0.0.0.0/0`. Без цього порт ззовні закритий.

- [ ] **Step 5: Розгорнути й перевірити**

```bash
./deploy/deploy.sh oracle
```

Перевірка з Windows:

```powershell
Test-NetConnection 158.180.57.78 -Port 31524
```

Перевірка логів:

```bash
ssh oracle 'sudo journalctl -u c3hub -n 30 --no-pager; systemctl is-active cossacks3'
```

Expected: `c3hub` активний, у логах `lobby started`; старий `cossacks3` теж активний (гравці на 31523 не постраждали).

- [ ] **Step 6: Прийомний тест із реальними гравцями**

1. У `data\resources\servers.dat` на тестовому ПК вписати один рядок `* = 158.180.57.78:31524`.
2. 2–4 гравці заходять, створюють кімнату, грають партію до кінця, пробують чат, кік і вихід хоста під час гри.
3. Паралельно дивитися `ssh oracle 'sudo journalctl -u c3hub -f'`.

Expected: жодних `panic`, `send queue overflow` чи розривів; партія доходить до кінця.

- [ ] **Step 7: Коміт**

```bash
git add deploy
git commit -m "chore(deploy): systemd unit and deploy script"
git push
```

**Перемикання порту 31523 на `c3hub` робиться тільки після успішного прийомного тесту і є першим кроком етапу 2.**
