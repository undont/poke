# poke

a terminal-native "tap on the shoulder" for a small dev team. one dev pokes
another and the target's tmux status bar lights up, optionally with a bell or a
desktop notification, scaled by an urgency level. no slack, no browser.

three artifacts from one go binary set:

- **`poke`** — the cli. ephemeral: `connect`, `<user>`, `clear`, `who`, `dnd`,
  `name`, `secret`, `render`. talks only to the local daemon over a unix socket
  (`name`/`secret`/`render` act on local config and files directly).
- **`poked`** — the per-machine daemon. holds the relay connection, tracks
  presence, and on an incoming poke writes the tmux alert surface and rings the
  bell.
- **`poked --relay`** — the same binary in relay mode. one always-on box on the
  lan routes pokes and holds the offline queue.

## How it works

a poke travels across machines end to end: the daemon
finds the relay over mDNS, dials it over a websocket, and on delivery writes
the peers file and rings the bell. `poke render` paints a status-right segment
(icon, stable per-user colour, urgency emphasis, `+N` overflow); see
`examples/tmux.conf` for the status line and clear keybinding. a poke to an
offline target is queued durably on the relay (bbolt) and drained in order when
they reconnect, dropping anything past `POKE_QUEUE_TTL` (default 24h). `poke
who` shows the live roster, kept current as peers join and leave. every poke
persists until you clear it; urgency drives only how loud the arrival is.

when no relay is advertising, the daemon falls back to live-only delivery: it
resolves the target's daemon over mDNS and pokes it directly. this lands only
while both peers are online (no durable queue without a relay), and the cli
reports the mode it used (`delivered` / `queued` / `live-only`) so the sender is
never misled about durability.

clearing a poke acknowledges it: the recipient's daemon sends a seen ack back
to the sender (via the relay), who gets a "saw your poke" notification. the
sender's poke already reported `delivered`; seen arrives later, when they look.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/undont/poke/main/install.sh | bash
```

this downloads the `poke` and `poked` binaries for your machine from the latest
release into `~/.local/bin` (override with `POKE_INSTALL_DIR`). if no release
exists yet and go is installed, it builds from source instead. equivalent
manual routes:

```sh
go install github.com/undont/poke/cmd/poke@latest
go install github.com/undont/poke/cmd/poked@latest
# or, from a checkout:
make install
```

## Build

```sh
make build          # bin/poke and bin/poked, version stamped from git
make dist           # cross-compiled release binaries into dist/
make help           # list every target
```

## Running

every machine shares one secret. set it (and your display name) from the cli;
both persist to `$XDG_CONFIG_HOME/poke/config` (mode 0600). `poke secret` prompts
on a terminal or reads stdin, like `gh secret set`, and never echoes the value.

```sh
poke secret                       # prompts; or: echo "$SECRET" | poke secret
poke name sean                    # your display name (defaults to $USER)

poke connect                      # starts the daemon if it is not up
poke alice "prod is down" --high   # urgency flag may go anywhere; default medium
```

a relay is optional: with no relay on the network the daemon delivers directly
peer-to-peer (live-only). run one if you want durable offline queueing:

```sh
poked --relay                     # on a box that stays on
```

`POKE_SECRET` / `POKE_USER` environment variables still work and take precedence
over the config file, which is handy for testing or login items.

for the daemon to be up before you think to run `poke connect`, install the
login item for your platform: `examples/launchd/com.poke.poked.plist` (macOS) or
`examples/systemd/poked.service` (linux). both carry the secret privately rather
than on the command line.
