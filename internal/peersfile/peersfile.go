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

// Read returns the live pokes on disk, skipping any malformed line. it takes no
// lock: the renderer runs on every status refresh and must not block a writer,
// and a torn line is simply dropped until the next refresh. a missing file
// reads as no pokes.
func Read(path string) ([]Entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for line := range strings.SplitSeq(string(b), "\n") {
		if line == "" {
			continue
		}
		if e, ok := decode(line); ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func decode(line string) (Entry, bool) {
	f := strings.SplitN(line, ":", 6)
	if len(f) != 6 {
		return Entry{}, false
	}
	ts, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil {
		return Entry{}, false
	}
	return Entry{
		From:     f[0],
		Strength: protocol.Strength(f[1]),
		TS:       ts,
		ID:       f[3],
		Seen:     f[4] == "1",
		Note:     decodeNote(f[5]),
	}, true
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

// decodeNote reverses encodeNote.
func decodeNote(s string) string {
	r := strings.NewReplacer("%3A", ":", "%0A", "\n", "%25", "%")
	return r.Replace(s)
}
