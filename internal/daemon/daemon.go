// package daemon is the per-machine persistent process. it holds the relay
// connection, applies the urgency behaviour on an incoming poke (peers file,
// bell, notify), and exposes the unix socket for the CLI.
//
// the network half (discovery, relay connection, presence) is stubbed: this
// skeleton serves the CLI locally and records pokes, but does not yet route
// across machines.
package daemon

import (
	"context"
	"log/slog"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/ipc"
	"github.com/undont/poke/internal/peersfile"
	"github.com/undont/poke/internal/protocol"
	"github.com/undont/poke/internal/tmux"
)

// Daemon is the running per-machine process.
type Daemon struct {
	cfg   *config.Config
	log   *slog.Logger
	peers *peersfile.Writer
	srv   *ipc.Server

	dnd bool
}

// New constructs a Daemon from resolved config.
func New(cfg *config.Config, log *slog.Logger) *Daemon {
	return &Daemon{
		cfg:   cfg,
		log:   log,
		peers: peersfile.New(cfg.PeersFile),
	}
}

// Run binds the unix socket and serves the CLI until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	srv, err := ipc.Listen(d.cfg.SocketPath)
	if err != nil {
		return err
	}
	d.srv = srv
	d.log.Info("daemon listening", "socket", d.cfg.SocketPath, "user", d.cfg.User)

	// TODO: discover the relay, dial it, and pump frames; on a `poked` frame
	// call deliver(). until then the daemon is local-only.

	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return srv.Serve(d.handle)
}

// handle answers one CLI request.
func (d *Daemon) handle(req protocol.IPCRequest) protocol.IPCResponse {
	switch req.Verb {
	case protocol.IPCConnect:
		return protocol.IPCResponse{OK: true, Message: "connected (local-only)"}
	case protocol.IPCPoke:
		return d.handlePoke(req)
	case protocol.IPCClear:
		if err := d.peers.Clear(); err != nil {
			return protocol.IPCResponse{OK: false, Error: err.Error()}
		}
		return protocol.IPCResponse{OK: true, Message: "cleared"}
	case protocol.IPCWho:
		return protocol.IPCResponse{OK: true, Roster: nil}
	case protocol.IPCDND:
		if req.DND != nil {
			d.dnd = *req.DND
		}
		v := d.dnd
		return protocol.IPCResponse{OK: true, DND: &v}
	default:
		return protocol.IPCResponse{OK: false, Error: "unknown verb: " + req.Verb}
	}
}

// handlePoke would route to the relay; for now it reports the missing link.
func (d *Daemon) handlePoke(req protocol.IPCRequest) protocol.IPCResponse {
	if req.To == "" {
		return protocol.IPCResponse{OK: false, Error: "no target"}
	}
	s := req.Strength
	if s == "" {
		s = protocol.Medium
	}
	if !protocol.ValidStrength(s) {
		return protocol.IPCResponse{OK: false, Error: "bad strength: " + string(s)}
	}
	// TODO: send a protocol.Poke over the relay connection and await the mode.
	return protocol.IPCResponse{OK: false, Error: "no relay connection yet"}
}

// deliver applies the urgency behaviour for an incoming poke. unused until the
// relay connection lands; kept here so the surface is defined.
func (d *Daemon) deliver(p protocol.Poked) {
	e := peersfile.Entry{
		From:     p.From,
		Strength: p.Strength,
		TS:       p.TS,
		ID:       p.ID,
		Note:     p.Note,
	}
	if err := d.peers.Append(e); err != nil {
		d.log.Error("peers append failed", "err", err)
	}
	if d.dnd {
		return
	}
	switch p.Strength {
	case protocol.High:
		_ = tmux.Bell()
		_ = tmux.Notify("poke from "+p.From, p.Note)
	case protocol.Medium:
		_ = tmux.Bell()
	}
}
