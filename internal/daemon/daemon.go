// package daemon is the per-machine persistent process. it holds the relay
// connection, applies the urgency behaviour on an incoming poke (peers file,
// bell, notify), and exposes the unix socket for the CLI.
package daemon

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/discovery"
	"github.com/undont/poke/internal/id"
	"github.com/undont/poke/internal/ipc"
	"github.com/undont/poke/internal/notify"
	"github.com/undont/poke/internal/peersfile"
	"github.com/undont/poke/internal/protocol"
	"github.com/undont/poke/internal/tmux"
	"github.com/undont/poke/internal/transport"
	"github.com/undont/poke/internal/version"
)

// how long the CLI side waits for a poke to be confirmed.
const ackTimeout = 2 * time.Second

// how long to wait for a peer's hello on an incoming direct connection.
const handshakeTimeout = 5 * time.Second

const (
	backoffMin  = 500 * time.Millisecond // pause before reconnecting after a dropped relay
	relaySearch = 4 * time.Second        // how long to look for a relay each cycle
	relayRetry  = 8 * time.Second        // how long to stay live-only before re-checking
	peerSearch  = 3 * time.Second        // how long to look for a target daemon in live-only
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
	locator discovery.RelayLocator
	adv     discovery.Advertiser

	cancel context.CancelFunc // stops the daemon, set in Run

	mu      sync.Mutex
	conn    transport.Conn
	peerSet map[string]protocol.RosterEntry // currently-online peers, keyed by user
	pending map[string]chan outcome
	dnd     bool
}

// New constructs a Daemon from resolved config. a configured relay_addr selects
// the fixed-address discovery tier (no mDNS); otherwise the daemon browses the
// LAN for a relay.
func New(cfg *config.Config, log *slog.Logger) *Daemon {
	d := &Daemon{
		cfg:     cfg,
		log:     log,
		peers:   peersfile.New(cfg.PeersFile),
		dialer:  transport.NewDialer(),
		browser: discovery.NewBrowser(),
		adv:     discovery.NewAdvertiser(),
		peerSet: make(map[string]protocol.RosterEntry),
		pending: make(map[string]chan outcome),
	}
	if cfg.RelayAddr != "" {
		d.locator = discovery.NewFixedLocator(cfg.RelayAddr)
	} else {
		d.locator = discovery.NewMDNSLocator(d.browser)
	}
	return d
}

// Run binds the unix socket, advertises and serves direct peer connections,
// drives the relay connection, and serves the CLI until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.cancel = cancel

	srv, err := ipc.Listen(d.cfg.SocketPath)
	if err != nil {
		return err
	}

	// listen for direct peer pokes and advertise this daemon so others can find
	// it, both for the live roster and for live-only delivery when no relay is up.
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := d.adv.Advertise(ctx, d.cfg.User, false, port); err != nil {
		d.log.Warn("mdns advertise failed", "err", err)
	}
	d.log.Info("daemon listening", "socket", d.cfg.SocketPath, "p2p", port, "user", d.cfg.User)

	go d.serveP2P(ctx, ln)
	go d.connectLoop(ctx)
	go func() {
		<-ctx.Done()
		_ = srv.Close()
		_ = d.adv.Close()
	}()
	return srv.Serve(d.handle)
}

// serveP2P accepts direct peer connections for live-only delivery.
func (d *Daemon) serveP2P(ctx context.Context, ln net.Listener) {
	l := transport.Listen(ln)
	go func() {
		<-ctx.Done()
		_ = l.Close()
	}()
	for {
		conn, err := l.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go d.servePeer(ctx, conn)
	}
}

// servePeer handles one inbound direct connection: authenticate, then deliver
// any poked frames the peer sends, acking each.
func (d *Daemon) servePeer(ctx context.Context, conn transport.Conn) {
	defer conn.Close()

	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	frame, err := conn.ReadFrame(hctx)
	cancel()
	if err != nil {
		return
	}
	var hello protocol.Hello
	if json.Unmarshal(frame, &hello) != nil || hello.Type != protocol.TypeHello {
		return
	}
	if hello.Secret != d.cfg.Secret {
		return
	}
	if err := writeFrame(ctx, conn, protocol.Welcome{Type: protocol.TypeWelcome, Protocol: version.Protocol}); err != nil {
		return
	}

	for {
		frame, err := conn.ReadFrame(ctx)
		if err != nil {
			return
		}
		var env protocol.Envelope
		if json.Unmarshal(frame, &env) != nil {
			continue
		}
		if env.Type == protocol.TypePoked {
			var p protocol.Poked
			if json.Unmarshal(frame, &p) == nil {
				d.deliver(p)
				_ = writeFrame(ctx, conn, protocol.Ack{Type: protocol.TypeAck, ID: p.ID, Seen: false})
			}
		}
	}
}

