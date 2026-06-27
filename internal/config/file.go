package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// the persistent config file is a small key = value text file; lines starting
// with # and blank lines are ignored. it currently carries the user's chosen
// name, set with `poke name`.

// ConfigFile is the path to the persistent config file.
func ConfigFile() string {
	return filepath.Join(configHome(), "poke", "config")
}

// loadFile parses the config file into a key->value map, best effort: a missing
// or unreadable file reads as no values.
func loadFile() map[string]string {
	out := map[string]string{}
	f, err := os.Open(ConfigFile())
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// SetValue upserts one key in the config file, preserving other lines and
// comments.
func SetValue(key, value string) error {
	path := ConfigFile()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	existing, _ := os.ReadFile(path)

	var lines []string
	replaced := false
	prefix := key + " ="
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), key+" =") || strings.HasPrefix(strings.TrimSpace(line), key+"=") {
			lines = append(lines, prefix+" "+value)
			replaced = true
			continue
		}
		lines = append(lines, line)
	}
	if !replaced {
		lines = append(lines, prefix+" "+value)
	}

	out := strings.Join(lines, "\n")
	out = strings.TrimLeft(out, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return os.WriteFile(path, []byte(out), 0o600)
}

var nameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$`)

// ValidateName rejects names that would not survive the peers file or an mDNS
// record: it must start alphanumeric and contain only [A-Za-z0-9._-], up to 32
// characters.
func ValidateName(name string) error {
	if !nameRE.MatchString(name) {
		return fmt.Errorf("invalid name %q: use letters, digits, dot, dash, underscore (max 32, starting alphanumeric)", name)
	}
	return nil
}
