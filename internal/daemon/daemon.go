// package daemon is the per-machine persistent process. it holds the relay
// connection, applies the urgency behaviour on an incoming poke (peers file,
// bell, notify), and exposes the unix socket for the CLI.
package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/discovery"
	"github.com/undont/poke/internal/id"
	"github.com/undont/poke/internal/ipc"
	"github.com/undont/poke/internal/peersfile"
	"github.com/undont/poke/internal/protocol"
	"github.com/undont/poke/internal/tmux"
	"github.com/undont/poke/internal/transport"
	"github.com/undont/poke/internal/version"
)

// how long the CLI side waits for the relay to confirm a poke landed.
const ackTimeout = 2 * time.Second

// reconnect backoff bounds.
const (
	backoffMin = 500 * time.Millisecond
	backoffMax = 15 * time.Second
)

// outcome is the relay's verdict on a sent poke, delivered to a waiting handler.
type outcome struct {
	mode protocol.DeliveryMode
	err  string
}

// Daemon is the running per-machine process.
type Daemon struct {
	cfg     *config.Config
	log     *slog.Logger
	peers   *peersfile.Writer
	dialer  transport.Dialer
	browser discovery.Browser

	mu      sync.Mutex
	conn    transport.Conn
	roster  []protocol.RosterEntry
	online  map[string]bool
	pending map[string]chan outcome
	dnd     bool
}

// New constructs a Daemon from resolved config.
func New(cfg *config.Config, log *slog.Logger) *Daemon {
	return &Daemon{
		cfg:     cfg,
		log:     log,
		peers:   peersfile.New(cfg.PeersFile),
		dialer:  transport.NewDialer(),
		browser: discovery.NewBrowser(),
		online:  make(map[string]bool),
		pending: make(map[string]chan outcome),
	}
}

// Run binds the unix socket, drives the relay connection, and serves the CLI
// until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	srv, err := ipc.Listen(d.cfg.SocketPath)
	if err != nil {
		return err
	}
	d.log.Info("daemon listening", "socket", d.cfg.SocketPath, "user", d.cfg.User)

	go d.connectLoop(ctx)
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	return srv.Serve(d.handle)
}

// connectLoop discovers and dials the relay, runs a session, and reconnects
// with capped backoff.
func (d *Daemon) connectLoop(ctx context.Context) {
	backoff := backoffMin
	for ctx.Err() == nil {
		relay, err := d.browser.FindRelay(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d.sleep(ctx, backoff)
			backoff = next(backoff)
			continue
		}
		d.log.Info("relay found", "addr", relay.Addr, "host", relay.Host)
		if err := d.session(ctx, relay.Addr); err != nil && ctx.Err() == nil {
			d.log.Info("relay session ended", "err", err)
		}
		d.setConn(nil)
		d.sleep(ctx, backoff)
		backoff = next(backoff)
	}
}

// session dials, completes the handshake, and pumps frames until it drops.
func (d *Daemon) session(ctx context.Context, addr string) error {
	conn, err := d.dialer.Dial(ctx, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	hello := protocol.Hello{
		Type: protocol.TypeHello, User: d.cfg.User,
		Host: d.cfg.Host, Secret: d.cfg.Secret,
	}
	if err := writeFrame(ctx, conn, hello); err != nil {
		return err
	}

	frame, err := conn.ReadFrame(ctx)
	if err != nil {
		return err
	}
	var env protocol.Envelope
	_ = json.Unmarshal(frame, &env)
	if env.Type != protocol.TypeWelcome {
		return &sessionError{"relay refused connection"}
	}
	var welcome protocol.Welcome
	_ = json.Unmarshal(frame, &welcome)
	if welcome.Protocol != version.Protocol {
		return &sessionError{"protocol mismatch; rebuild poked"}
	}
	d.setRoster(welcome.Roster)
	d.setConn(conn)
	d.log.Info("connected to relay", "peers", len(welcome.Roster))

	for {
		frame, err := conn.ReadFrame(ctx)
		if err != nil {
			return err
		}
		d.handleFrame(frame)
	}
}

// handleFrame dispatches one inbound relay frame.
func (d *Daemon) handleFrame(frame []byte) {
	var env protocol.Envelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return
	}
	switch env.Type {
	case protocol.TypePoked:
		var p protocol.Poked
		if err := json.Unmarshal(frame, &p); err == nil {
			d.deliver(p)
		}
	case protocol.TypeAck:
		var a protocol.Ack
		if err := json.Unmarshal(frame, &a); err == nil {
			d.resolve(a.ID, outcome{mode: protocol.Delivered})
		}
	case protocol.TypeError:
		var e protocol.Error
		if err := json.Unmarshal(frame, &e); err == nil && e.ID != "" {
			d.resolve(e.ID, outcome{err: e.Message})
		}
	case protocol.TypePresence:
		var pr protocol.Presence
		if err := json.Unmarshal(frame, &pr); err == nil {
			d.setOnline(pr.User, pr.Online)
		}
	}
}

