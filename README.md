# poke

a terminal-native "tap on the shoulder" for a small dev team. one dev pokes
another and the target's tmux status bar lights up, optionally with a bell or a
desktop notification, scaled by an urgency level. no slack, no browser.

three artifacts from one go binary set:

- **`poke`** — the cli. ephemeral: `connect`, `<user>`, `clear`, `who`, `dnd`.
  talks only to the local daemon over a unix socket.
- **`poked`** — the per-machine daemon. holds the relay connection, tracks
  presence, and on an incoming poke writes the tmux alert surface and rings the
  bell.
- **`poked --relay`** — the same binary in relay mode. one always-on box on the
  lan routes pokes and holds the offline queue.

## Status

skeleton. `go build ./...` compiles; the cli <-> daemon IPC round-trips and the
peers-file/bell/notify surfaces are real. the network half (mDNS discovery, the
relay connection, presence, the offline queue) is stubbed behind the
`discovery` and `transport` interfaces.

## Build

```sh
go build ./...
go build -o poke ./cmd/poke
go build -o poked ./cmd/poked
```
