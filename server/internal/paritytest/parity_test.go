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
