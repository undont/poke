// package config resolves runtime paths, identity, and the shared secret.
package config

import (
	"os"
	"path/filepath"
)

// Config is the resolved daemon/CLI configuration.
type Config struct {
	User       string // self-claimed username, defaults to $USER
	Host       string // os hostname
	Secret     string // shared team secret, never logged
	SocketPath string // CLI <-> daemon unix socket
	PeersFile  string // tmux alert surface for incoming pokes
	StateDir   string // logs and durable daemon state
	RelayAddr  string // optional fixed relay address, empty means mDNS
	Icon       string // status-bar glyph for incoming pokes, empty means default
}

// Load assembles a Config from environment and sensible defaults.
func Load() (*Config, error) {
	host, err := os.Hostname()
	if err != nil {
		host = "unknown"
	}
	c := &Config{
		User:       firstNonEmpty(os.Getenv("POKE_USER"), os.Getenv("USER"), "unknown"),
		Host:       host,
		Secret:     os.Getenv("POKE_SECRET"),
		SocketPath: socketPath(),
		PeersFile:  peersFile(),
		StateDir:   stateDir(),
		RelayAddr:  os.Getenv("POKE_RELAY_ADDR"),
		Icon:       os.Getenv("POKE_ICON"),
	}
	return c, nil
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
