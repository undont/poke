<div align="center">

# poke

**A terminal-native "tap on the shoulder". Built for small dev teams.**

[![Release](https://img.shields.io/github/v/release/undont/poke?style=flat&logo=github&logoColor=white&label=Release&color=2EA043)](https://github.com/undont/poke/releases) [![Licence](https://img.shields.io/github/license/undont/poke?style=flat&label=licence&color=2EA043)](LICENCE) [![Go](https://img.shields.io/badge/Go-1.26+-3FB950?style=flat&logo=go&logoColor=white)](go.mod) [![macOS](https://img.shields.io/badge/macOS-supported-6e7681?style=flat&logo=apple&logoColor=white)](#installation) [![Linux](https://img.shields.io/badge/Linux-supported-6e7681?style=flat&logo=linux&logoColor=white)](#installation)

[Quick Start](#quick-start) · [How It Works](#how-it-works) · [Commands](#commands) · [Team Setup](#team-setup) · [Build](#build)

</div>

---

One dev pokes another and the target's tmux status bar lights up, optionally with a bell or a desktop notification, scaled by an urgency level. No Slack, no browser.

Three artifacts come from one Go binary set:

- **`poke`** as the CLI, ephemeral and talking only to the local daemon over a unix socket (`name` / `secret` / `render` act on local config and files directly)
- **`poked`** as the per-machine daemon, holding the relay connection, tracking presence, and writing the tmux alert surface on an incoming poke
- **`poked --relay`** as the same binary in relay mode, one always-on box on the LAN that routes pokes and holds the offline queue

---

## Quick Start

```sh
brew install undont/tap/poke   # macOS and Linux

poke secret                    # paste the shared team secret (hidden prompt)
poke name sean                 # display name, defaults to $USER
poke connect                   # starts the daemon if it is not up
poke alice "prod is down" --high   # urgency flag may go anywhere; default medium
```

See [Team Setup](#team-setup) for minting and sharing the secret across machines.

---

## How It Works

A poke travels across machines end to end: the daemon finds the relay over mDNS, dials it over a websocket, and on delivery writes the peers file and rings the bell. `poke render` paints a status-right segment (icon, stable per-user colour, an urgency arrow, `+N` overflow); see [`examples/tmux.conf`](examples/tmux.conf) for the status line and clear keybinding. `poke show` prints the full message behind each poke, in the same style, and clears them once you've read them (`--keep` to just peek).

A poke to an offline target is queued durably on the relay (bbolt) and drained in order when they reconnect, dropping anything past `POKE_QUEUE_TTL` (default 24h). `poke who` shows the live roster, kept current as peers join and leave. Every poke persists until you clear it; urgency drives only how loud the arrival is.

When no relay is advertising, the daemon falls back to live-only delivery: it resolves the target's daemon over mDNS and pokes it directly. This lands only while both peers are online (no durable queue without a relay), and the CLI reports the mode it used (`delivered` / `queued` / `live-only`) so the sender is never misled about durability.

Clearing a poke acknowledges it: the recipient's daemon sends a seen ack back to the sender (via the relay), who gets a "saw your poke" notification. The sender's poke already reported `delivered`; seen arrives later, when they look.

How an incoming poke is *surfaced* is the recipient's choice (`poke surface`, or `POKE_SURFACE`), independent of the sender. The `tmux` surface (default) is the ambient status-bar segment plus a bell on every attached client, with a desktop notification at high urgency. The `desktop` surface drops the tmux dependency and makes an OS notification the primary cue (`terminal-notifier` on macOS, `notify-send` on Linux): medium notifies at normal priority, high at critical. `auto` uses tmux when a server is up and desktop otherwise. The urgency ladder means the same on both surfaces; only the cue differs. The peers file is always written, so `poke who` and `poke render` work regardless of surface, the latter with `--format=ansi` for a shell prompt or polybar/waybar/sketchybar.

---

## Commands

`poke` talks only to the local daemon over a unix socket. `name`, `secret`, and `render` act on local config and files directly.

| Command | Action |
|---------|--------|
| `poke <user> [msg] [--high]` | Poke a teammate; urgency flag may go anywhere, default medium |
| `poke connect` | Start the daemon and join the relay if it is not up |
| `poke disconnect` | Stop the daemon |
| `poke clear` | Clear your current poke and send a seen ack |
| `poke show [--keep]` | Print pending pokes with message and urgency; clears them (and sends a seen ack) unless `--keep` |
| `poke who` | Show the live roster of peers |
| `poke dnd` | Toggle do-not-disturb |
| `poke name <name>` | Set your display name, defaults to `$USER` |
| `poke secret [--generate]` | Set the shared secret; `--generate` mints and copies one |
| `poke surface [tmux\|desktop\|auto]` | Show or set how incoming pokes are surfaced |
| `poke render [--format=ansi\|tmux]` | Paint the status segment; `ansi` suits a non-tmux bar or prompt |

---

## Installation

Homebrew is the simplest route on macOS and Linux:

```sh
brew install undont/tap/poke
```

The install script suits machines without Homebrew. It pulls the per-platform tarball for your machine from the latest release into `~/.local/bin` (override with `POKE_INSTALL_DIR`), and builds from source with Go if no release exists yet:

```sh
curl -fsSL https://raw.githubusercontent.com/undont/poke/main/install.sh | bash
```

Manual routes:

```sh
go install github.com/undont/poke/cmd/poke@latest
go install github.com/undont/poke/cmd/poked@latest
# or, from a checkout:
make install
```

---

## Team Setup

Every machine on the team shares one secret. Whoever sets up poke initialises the secret:

```sh
poke secret --generate     # strong secret: stored, and copied to your clipboard
```

Share that value out of band, ideally a team password manager (or a DM). Users who want to join the poke network run the following:

```sh
poke secret                # hidden prompt; paste the shared value (or pipe it in)
poke name bob              # display name, defaults to $USER
poke connect               # starts the daemon if it is not up
poke alice "prod is down" --high   # urgency flag may go anywhere; default medium
```

Both persist to `$XDG_CONFIG_HOME/poke/config` (mode 0600). `poke secret` reads a hidden prompt or stdin, like `gh secret set`, and never echoes the value. If your team keeps secrets in a password manager with a CLI, pull it straight in instead:

```sh
op read "op://team/poke/secret" | poke secret
```

A relay is optional: with no relay on the network the daemon delivers directly peer-to-peer (live-only). Run one if you want durable offline queueing:

```sh
poked --relay                     # on a box that stays on, stable port :7373
```

On a flat LAN the daemon finds the relay over mDNS, no addresses typed. To reach a relay that mDNS will not surface (a different subnet, or a fixed box), set `relay_addr` in the config file (or `POKE_RELAY_ADDR`) to its `host:port`; the daemon then dials it directly and skips discovery. The relay listens on `:7373` by default, overridable with `relay_listen` / `POKE_RELAY_LISTEN`.

```sh
# ~/.config/poke/config
relay_addr = relaybox.example:7373
```

`POKE_SECRET` and `POKE_USER` environment variables still work and take precedence over the config file, which is handy for testing or login items.

For the daemon to be up before you think to run `poke connect`, install the login item for your platform: [`examples/launchd/com.poke.poked.plist`](examples/launchd/com.poke.poked.plist) (macOS) or [`examples/systemd/poked.service`](examples/systemd/poked.service) (Linux). Both carry the secret privately rather than on the command line.

---

## Build

```sh
make build          # bin/poke and bin/poked, version stamped from git
make install        # install both binaries into the go bin dir
make dist           # cross-compiled per-platform tarballs into dist/
make test           # run all tests
make lint           # check formatting and run go vet
make help           # list every target
```

---

## Licence

[MIT](LICENCE)

