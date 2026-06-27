package peersfile

import (
	"path/filepath"
	"testing"

	"github.com/undont/poke/internal/protocol"
)

func TestRoundTripEncodesNote(t *testing.T) {
	w := New(filepath.Join(t.TempDir(), "peers"))
	if err := w.Append(Entry{From: "alice", Strength: protocol.High, TS: 1, ID: "x", Note: "a:b%c\nd"}); err != nil {
		t.Fatal(err)
	}
	got, err := Read(w.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Note != "a:b%c\nd" {
		t.Fatalf("note did not survive round trip: %+v", got)
	}
}
