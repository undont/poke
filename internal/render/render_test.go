package render

import (
	"strings"
	"testing"

	"github.com/undont/poke/internal/peersfile"
	"github.com/undont/poke/internal/protocol"
)

func entry(from string, s protocol.Strength) peersfile.Entry {
	return peersfile.Entry{From: from, Strength: s}
}

func TestSegmentEmpty(t *testing.T) {
	if got := Segment(nil, Options{}); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

func TestColourStablePerUser(t *testing.T) {
	got := colour("sean")
	if !strings.HasPrefix(got, "colour") {
		t.Fatalf("want a tmux colour token, got %q", got)
	}
	if again := colour("sean"); got != again {
		t.Fatalf("same user must hash to the same colour: %q vs %q", got, again)
	}
}

func TestSegmentEmphasis(t *testing.T) {
	seg := Segment([]peersfile.Entry{entry("alice", protocol.High)}, Options{Icon: "B"})
	if !strings.Contains(seg, "#[bold]") {
		t.Fatalf("high urgency should be bold: %q", seg)
	}
	if !strings.Contains(seg, "alice") {
		t.Fatalf("name missing: %q", seg)
	}
}

func TestSegmentOverflow(t *testing.T) {
	es := []peersfile.Entry{
		entry("ann", protocol.Low), entry("bob", protocol.Low),
		entry("cal", protocol.Low), entry("dave", protocol.Low), entry("erin", protocol.Low),
	}
	seg := Segment(es, Options{Icon: "B", Max: 3})
	if !strings.Contains(seg, "+2") {
		t.Fatalf("want +2 overflow: %q", seg)
	}
	if strings.Contains(seg, "dave") || strings.Contains(seg, "erin") {
		t.Fatalf("overflowed names should be hidden: %q", seg)
	}
}
