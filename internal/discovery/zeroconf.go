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
	instance := fmt.Sprintf("%s@%s", user, host)
	txt := []string{"user=" + user}
	if isRelay {
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

func (b *zcBrowser) Close() error { return nil }

// relayFrom extracts a dialable relay from an entry that advertises relay=1.
func relayFrom(e *zeroconf.ServiceEntry) (Relay, bool) {
	if !hasFlag(e.Text, "relay=1") {
		return Relay{}, false
	}
	ip := firstIPv4(e.AddrIPv4)
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

func firstIPv4(ips []net.IP) string {
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String()
		}
	}
	return ""
}
