// command poke is the ephemeral CLI: parse a verb, round-trip the local
// daemon over the unix socket, print the reply, exit.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/ipc"
	"github.com/undont/poke/internal/protocol"
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

	req, err := parse(args)
	if err != nil {
		return err
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

// parsePoke reads `<user> [strength] [note...]`.
func parsePoke(args []string) (protocol.IPCRequest, error) {
	req := protocol.IPCRequest{Verb: protocol.IPCPoke, To: args[0], Strength: protocol.Medium}
	rest := args[1:]
	if len(rest) > 0 && protocol.ValidStrength(protocol.Strength(rest[0])) {
		req.Strength = protocol.Strength(rest[0])
		rest = rest[1:]
	}
	if len(rest) > 0 {
		req.Note = strings.Join(rest, " ")
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
  poke <user> [low|medium|high] [note]
  poke clear                dismiss incoming pokes
  poke who                  show the live roster
  poke dnd [on|off]         toggle do-not-disturb
`)
}
