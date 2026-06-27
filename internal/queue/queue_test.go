package queue

import (
	"testing"
	"time"

	"github.com/undont/poke/internal/protocol"
)

func open(t *testing.T) *Queue {
	t.Helper()
	q, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { q.Close() })
	return q
}

func poke(id string, ts int64) protocol.Poked {
	return protocol.Poked{Type: protocol.TypePoked, ID: id, From: "alice", Strength: protocol.Medium, TS: ts}
}

func TestDrainPreservesOrderAndClears(t *testing.T) {
	q := open(t)
	now := time.Unix(1000, 0)
	for _, id := range []string{"a", "b", "c"} {
		if err := q.Enqueue("bob", poke(id, now.Unix())); err != nil {
			t.Fatal(err)
		}
	}
	got, err := q.Drain("bob", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "a" || got[2].ID != "c" {
		t.Fatalf("want a,b,c in order, got %+v", got)
	}
	// a second drain is empty: the bucket was cleared
	again, _ := q.Drain("bob", time.Hour, now)
	if len(again) != 0 {
		t.Fatalf("drain should clear the queue, got %d", len(again))
	}
}

func TestDrainDropsExpired(t *testing.T) {
	q := open(t)
	now := time.Unix(100000, 0)
	q.Enqueue("bob", poke("old", now.Add(-48*time.Hour).Unix()))
	q.Enqueue("bob", poke("fresh", now.Add(-1*time.Hour).Unix()))

	got, err := q.Drain("bob", 24*time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "fresh" {
		t.Fatalf("want only fresh, got %+v", got)
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	q1, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	q1.Enqueue("bob", poke("a", time.Unix(1000, 0).Unix()))
	q1.Close()

	q2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer q2.Close()
	got, _ := q2.Drain("bob", time.Hour, time.Unix(1000, 0))
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("queue should survive reopen, got %+v", got)
	}
}
