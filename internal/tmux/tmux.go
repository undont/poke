// package tmux carries the OS-level cues: ring the bell on attached clients
// and raise a desktop notification for high-urgency pokes.
package tmux

import (
	"os/exec"
	"strings"
)

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

// Notify raises a desktop notification via terminal-notifier when present.
func Notify(title, message string) error {
	if _, err := exec.LookPath("terminal-notifier"); err != nil {
		return nil
	}
	return exec.Command("terminal-notifier", "-title", title, "-message", message).Run()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
