// command poke is the ephemeral CLI: parse a verb, round-trip the local
// daemon over the unix socket, print the reply, exit.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/ipc"
	"github.com/undont/poke/internal/peersfile"
	"github.com/undont/poke/internal/protocol"
	"github.com/undont/poke/internal/render"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "poke:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// these touch local config or files directly, no daemon round-trip.
	switch args[0] {
	case "render":
		// status-right runs on every refresh; must not depend on the socket.
		return renderSegment(cfg)
	case "name":
		return setName(cfg, args[1:])
	case "secret":
		return setSecret(cfg, args[1:])
	}

	req, err := parse(args)
	if err != nil {
		return err
	}

	// connect is the one verb that may need to start the daemon first.
	if req.Verb == protocol.IPCConnect {
		if err := ensureDaemon(cfg); err != nil {
			return err
		}
	}

	resp, err := ipc.Send(cfg.SocketPath, req)
	if err != nil {
		return fmt.Errorf("daemon unreachable (%w); is `poked` running?", err)
	}
	if !resp.OK {
		return fmt.Errorf("%s", resp.Error)
	}
	printResp(req, resp)
	return nil
}

// renderSegment prints the tmux status-right segment for the live pokes.
func renderSegment(cfg *config.Config) error {
	entries, err := peersfile.Read(cfg.PeersFile)
	if err != nil {
		return err
	}
	fmt.Print(render.Segment(entries, render.Options{Icon: cfg.Icon}))
	return nil
}

// parse maps argv to an IPC request. the bare `poke <user> ...` form is a poke.
func parse(args []string) (protocol.IPCRequest, error) {
	switch args[0] {
	case "connect":
		return protocol.IPCRequest{Verb: protocol.IPCConnect}, nil
	case "clear":
		return protocol.IPCRequest{Verb: protocol.IPCClear}, nil
	case "who":
		return protocol.IPCRequest{Verb: protocol.IPCWho}, nil
	case "dnd":
		return parseDND(args[1:])
	case "help", "-h", "--help":
		usage()
		os.Exit(0)
	}
	return parsePoke(args)
}

// parsePoke reads `<user> [note...]` with an urgency flag (--low/--medium/
// --high) that may appear anywhere. urgency is a flag rather than a positional
// so a note starting with a level word is never mistaken for the urgency.
func parsePoke(args []string) (protocol.IPCRequest, error) {
	req := protocol.IPCRequest{Verb: protocol.IPCPoke, Strength: protocol.Medium}
	var words []string
	strengthSet := false
	for _, a := range args {
		switch a {
		case "--low", "--medium", "--high":
			s := protocol.Strength(strings.TrimPrefix(a, "--"))
			if strengthSet && req.Strength != s {
				return req, fmt.Errorf("conflicting urgency flags")
			}
			req.Strength, strengthSet = s, true
		default:
			if strings.HasPrefix(a, "--") {
				return req, fmt.Errorf("unknown flag %q", a)
			}
			words = append(words, a)
		}
	}
	if len(words) == 0 {
		return req, fmt.Errorf("usage: poke <user> [note] [--low|--medium|--high]")
	}
	req.To = words[0]
	if len(words) > 1 {
		req.Note = strings.Join(words[1:], " ")
	}
	return req, nil
}

func parseDND(args []string) (protocol.IPCRequest, error) {
	req := protocol.IPCRequest{Verb: protocol.IPCDND}
	if len(args) > 0 {
		switch args[0] {
		case "on":
			v := true
			req.DND = &v
		case "off":
			v := false
			req.DND = &v
		default:
			return req, fmt.Errorf("dnd takes on|off, got %q", args[0])
		}
	}
	return req, nil
}

func printResp(req protocol.IPCRequest, resp protocol.IPCResponse) {
	switch req.Verb {
	case protocol.IPCPoke:
		fmt.Println(resp.Mode)
	case protocol.IPCWho:
		if len(resp.Roster) == 0 {
			fmt.Fprintln(os.Stderr, "no peers online")
			return
		}
		for _, e := range resp.Roster {
			fmt.Printf("%s\t%s\n", e.User, e.Host)
		}
	case protocol.IPCDND:
		state := "off"
		if resp.DND != nil && *resp.DND {
			state = "on"
		}
		fmt.Println("dnd", state)
	default:
		if resp.Message != "" {
			fmt.Println(resp.Message)
		}
	}
}

func usage() {
	fmt.Print(`poke - tap a teammate on the shoulder, in the terminal

usage:
  poke connect              ensure the daemon is up, announce presence
  poke <user> [note] [--low|--medium|--high]    urgency may go anywhere, default medium
  poke clear                dismiss incoming pokes
  poke who                  show the live roster
  poke dnd [on|off]         toggle do-not-disturb
  poke name [<name>]        show or set your display name
  poke secret               set the shared team secret (prompts, or reads stdin)
  poke render               print the tmux status segment (for status-right)
`)
}
