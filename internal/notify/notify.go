// package notify raises desktop notifications, the cue for the desktop surface
// and for high-urgency pokes on the tmux surface. macOS goes through
// terminal-notifier, linux through notify-send; absent either, it is a no-op.
package notify

import "os/exec"

// Priority maps the urgency ladder onto what the notifier can express: normal
// for medium, critical (sticky where supported) for high.
type Priority int

const (
	Normal Priority = iota
	Critical
)

// Send raises a desktop notification, best effort. it returns nil when no
// notifier is installed so a missing tool never fails delivery.
func Send(title, message string, p Priority) error {
	switch {
	case has("terminal-notifier"):
		// terminal-notifier has no urgency switch; -ignoreDnD keeps a critical
		// poke visible past macOS focus modes.
		args := []string{"-title", title, "-message", message}
		if p == Critical {
			args = append(args, "-ignoreDnD")
		}
		return exec.Command("terminal-notifier", args...).Run()
	case has("notify-send"):
		return exec.Command("notify-send", "-u", urgency(p), title, message).Run()
	}
	return nil
}

func urgency(p Priority) string {
	if p == Critical {
		return "critical"
	}
	return "normal"
}

func has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
