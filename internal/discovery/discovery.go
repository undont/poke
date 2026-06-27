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

// Peer is a discovered daemon endpoint, used for live-only direct delivery.
type Peer struct {
	User string
	Host string
	Addr string // host:port the daemon dials
}

// Browser finds relays and peers on the network.
type Browser interface {
	// FindRelay blocks until a relay is discovered or ctx is done.
	FindRelay(ctx context.Context) (Relay, error)
	// FindPeer blocks until the named daemon is discovered or ctx is done.
	FindPeer(ctx context.Context, user string) (Peer, error)
	Close() error
}

// RelayLocator yields a dialable relay endpoint. it is the discovery tier seam:
// mDNS browses the LAN, a fixed locator returns a configured address and skips
// discovery entirely. the tailnet tier slots in here as a third implementation.
type RelayLocator interface {
	// Locate blocks until a relay endpoint is known or ctx is done.
	Locate(ctx context.Context) (Relay, error)
}

// NewMDNSLocator locates the relay by browsing mDNS on the LAN.
func NewMDNSLocator(b Browser) RelayLocator { return mdnsLocator{b} }

// NewFixedLocator returns a locator that always yields addr without any
// discovery, used when relay_addr is configured.
func NewFixedLocator(addr string) RelayLocator { return fixedLocator{addr} }

type mdnsLocator struct{ b Browser }

func (m mdnsLocator) Locate(ctx context.Context) (Relay, error) { return m.b.FindRelay(ctx) }

type fixedLocator struct{ addr string }

func (f fixedLocator) Locate(context.Context) (Relay, error) {
	return Relay{Host: f.addr, Addr: f.addr}, nil
}

// Advertiser publishes this node's presence on the network.
type Advertiser interface {
	// Advertise announces the service on port until Close or ctx is done.
	Advertise(ctx context.Context, user string, isRelay bool, port int) error
	Close() error
}
