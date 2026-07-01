// package render turns the live pokes on disk into either a compact status
// segment (icon, each poker's name in a stable per-user colour, an urgency
// arrow, and a +N overflow when more pokes are pending than fit) or a full
// pretty-printed list for `poke show`. the segment emits tmux status markup by
// default, or plain ANSI for a shell prompt or polybar/waybar/sketchybar; the
// list is always ANSI, since `poke show` is a direct terminal read.
package render

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/undont/poke/internal/peersfile"
	"github.com/undont/poke/internal/protocol"
)

// MaxNames caps how many pokers are named before collapsing to +N.
const MaxNames = 3

// defaultIcon is a nerd-font bell; override with POKE_ICON.
const defaultIcon = ""

// Format selects the styling dialect of the segment.
type Format string

const (
	FormatTmux Format = "tmux"
	FormatANSI Format = "ansi"
)

// palette is a curated set of readable 256-colour indices a username hashes
// into. a stable hash means a given person is always the same colour for
// everyone, and the indices are shared by the tmux and ANSI dialects.
var palette = []int{
	39, 208, 76, 213, 178, 45, 170, 154, 203, 81, 222, 141,
}

// Options tunes a render.
type Options struct {
	Icon   string
	Max    int
	Format Format
}

// Segment builds the status segment for the given pokes. it returns the empty
// string when there are none, so the segment collapses cleanly.
func Segment(entries []peersfile.Entry, opt Options) string {
	if len(entries) == 0 {
		return ""
	}
	icon := opt.Icon
	if icon == "" {
		icon = defaultIcon
	}
	max := opt.Max
	if max <= 0 {
		max = MaxNames
	}
	st := stylerFor(opt.Format)

	var b strings.Builder
	b.WriteString(st.fg(paletteIndex(highest(entries))))
	b.WriteString(icon)
	b.WriteString(st.reset())

	shown := entries
	overflow := 0
	if len(entries) > max {
		shown = entries[:max]
		overflow = len(entries) - max
	}
	for _, e := range shown {
		b.WriteByte(' ')
		b.WriteString(name(e, st))
	}
	if overflow > 0 {
		b.WriteString(" +")
		b.WriteString(strconv.Itoa(overflow))
	}
	return b.String()
}

// name renders one poker: urgency arrow, then colour by username.
func name(e peersfile.Entry, st styler) string {
	prefix := urgencyArrow(e.Strength)
	if prefix != "" {
		prefix += " "
	}
	return prefix + st.fg(paletteIndex(e.From)) + e.From + st.reset()
}

// urgencyArrow maps the urgency ladder to an arrow: high up, low down, medium
// unmarked.
func urgencyArrow(s protocol.Strength) string {
	switch s {
	case protocol.High:
		return "↑"
	case protocol.Low:
		return "↓"
	default:
		return ""
	}
}

// styler renders the per-dialect escapes; one implementation per Format.
type styler interface {
	fg(idx int) string
	reset() string
}

func stylerFor(f Format) styler {
	if f == FormatANSI {
		return ansiStyler{}
	}
	return tmuxStyler{}
}

type tmuxStyler struct{}

func (tmuxStyler) fg(idx int) string { return "#[fg=colour" + strconv.Itoa(idx) + "]" }
func (tmuxStyler) reset() string     { return "#[default]" }

type ansiStyler struct{}

func (ansiStyler) fg(idx int) string { return "\x1b[38;5;" + strconv.Itoa(idx) + "m" }
func (ansiStyler) reset() string     { return "\x1b[0m" }

// paletteIndex picks a stable palette entry from the username hash.
func paletteIndex(user string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(user))
	return palette[int(h.Sum32())%len(palette)]
}

// colour is the tmux colour token for a user, retained for callers and tests
// that want the tmux dialect directly.
func colour(user string) string {
	return "colour" + strconv.Itoa(paletteIndex(user))
}

// List pretty-prints the live pokes for `poke show`: one line each, newest
// first, an urgency arrow, the sender in their colour, the note, and how long
// ago it arrived.
func List(entries []protocol.PokeEntry) string {
	if len(entries) == 0 {
		return "no pokes waiting"
	}
	ordered := make([]protocol.PokeEntry, len(entries))
	copy(ordered, entries)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].TS > ordered[j].TS })

	nameWidth, noteWidth := 0, 0
	for _, e := range ordered {
		nameWidth = max(nameWidth, len(e.From))
		noteWidth = max(noteWidth, len(e.Note))
	}

	st := ansiStyler{}
	var b strings.Builder
	for i, e := range ordered {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%-2s%s%-*s%s  %-*s  %s",
			urgencyArrow(e.Strength), st.fg(paletteIndex(e.From)), nameWidth, e.From, st.reset(),
			noteWidth, e.Note, relTime(e.TS))
	}
	return b.String()
}

// relTime renders a unix timestamp as a short "how long ago" string.
func relTime(ts int64) string {
	d := time.Since(time.Unix(ts, 0))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// highest returns the loudest poker's name, used to colour the icon.
func highest(entries []peersfile.Entry) string {
	rank := map[protocol.Strength]int{protocol.Low: 0, protocol.Medium: 1, protocol.High: 2}
	best := entries[0]
	for _, e := range entries[1:] {
		if rank[e.Strength] > rank[best.Strength] {
			best = e
		}
	}
	return best.From
}
