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
