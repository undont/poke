// package peersfile owns the tmux alert surface for incoming pokes: a file
// of one line per live poke, written under a mkdir lock so concurrent daemon
// writes stay safe. lines are colon-delimited with the note percent-encoded.
package peersfile

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/undont/poke/internal/protocol"
)

// Entry is one live poke as stored on disk.
type Entry struct {
	From     string
	Strength protocol.Strength
	TS       int64
	ID       string
	Seen     bool
	Note     string
}

// Writer serialises access to the peers file.
type Writer struct {
	path string
}

// New returns a Writer for the given peers-file path.
func New(path string) *Writer { return &Writer{path: path} }

// Append adds one poke line, taking the lock for the duration.
func (w *Writer) Append(e Entry) error {
	unlock, err := w.lock()
	if err != nil {
		return err
	}
	defer unlock()
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(encode(e) + "\n")
	return err
}

// Clear truncates the peers file, dismissing every live poke.
func (w *Writer) Clear() error {
	unlock, err := w.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.Remove(w.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// lock implements the mkdir-based mutual exclusion the tmux-alerts scripts use.
func (w *Writer) lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
		return nil, err
	}
	dir := w.path + ".lock"
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := os.Mkdir(dir, 0o700)
		if err == nil {
			return func() { _ = os.Remove(dir) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			// stale lock from a crashed writer; steal it
			_ = os.Remove(dir)
			continue
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func encode(e Entry) string {
	return strings.Join([]string{
		e.From,
		string(e.Strength),
		strconv.FormatInt(e.TS, 10),
		e.ID,
		boolField(e.Seen),
		encodeNote(e.Note),
	}, ":")
}

func boolField(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// encodeNote percent-encodes the bytes that would break colon-delimited parsing.
func encodeNote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ':':
			b.WriteString("%3A")
		case '%':
			b.WriteString("%25")
		case '\n':
			b.WriteString("%0A")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