// deliver applies the urgency behaviour for an incoming poke.
func (d *Daemon) deliver(p protocol.Poked) {
	e := peersfile.Entry{
		From: p.From, Strength: p.Strength, TS: p.TS, ID: p.ID, Note: p.Note,
	}
	if err := d.peers.Append(e); err != nil {
		d.log.Error("peers append failed", "err", err)
	}
	d.mu.Lock()
	dnd := d.dnd
	d.mu.Unlock()
	if dnd {
		return
	}
	switch p.Strength {
	case protocol.High:
		_ = tmux.Bell()
		_ = tmux.Notify("poke from "+p.From, p.Note)
	case protocol.Medium:
		_ = tmux.Bell()
	}
}

// handle answers one CLI request.
func (d *Daemon) handle(req protocol.IPCRequest) protocol.IPCResponse {
	switch req.Verb {
	case protocol.IPCConnect:
		if d.connected() {
			return protocol.IPCResponse{OK: true, Message: "connected"}
		}
		return protocol.IPCResponse{OK: true, Message: "starting; searching for relay"}
	case protocol.IPCPoke:
		return d.handlePoke(req)
	case protocol.IPCClear:
		if err := d.peers.Clear(); err != nil {
			return protocol.IPCResponse{OK: false, Error: err.Error()}
		}
		return protocol.IPCResponse{OK: true, Message: "cleared"}
	case protocol.IPCWho:
		return d.handleWho()
	case protocol.IPCDND:
		return d.handleDND(req)
	default:
		return protocol.IPCResponse{OK: false, Error: "unknown verb: " + req.Verb}
	}
}

func (d *Daemon) handlePoke(req protocol.IPCRequest) protocol.IPCResponse {
	if req.To == "" {
		return protocol.IPCResponse{OK: false, Error: "no target"}
	}
	s := req.Strength
	if s == "" {
		s = protocol.Medium
	}
	if !protocol.ValidStrength(s) {
		return protocol.IPCResponse{OK: false, Error: "bad strength: " + string(s)}
	}
	conn := d.getConn()
	if conn == nil {
		return protocol.IPCResponse{OK: false, Error: "no relay connection"}
	}

	p := protocol.Poke{
		Type: protocol.TypePoke, ID: id.New(), To: req.To,
		Strength: s, Note: clampNote(req.Note), TS: time.Now().Unix(),
	}
	ch := make(chan outcome, 1)
	d.addPending(p.ID, ch)
	defer d.delPending(p.ID)

	if err := writeFrame(context.Background(), conn, p); err != nil {
		return protocol.IPCResponse{OK: false, Error: err.Error()}
	}

	select {
	case o := <-ch:
		if o.err != "" {
			return protocol.IPCResponse{OK: false, Error: o.err}
		}
		return protocol.IPCResponse{OK: true, Mode: o.mode}
	case <-time.After(ackTimeout):
		return protocol.IPCResponse{OK: true, Mode: protocol.Delivered}
	}
}

func (d *Daemon) handleWho() protocol.IPCResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	online := make([]string, 0, len(d.online))
	for u, on := range d.online {
		if on {
			online = append(online, u)
		}
	}
	return protocol.IPCResponse{OK: true, Roster: d.roster, Online: online}
}

func (d *Daemon) handleDND(req protocol.IPCRequest) protocol.IPCResponse {
	d.mu.Lock()
	if req.DND != nil {
		d.dnd = *req.DND
	}
	v := d.dnd
	d.mu.Unlock()
	return protocol.IPCResponse{OK: true, DND: &v}
}

// connection-state helpers

func (d *Daemon) setConn(c transport.Conn) {
	d.mu.Lock()
	d.conn = c
	d.mu.Unlock()
}

func (d *Daemon) getConn() transport.Conn {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.conn
}

func (d *Daemon) connected() bool { return d.getConn() != nil }

func (d *Daemon) setRoster(r []protocol.RosterEntry) {
	d.mu.Lock()
	d.roster = r
	for _, e := range r {
		d.online[e.User] = true
	}
	d.mu.Unlock()
}

func (d *Daemon) setOnline(user string, on bool) {
	d.mu.Lock()
	d.online[user] = on
	d.mu.Unlock()
}

func (d *Daemon) addPending(id string, ch chan outcome) {
	d.mu.Lock()
	d.pending[id] = ch
	d.mu.Unlock()
}

func (d *Daemon) delPending(id string) {
	d.mu.Lock()
	delete(d.pending, id)
	d.mu.Unlock()
}

func (d *Daemon) resolve(id string, o outcome) {
	d.mu.Lock()
	ch := d.pending[id]
	d.mu.Unlock()
	if ch != nil {
		select {
		case ch <- o:
		default:
		}
	}
}

func (d *Daemon) sleep(ctx context.Context, dur time.Duration) {
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func writeFrame(ctx context.Context, conn transport.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.WriteFrame(ctx, b)
}

func clampNote(s string) string {
	if len(s) > protocol.NoteMaxBytes {
		return s[:protocol.NoteMaxBytes]
	}
	return s
}

func next(b time.Duration) time.Duration {
	b *= 2
	if b > backoffMax {
		return backoffMax
	}
	return b
}

type sessionError struct{ msg string }

func (e *sessionError) Error() string { return e.msg }
