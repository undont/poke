// package discovery isolates how a daemon learns the relay address and how a
// relay announces itself. mDNS backs it on a LAN today; a fixed address slots
// in behind the same interface later.
package discovery

import "context"

// the mDNS service the project advertises and browses for.
const (
	Service = "_poke._tcp"
	Domain  = "local."
)

// Relay is a discovered relay endpoint.
type Relay struct {
	Host string
	Addr string // host:port the daemon dials
}

// Browser finds relays on the network.
type Browser interface {
	// FindRelay blocks until a relay is discovered or ctx is done.
	FindRelay(ctx context.Context) (Relay, error)
	Close() error
}

// Advertiser publishes this node's presence on the network.
type Advertiser interface {
	// Advertise announces the service on port until Close or ctx is done.
	Advertise(ctx context.Context, user string, isRelay bool, port int) error
	Close() error
}
