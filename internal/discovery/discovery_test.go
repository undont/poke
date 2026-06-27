package discovery

import (
	"context"
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
