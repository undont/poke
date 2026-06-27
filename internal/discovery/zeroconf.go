package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/grandcat/zeroconf"
)

// zcAdvertiser registers this node over mDNS.
type zcAdvertiser struct {
	server *zeroconf.Server
}

// NewAdvertiser returns an mDNS-backed Advertiser.
func NewAdvertiser() Advertiser { return &zcAdvertiser{} }

func (a *zcAdvertiser) Advertise(ctx context.Context, user string, isRelay bool, port int) error {
	host, _ := os.Hostname()
	// give the relay a distinct instance so one host can run both a relay and a
	// daemon for the same user without an mDNS instance-name collision.
	instance := fmt.Sprintf("%s@%s", user, host)
	txt := []string{"user=" + user}
	if isRelay {
		instance += "-relay"
		txt = append(txt, "relay=1")
	}
	server, err := zeroconf.Register(instance, Service, Domain, port, txt, nil)
	if err != nil {
		return err
	}
	a.server = server
	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()
	return nil
}

func (a *zcAdvertiser) Close() error {
	if a.server != nil {
		a.server.Shutdown()
	}
	return nil
}

// zcBrowser browses for relays over mDNS.
type zcBrowser struct{}

// NewBrowser returns an mDNS-backed Browser.
func NewBrowser() Browser { return &zcBrowser{} }

func (b *zcBrowser) FindRelay(ctx context.Context) (Relay, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return Relay{}, err
	}
	entries := make(chan *zeroconf.ServiceEntry, 8)
	if err := resolver.Browse(ctx, Service, Domain, entries); err != nil {
		return Relay{}, err
	}
	for {
		select {
		case e, ok := <-entries:
			if !ok {
				return Relay{}, ctx.Err()
			}
			if r, ok := relayFrom(e); ok {
				return r, nil
			}
		case <-ctx.Done():
			return Relay{}, ctx.Err()
		}
	}
}

// FindPeer resolves the named daemon (matching its user= record and not a
// relay) so a poke can be delivered directly when no relay is up.
func (b *zcBrowser) FindPeer(ctx context.Context, user string) (Peer, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return Peer{}, err
	}
	entries := make(chan *zeroconf.ServiceEntry, 8)
	if err := resolver.Browse(ctx, Service, Domain, entries); err != nil {
		return Peer{}, err
	}
	for {
		select {
		case e, ok := <-entries:
			if !ok {
				return Peer{}, ctx.Err()
			}
			if p, ok := peerFrom(e, user); ok {
				return p, nil
			}
		case <-ctx.Done():
			return Peer{}, ctx.Err()
		}
	}
}

func (b *zcBrowser) Close() error { return nil }

// peerFrom extracts a dialable daemon for the wanted user, skipping relays.
func peerFrom(e *zeroconf.ServiceEntry, want string) (Peer, bool) {
	if hasFlag(e.Text, "relay=1") || txtUser(e.Text) != want {
		return Peer{}, false
	}
	ip := bestIPv4(e.AddrIPv4)
	if ip == "" || e.Port == 0 {
		return Peer{}, false
	}
	return Peer{
		User: want,
		Host: e.HostName,
		Addr: net.JoinHostPort(ip, fmt.Sprint(e.Port)),
	}, true
}

// txtUser returns the user= value from a TXT record set, or "".
func txtUser(txt []string) string {
	for _, t := range txt {
		if v, ok := strings.CutPrefix(strings.TrimSpace(t), "user="); ok {
			return v
		}
	}
	return ""
}

// relayFrom extracts a dialable relay from an entry that advertises relay=1.
func relayFrom(e *zeroconf.ServiceEntry) (Relay, bool) {
	if !hasFlag(e.Text, "relay=1") {
		return Relay{}, false
	}
	ip := bestIPv4(e.AddrIPv4)
	if ip == "" || e.Port == 0 {
		return Relay{}, false
	}
	return Relay{
		Host: e.HostName,
		Addr: net.JoinHostPort(ip, fmt.Sprint(e.Port)),
	}, true
}

func hasFlag(txt []string, flag string) bool {
	for _, t := range txt {
		if strings.EqualFold(strings.TrimSpace(t), flag) {
			return true
		}
	}
	return false
}

// bestIPv4 picks a dialable IPv4 from an advertised set. an mDNS advert carries
// every interface address, so a host on tailscale publishes its 100.64.0.0/10
// CGNAT tailnet address alongside its LAN one; that address is unreachable from
// a peer on a different tailnet, so it is only used when nothing else is offered.
func bestIPv4(ips []net.IP) string {
	var fallback string
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		if isCGNAT(v4) {
			if fallback == "" {
				fallback = v4.String()
			}
			continue
		}
		return v4.String()
	}
	return fallback
}

// isCGNAT reports whether v4 falls in the 100.64.0.0/10 shared address space.
func isCGNAT(v4 net.IP) bool {
	return v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}
