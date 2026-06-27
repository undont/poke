package main

import (
	"testing"

	"github.com/undont/poke/internal/protocol"
)

func TestParsePoke(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		to       string
		strength protocol.Strength
		note     string
	}{
		{"default medium", []string{"alice"}, "alice", protocol.Medium, ""},
		{"note only", []string{"alice", "prod is down"}, "alice", protocol.Medium, "prod is down"},
		{"flag after note", []string{"alice", "prod is down", "--high"}, "alice", protocol.High, "prod is down"},
		{"flag before note", []string{"alice", "--high", "prod is down"}, "alice", protocol.High, "prod is down"},
		{"flag first", []string{"--high", "alice", "prod is down"}, "alice", protocol.High, "prod is down"},
		{"level word in note is not urgency", []string{"alice", "high", "five"}, "alice", protocol.Medium, "high five"},
		{"low flag", []string{"bob", "--low"}, "bob", protocol.Low, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := parsePoke(c.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if req.To != c.to || req.Strength != c.strength || req.Note != c.note {
				t.Fatalf("got to=%q strength=%q note=%q", req.To, req.Strength, req.Note)
			}
		})
	}
}

func TestParsePokeErrors(t *testing.T) {
	for _, args := range [][]string{
		{"alice", "--low", "--high"}, // conflicting urgency
		{"alice", "--bogus"},         // unknown flag
		{"--high"},                   // no target
	} {
		if _, err := parsePoke(args); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}
