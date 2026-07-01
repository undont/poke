package protocol

// the CLI <-> daemon local channel: one-line JSON request, one-line JSON reply.
// the CLI never speaks the wire protocol above; only the daemon does.

// IPC verbs
const (
	IPCConnect    = "connect"
	IPCDisconnect = "disconnect"
	IPCPoke       = "poke"
	IPCClear      = "clear"
	IPCWho        = "who"
	IPCDND        = "dnd"
	IPCShow       = "show"
)

// IPCRequest is one CLI invocation.
type IPCRequest struct {
	Verb     string   `json:"verb"`
	To       string   `json:"to,omitempty"`
	Strength Strength `json:"strength,omitempty"`
	Note     string   `json:"note,omitempty"`
	DND      *bool    `json:"dnd,omitempty"`
	Keep     bool     `json:"keep,omitempty"`
}

// PokeEntry is one live poke as reported to the CLI by `poke show`.
type PokeEntry struct {
	From     string   `json:"from"`
	Strength Strength `json:"strength"`
	TS       int64    `json:"ts"`
	Note     string   `json:"note"`
}

// DeliveryMode reports how a poke left the building.
type DeliveryMode string

const (
	Delivered DeliveryMode = "delivered"
	Queued    DeliveryMode = "queued"
	LiveOnly  DeliveryMode = "live-only"
)

// IPCResponse is the daemon's reply.
type IPCResponse struct {
	OK      bool          `json:"ok"`
	Error   string        `json:"error,omitempty"`
	Mode    DeliveryMode  `json:"mode,omitempty"`
	Roster  []RosterEntry `json:"roster,omitempty"`
	DND     *bool         `json:"dnd,omitempty"`
	Message string        `json:"message,omitempty"`
	Entries []PokeEntry   `json:"entries,omitempty"`
}
