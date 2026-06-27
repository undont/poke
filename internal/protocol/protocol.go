// package protocol defines the daemon <-> relay wire protocol.
// newline-delimited JSON, one object per frame, each tagged with a `type`.
package protocol

// Strength is the urgency ladder.
type Strength string

const (
	Low    Strength = "low"
	Medium Strength = "medium"
	High   Strength = "high"
)

// ValidStrength reports whether s is one of the three known levels.
func ValidStrength(s Strength) bool {
	switch s {
	case Low, Medium, High:
		return true
	}
	return false
}

// NoteMaxBytes caps the optional poke note.
const NoteMaxBytes = 120

// frame type tags
const (
	TypeHello    = "hello"
	TypeWelcome  = "welcome"
	TypePoke     = "poke"
	TypePoked    = "poked"
	TypeAck      = "ack"
	TypeQueued   = "queued"
	TypePresence = "presence"
	TypeError    = "error"
)

// RosterEntry is one known peer in the relay roster.
type RosterEntry struct {
	User string `json:"user"`
	Host string `json:"host"`
}

// Hello is the daemon's first frame; the relay validates Secret once per
// connection and never logs it.
type Hello struct {
	Type   string `json:"type"`
	User   string `json:"user"`
	Host   string `json:"host"`
	Secret string `json:"secret"`
}

// Welcome answers a good Hello and carries the current roster.
type Welcome struct {
	Type     string        `json:"type"`
	Roster   []RosterEntry `json:"roster"`
	Protocol int           `json:"protocol"`
}

// Poke travels sender daemon -> relay.
type Poke struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	To       string   `json:"to"`
	Strength Strength `json:"strength"`
	Note     string   `json:"note,omitempty"`
	TS       int64    `json:"ts"`
}

// Poked travels relay -> recipient daemon, live or drained from the queue.
type Poked struct {
	Type     string   `json:"type"`
	ID       string   `json:"id"`
	From     string   `json:"from"`
	Strength Strength `json:"strength"`
	Note     string   `json:"note,omitempty"`
	TS       int64    `json:"ts"`
}

// Ack reports delivery/seen back to the sender.
type Ack struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Seen bool   `json:"seen"`
}

// QueuedNotice tells the sender its poke was stored for an offline target.
type QueuedNotice struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Presence is broadcast on roster changes.
type Presence struct {
	Type   string `json:"type"`
	User   string `json:"user"`
	Online bool   `json:"online"`
}

// Error is a typed failure frame (e.g. unknown target).
type Error struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Envelope is used to peek at a frame's type before full decode.
type Envelope struct {
	Type string `json:"type"`
}
