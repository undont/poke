// package tmux carries the tmux-specific cue: ring the bell on every attached
// client. desktop notifications live in the notify package, which is surface
// agnostic.
package tmux

import (
	"os/exec"
	"strings"
)

// Running reports whether a tmux server is up, used by the `auto` surface to
// pick tmux when one is present and fall back to desktop otherwise. a live
// `tmux list-clients` exits zero whenever a server exists, even with no clients
// attached.
func Running() bool {
	return exec.Command("tmux", "list-clients").Run() == nil
}

// Bell writes \a to every attached client's tty rather than to the origin
// pane, so the cue reaches the recipient wherever they are attached without
// leaving a lingering monitor-bell highlight.
func Bell() error {
	out, err := exec.Command("tmux", "list-clients", "-F", "#{client_tty}").Output()
	if err != nil {
		return err
	}
	for _, tty := range strings.Fields(string(out)) {
		// best effort per client; a detached tty should not fail the rest
		_ = writeBell(tty)
	}
	return nil
}

func writeBell(tty string) error {
	return exec.Command("sh", "-c", "printf '\\a' > "+shellQuote(tty)).Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
