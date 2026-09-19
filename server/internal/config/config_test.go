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
