// package discovery isolates how a daemon learns the relay address and how it
// learns the live roster. mDNS backs it on a LAN today; a fixed address slots
// in behind the same interface later. the stub here keeps the tree compiling
// until grandcat/zeroconf is wired in.
package discovery

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the placeholder backend.
var ErrNotImplemented = errors.New("discovery: not implemented")

// Relay is a discovered relay endpoint.
type Relay struct {
	Host string
	Addr string // host:port the daemon dials
}

// Browser finds relays and tracks peer presence on the network.
type Browser interface {
	// FindRelay blocks until a relay is discovered or ctx is done.
	FindRelay(ctx context.Context) (Relay, error)
	// Close stops browsing.
	Close() error
}

// Advertiser publishes this node's presence on the network.
type Advertiser interface {
	// Advertise announces the service until ctx is done.
	Advertise(ctx context.Context, user string, isRelay bool) error
	// Close withdraws the advertisement.
	Close() error
}

// stub is the no-op backend used before mDNS is wired in.
type stub struct{}

// NewStub returns a placeholder that implements both roles and does nothing.
func NewStub() *stub { return &stub{} }

func (s *stub) FindRelay(ctx context.Context) (Relay, error) {
	return Relay{}, ErrNotImplemented
}

func (s *stub) Advertise(ctx context.Context, user string, isRelay bool) error {
	return ErrNotImplemented
}

func (s *stub) Close() error { return nil }
