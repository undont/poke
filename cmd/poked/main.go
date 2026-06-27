// command poked is the persistent process. without flags it runs the
// per-machine daemon; with --relay it runs the same binary in relay mode.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/daemon"
	"github.com/undont/poke/internal/relay"
	"github.com/undont/poke/internal/version"
)

func main() {
	var (
		asRelay     = flag.Bool("relay", false, "run in relay mode")
		showVersion = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("poked %s (protocol %d)\n", version.Version, version.Protocol)
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *asRelay {
		if err := relay.New(cfg, log).Run(ctx); err != nil && ctx.Err() == nil {
			log.Error("relay", "err", err)
			os.Exit(1)
		}
		return
	}

	if err := daemon.New(cfg, log).Run(ctx); err != nil && ctx.Err() == nil {
		log.Error("daemon", "err", err)
		os.Exit(1)
	}
}
