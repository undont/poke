package discovery

import (
	"context"
	"net"
	"testing"
)

func TestFixedLocator(t *testing.T) {
	const addr = "relay.poke.ts.net:7373"
	r, err := NewFixedLocator(addr).Locate(context.Background())
	if err != nil {
		t.Fatalf("fixed locator should never error: %v", err)
	}
	if r.Addr != addr {
		t.Fatalf("addr = %q, want %q", r.Addr, addr)
	}
}

func TestBestIPv4(t *testing.T) {
	ip := func(s string) net.IP { return net.ParseIP(s) }
	cases := []struct {
		name string
		ips  []net.IP
		want string
	}{
		{"lan over tailnet", []net.IP{ip("100.66.4.57"), ip("192.168.0.220")}, "192.168.0.220"},
		{"tailnet only is last resort", []net.IP{ip("100.66.4.57")}, "100.66.4.57"},
		{"skips ipv6", []net.IP{ip("fe80::1"), ip("10.0.0.4")}, "10.0.0.4"},
		{"empty", nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bestIPv4(c.ips); got != c.want {
				t.Fatalf("bestIPv4 = %q, want %q", got, c.want)
			}
		})
	}
}