// connectLoop locates a relay each cycle; finding one it runs a relay session,
// otherwise it stays in live-only mode and re-checks later. while there is no
// relay connection, pokes are delivered directly peer-to-peer.
func (d *Daemon) connectLoop(ctx context.Context) {
	for ctx.Err() == nil {
		findCtx, cancel := context.WithTimeout(ctx, relaySearch)
		relay, err := d.locator.Locate(findCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d.sleep(ctx, relayRetry) // no relay; remain live-only and re-check
			continue
		}
		d.log.Info("relay found", "addr", relay.Addr, "host", relay.Host)
		if err := d.session(ctx, relay.Addr); err != nil && ctx.Err() == nil {
			d.log.Info("relay session ended", "err", err)
		}
		d.setConn(nil)
		d.setRoster(nil) // the roster is unknown while disconnected
		d.sleep(ctx, d.reconnectDelay())
	}
}

// reconnectDelay paces reconnection. with mDNS, re-browsing already throttles
// the loop, so a short pause suffices; with a fixed address Locate returns
// instantly, so back off longer to avoid spinning on an unreachable relay.
func (d *Daemon) reconnectDelay() time.Duration {
	if d.cfg.RelayAddr != "" {
		return relayRetry
	}
	return backoffMin
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
			if a.Seen {
				d.onSeen(a.From)
			} else {
				d.resolve(a.ID, outcome{mode: protocol.Delivered})
			}
		}
	case protocol.TypeQueued:
		var qd protocol.QueuedNotice
		if err := json.Unmarshal(frame, &qd); err == nil {
			d.resolve(qd.ID, outcome{mode: protocol.Queued})
		}
	case protocol.TypeError:
		var e protocol.Error
		if err := json.Unmarshal(frame, &e); err == nil && e.ID != "" {
			d.resolve(e.ID, outcome{err: e.Message})
		}
	case protocol.TypePresence:
		var pr protocol.Presence
		if err := json.Unmarshal(frame, &pr); err == nil {
			d.setPresence(pr)
		}
	}
}

// deliver records an incoming poke and fires the recipient's chosen cue. the
// peers file is written regardless of surface (it is the single source of truth
// for live pokes); only the cue differs between the tmux and desktop surfaces.
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
		return // recorded for when you look, but no active cue
	}
	if d.surface() == config.SurfaceDesktop {
		d.deliverDesktop(p)
		return
	}
	d.deliverTmux(p)
}

// deliverTmux is the ambient status-bar surface: the bell reaches every attached
// client for medium and high, and high additionally raises a desktop
// notification. low is silent, surfaced only by the status segment.
func (d *Daemon) deliverTmux(p protocol.Poked) {
	switch p.Strength {
	case protocol.High:
		_ = tmux.Bell()
		_ = notify.Send("poke from "+p.From, p.Note, notify.Critical)
	case protocol.Medium:
		_ = tmux.Bell()
	}
}

// deliverDesktop makes the OS notification the primary cue, with no tmux
// dependency: medium notifies at normal priority, high at critical. low is
// recorded only.
func (d *Daemon) deliverDesktop(p protocol.Poked) {
	switch p.Strength {
	case protocol.High:
		_ = notify.Send("poke from "+p.From, p.Note, notify.Critical)
	case protocol.Medium:
		_ = notify.Send("poke from "+p.From, p.Note, notify.Normal)
	}
}

// surface resolves the configured notification surface, collapsing auto to tmux
// when a tmux server is up and desktop otherwise.
func (d *Daemon) surface() string {
	if d.cfg.Surface == config.SurfaceAuto {
		if tmux.Running() {
			return config.SurfaceTmux
		}
		return config.SurfaceDesktop
	}
	return d.cfg.Surface
}

// emitSeen acks every live poke back to its sender when the user clears, one
// ack per distinct sender. it needs the relay to route acks, so in live-only
// mode (no relay connection) seen reporting is skipped.
func (d *Daemon) emitSeen() {
	conn := d.getConn()
	if conn == nil {
		return
	}
	entries, err := peersfile.Read(d.cfg.PeersFile)
	if err != nil {
		return
	}
	sent := make(map[string]bool)
	for _, e := range entries {
		if e.From == "" || e.From == d.cfg.User || sent[e.From] {
			continue
		}
		sent[e.From] = true
		_ = writeFrame(context.Background(), conn, protocol.Ack{
			Type: protocol.TypeAck, ID: e.ID, Seen: true, To: e.From,
		})
	}
}

