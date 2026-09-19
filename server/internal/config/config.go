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
