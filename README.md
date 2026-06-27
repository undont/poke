# poke

a terminal-native "tap on the shoulder" for a small dev team. one dev pokes
another and the target's tmux status bar lights up, optionally with a bell or a
desktop notification, scaled by an urgency level. no slack, no browser.

three artifacts from one go binary set:

- **`poke`** — the cli. ephemeral: `connect`, `<user>`, `clear`, `who`, `dnd`,
  `render`. talks only to the local daemon over a unix socket (`render` reads
  the peers file directly).
- **`poked`** — the per-machine daemon. holds the relay connection, tracks
  presence, and on an incoming poke writes the tmux alert surface and rings the
  bell.
- **`poked --relay`** — the same binary in relay mode. one always-on box on the
  lan routes pokes and holds the offline queue.

## Status

phases 0, 1, and 3 land. a poke travels across machines end to end: the daemon
finds the relay over mDNS, dials it over a websocket, and on delivery writes
the peers file and rings the bell. `poke render` paints a status-right segment
(icon, stable per-user colour, urgency emphasis, `+N` overflow); see
`examples/tmux.conf` for the status line and clear keybinding. a poke to an
offline target is queued durably on the relay (bbolt) and drained in order when
they reconnect, dropping anything past `POKE_QUEUE_TTL` (default 24h).

not yet built: urgency persistence (high persisting until cleared), `seen`
acks, presence-driven `who`, and the live-only fallback when no relay is up.

## Build

```sh
go build ./...
go build -o poke ./cmd/poke
go build -o poked ./cmd/poked
```

## Running

every machine shares one secret, set as `POKE_SECRET`. on one always-on box run
the relay (`poked --relay`); everywhere else run the daemon.

```sh
export POKE_SECRET=your-team-secret   # same on every machine

# the always-on box
poked --relay

# each dev machine: start the daemon (poke connect starts it if it is not up)
poke connect
poke alice high "prod is down"
```

for the daemon to be up before you think to run `poke connect`, install the
login item for your platform: `examples/launchd/com.poke.poked.plist` (macOS) or
`examples/systemd/poked.service` (linux). both carry the secret privately rather
than on the command line.