// onSeen surfaces that a poke this machine sent was acknowledged.
func (d *Daemon) onSeen(from string) {
	if from == "" {
		return
	}
	d.log.Info("poke seen", "by", from)
	d.mu.Lock()
	dnd := d.dnd
	d.mu.Unlock()
	if !dnd {
		_ = notify.Send("poke seen", from+" saw your poke", notify.Normal)
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
	case protocol.IPCDisconnect:
		// stop shortly after replying so the cli gets its response first
		go func() {
			time.Sleep(150 * time.Millisecond)
			d.cancel()
		}()
		return protocol.IPCResponse{OK: true, Message: "stopping"}
	case protocol.IPCPoke:
		return d.handlePoke(req)
	case protocol.IPCClear:
		d.emitSeen() // tell senders their pokes were acknowledged, before clearing
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
		// no relay: deliver directly to the target's daemon, live only.
		return d.pokeDirect(req.To, s, clampNote(req.Note))
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

// pokeDirect delivers a poke straight to the target's daemon over mDNS, used
// when no relay is up. there is no durable queue on this path, so an offline
// target is simply unreachable.
func (d *Daemon) pokeDirect(to string, s protocol.Strength, note string) protocol.IPCResponse {
	searchCtx, cancel := context.WithTimeout(context.Background(), peerSearch)
	peer, err := d.browser.FindPeer(searchCtx, to)
	cancel()
	if err != nil {
		return protocol.IPCResponse{OK: false, Error: to + " not reachable (no relay, peer offline)"}
	}

	dctx, cancel := context.WithTimeout(context.Background(), ackTimeout)
	defer cancel()
	conn, err := d.dialer.Dial(dctx, peer.Addr)
	if err != nil {
		return protocol.IPCResponse{OK: false, Error: err.Error()}
	}
	defer conn.Close()

	hello := protocol.Hello{Type: protocol.TypeHello, User: d.cfg.User, Host: d.cfg.Host, Secret: d.cfg.Secret}
	if err := writeFrame(dctx, conn, hello); err != nil {
		return protocol.IPCResponse{OK: false, Error: err.Error()}
	}
	if env, err := readEnvelope(dctx, conn); err != nil || env.Type != protocol.TypeWelcome {
		return protocol.IPCResponse{OK: false, Error: "peer refused connection (secret mismatch?)"}
	}

	poked := protocol.Poked{
		Type: protocol.TypePoked, ID: id.New(), From: d.cfg.User,
		Strength: s, Note: note, TS: time.Now().Unix(),
	}
	if err := writeFrame(dctx, conn, poked); err != nil {
		return protocol.IPCResponse{OK: false, Error: err.Error()}
	}
	if env, err := readEnvelope(dctx, conn); err != nil || env.Type != protocol.TypeAck {
		return protocol.IPCResponse{OK: false, Error: "no ack from peer"}
	}
	return protocol.IPCResponse{OK: true, Mode: protocol.LiveOnly}
}

func (d *Daemon) handleWho() protocol.IPCResponse {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]protocol.RosterEntry, 0, len(d.peerSet))
	for _, e := range d.peerSet {
		if e.User == d.cfg.User {
			continue // don't list yourself
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].User < out[j].User })
	return protocol.IPCResponse{OK: true, Roster: out}
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

// setRoster replaces the peer set from a welcome snapshot. a nil roster clears
// it, which is what a dropped relay connection should do.
func (d *Daemon) setRoster(r []protocol.RosterEntry) {
	d.mu.Lock()
	d.peerSet = make(map[string]protocol.RosterEntry, len(r))
	for _, e := range r {
		d.peerSet[e.User] = e
	}
	d.mu.Unlock()
}

// setPresence keeps the peer set current: a peer coming online is added with
// its host, one going offline is removed.
func (d *Daemon) setPresence(pr protocol.Presence) {
	d.mu.Lock()
	if pr.Online {
		d.peerSet[pr.User] = protocol.RosterEntry{User: pr.User, Host: pr.Host}
	} else {
		delete(d.peerSet, pr.User)
	}
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

// readEnvelope reads the next frame and returns just its type tag.
func readEnvelope(ctx context.Context, conn transport.Conn) (protocol.Envelope, error) {
	frame, err := conn.ReadFrame(ctx)
	if err != nil {
		return protocol.Envelope{}, err
	}
	var env protocol.Envelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return protocol.Envelope{}, err
	}
	return env, nil
}

func clampNote(s string) string {
	if len(s) > protocol.NoteMaxBytes {
		return s[:protocol.NoteMaxBytes]
	}
	return s
}

type sessionError struct{ msg string }

func (e *sessionError) Error() string { return e.msg }
