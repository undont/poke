package protocol

// the CLI <-> daemon local channel: one-line JSON request, one-line JSON reply.
// the CLI never speaks the wire protocol above; only the daemon does.

// IPC verbs
const (
	IPCConnect = "connect"
	IPCPoke    = "poke"
	IPCClear   = "clear"
	IPCWho     = "who"
	IPCDND     = "dnd"
)

// IPCRequest is one CLI invocation.
type IPCRequest struct {
	Verb     string   `json:"verb"`
	To       string   `json:"to,omitempty"`
	Strength Strength `json:"strength,omitempty"`
	Note     string   `json:"note,omitempty"`
	DND      *bool    `json:"dnd,omitempty"`
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
}
