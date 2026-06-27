// package relay is the same binary in relay mode: one always-on box that
// authenticates daemons, holds the roster, and routes (or later queues) pokes.
//
// the accept loop and routing are stubbed pending a real transport.
package relay

import (
	"context"
	"log/slog"
	"sync"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/protocol"
)

// Relay is the running relay process.
type Relay struct {
	cfg *config.Config
	log *slog.Logger

	mu     sync.Mutex
	roster map[string]protocol.RosterEntry // user -> entry
}

// New constructs a Relay from resolved config.
func New(cfg *config.Config, log *slog.Logger) *Relay {
	return &Relay{
		cfg:    cfg,
		log:    log,
		roster: make(map[string]protocol.RosterEntry),
	}
}

// Run advertises the relay and serves daemons until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	r.log.Info("relay starting", "user", r.cfg.User, "host", r.cfg.Host)

	// TODO: advertise relay=1 over mDNS, accept daemon connections, validate
	// HELLO against the shared secret, then route POKE frames to a connected
	// target or enqueue for an offline one.

	<-ctx.Done()
	return ctx.Err()
}

// route would forward a poke to its target or enqueue it. defined now so the
// routing surface is explicit.
func (r *Relay) route(p protocol.Poke) error {
	r.mu.Lock()
	_, online := r.roster[p.To]
	r.mu.Unlock()
	if online {
		// TODO: forward as a protocol.Poked frame to the target connection
		return nil
	}
	// TODO: enqueue for offline delivery once the queue lands
	return nil
}
