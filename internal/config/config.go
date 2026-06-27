// package config resolves runtime paths, identity, and the shared secret.
package config

import (
	"os"
	"path/filepath"
	"time"
)

// defaultQueueTTL is how long a poke lingers for an offline target.
const defaultQueueTTL = 24 * time.Hour

// defaultRelayListen is the relay's stable listen address. a fixed port (rather
// than an ephemeral :0) is what lets a daemon dial a relay by a configured
// address without mDNS, the precondition for the off-LAN expansion.
const defaultRelayListen = ":7373"

// notification surfaces. tmux is the ambient status-bar surface; desktop makes
// an OS notification the primary cue; auto picks tmux when a server is up and
// desktop otherwise.
const (
	SurfaceTmux    = "tmux"
	SurfaceDesktop = "desktop"
	SurfaceAuto    = "auto"
)

// Config is the resolved daemon/CLI configuration.
type Config struct {
	User        string        // self-claimed username, defaults to $USER
	Host        string        // os hostname
	Secret      string        // shared team secret, never logged
	SocketPath  string        // CLI <-> daemon unix socket
	PeersFile   string        // tmux alert surface for incoming pokes
	StateDir    string        // logs and durable daemon state
	RelayAddr   string        // optional fixed relay address, empty means mDNS
	RelayListen string        // relay mode: address to listen on, defaults to :7373
	Icon        string        // status-bar glyph for incoming pokes, empty means default
	Surface     string        // how an incoming poke is surfaced: tmux, desktop, auto
	QueueTTL    time.Duration // how long a relay holds a poke for an offline target
}

// Load assembles a Config from environment, the persistent config file, and
// sensible defaults. for each value the environment wins, then the config file,
// then a built-in default; this keeps env overrides handy for testing while the
// config file holds what the user set with `poke name` / `poke secret`.
func Load() (*Config, error) {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	file := loadFile()
	c := &Config{
		User:        firstNonEmpty(os.Getenv("POKE_USER"), file["user"], os.Getenv("USER"), "unknown"),
		Host:        host,
		Secret:      firstNonEmpty(os.Getenv("POKE_SECRET"), file["secret"]),
		SocketPath:  socketPath(),
		PeersFile:   peersFile(),
		StateDir:    stateDir(),
		RelayAddr:   firstNonEmpty(os.Getenv("POKE_RELAY_ADDR"), file["relay_addr"]),
		RelayListen: firstNonEmpty(os.Getenv("POKE_RELAY_LISTEN"), file["relay_listen"], defaultRelayListen),
		Icon:        firstNonEmpty(os.Getenv("POKE_ICON"), file["icon"]),
		Surface:     surface(firstNonEmpty(os.Getenv("POKE_SURFACE"), file["surface"])),
		QueueTTL:    queueTTL(),
	}
	return c, nil
}

// surface normalises a configured surface value, falling back to tmux for an
// empty or unrecognised one so a typo degrades to the default rather than
// silently disabling cues.
func surface(v string) string {
	switch v {
	case SurfaceDesktop, SurfaceAuto, SurfaceTmux:
		return v
	default:
		return SurfaceTmux
	}
}

// queueTTL reads POKE_QUEUE_TTL (a Go duration like "12h"), falling back to the
// default.
func queueTTL() time.Duration {
	if v := os.Getenv("POKE_QUEUE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultQueueTTL
}

// socketPath prefers $XDG_RUNTIME_DIR, falling back to the config dir.
func socketPath() string {
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		return filepath.Join(rt, "poke.sock")
	}
	return filepath.Join(configHome(), "poke", "sock")
}

// peersFile is the tmux-alerts surface the daemon writes incoming pokes to.
func peersFile() string {
	return filepath.Join(configHome(), "tmux-alerts", "peers")
}

func stateDir() string {
	if s := os.Getenv("XDG_STATE_HOME"); s != "" {
		return filepath.Join(s, "poke")
	}
	return filepath.Join(home(), ".local", "state", "poke")
}

func configHome() string {
	if c := os.Getenv("XDG_CONFIG_HOME"); c != "" {
		return c
	}
	return filepath.Join(home(), ".config")
}

func home() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
