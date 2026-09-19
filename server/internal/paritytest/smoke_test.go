package paritytest

import (
	"os"
	"testing"
)

// TestSmokeAgainstDeployed ганяє повний сценарій проти вказаного сервера
// й перевіряє, що ключові сповіщення дійшли. Потрібна C3_SMOKE_ADDR.
func TestSmokeAgainstDeployed(t *testing.T) {
	addr := os.Getenv("C3_SMOKE_ADDR")
	if addr == "" {
		t.Skip("C3_SMOKE_ADDR is not set")
	}
	got, err := Run(addr)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for name, want := range map[string][]uint16{
		"host":   {0x19b, 0x19d, 0x1a5, 0x1a3},
		"guest1": {0x19b, 0x19f, 0x195, 0x1be},
		"guest2": {0x19b, 0x1a3, 0x1bd, 0x1a7},
	} {
		for _, cmd := range want {
			found := false
			for _, p := range got[name] {
				if p.Cmd == cmd {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s did not receive %#x", name, cmd)
			}
		}
	}
}
