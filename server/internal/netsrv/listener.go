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

	mu    sync.Mutex
	conns map[*session]struct{}
	perIP map[string]int
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
