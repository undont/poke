// package relay is the same binary in relay mode: one always-on box that
// authenticates daemons, holds the live roster, routes pokes to connected
// targets, and queues them durably for offline ones.
package relay

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/undont/poke/internal/config"
	"github.com/undont/poke/internal/discovery"
	"github.com/undont/poke/internal/protocol"
	"github.com/undont/poke/internal/queue"
	"github.com/undont/poke/internal/transport"
	"github.com/undont/poke/internal/version"
)

// handshakeTimeout bounds how long a new connection has to send its hello.
const handshakeTimeout = 5 * time.Second

// client is one connected daemon.
type client struct {
	user string
	host string
	conn transport.Conn
	mu   sync.Mutex // serialises writes to conn
}

func (c *client) send(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteFrame(ctx, b)
}

// Relay is the running relay process.
type Relay struct {
	cfg   *config.Config
	log   *slog.Logger
	adv   discovery.Advertiser
	queue *queue.Queue

	mu      sync.Mutex
	clients map[string]*client // user -> client
}

// New constructs a Relay from resolved config.
func New(cfg *config.Config, log *slog.Logger) *Relay {
	return &Relay{
		cfg:     cfg,
		log:     log,
		adv:     discovery.NewAdvertiser(),
		clients: make(map[string]*client),
	}
}

// Run binds an ephemeral port, advertises it over mDNS, and serves daemons
// until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	if err := os.MkdirAll(r.cfg.StateDir, 0o700); err != nil {
		return err
	}
	q, err := queue.Open(r.cfg.StateDir)
	if err != nil {
		return err
	}
	r.queue = q
	defer q.Close()

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		return err
	}
	port := ln.Addr().(*net.TCPAddr).Port

	if err := r.adv.Advertise(ctx, r.cfg.User, true, port); err != nil {
		r.log.Warn("mdns advertise failed", "err", err)
	}
	defer r.adv.Close()

	l := transport.Listen(ln)
	defer l.Close()
	r.log.Info("relay listening", "port", port, "user", r.cfg.User)

	for {
		conn, err := l.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			r.log.Warn("accept failed", "err", err)
			continue
		}
		go r.serve(ctx, conn)
	}
}

// serve runs one connection: handshake, then dispatch frames until it drops.
func (r *Relay) serve(ctx context.Context, conn transport.Conn) {
	c, err := r.handshake(ctx, conn)
	if err != nil {
		r.log.Info("handshake rejected", "err", err)
		_ = conn.Close()
		return
	}
	defer r.drop(ctx, c)
	r.log.Info("daemon joined", "user", c.user, "host", c.host)

	for {
		frame, err := conn.ReadFrame(ctx)
		if err != nil {
			return
		}
		r.dispatch(ctx, c, frame)
	}
}

// handshake reads the hello, validates the shared secret, registers the client,
// and replies with the roster.
func (r *Relay) handshake(ctx context.Context, conn transport.Conn) (*client, error) {
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()

	frame, err := conn.ReadFrame(hctx)
	if err != nil {
		return nil, err
	}
	var hello protocol.Hello
	if err := json.Unmarshal(frame, &hello); err != nil || hello.Type != protocol.TypeHello {
		return nil, errClose(conn, "expected hello")
	}
	if hello.Secret != r.cfg.Secret {
		return nil, errClose(conn, "bad secret")
	}

	c := &client{user: hello.User, host: hello.Host, conn: conn}
	r.register(c)

	welcome := protocol.Welcome{
		Type:     protocol.TypeWelcome,
		Roster:   r.roster(),
		Protocol: version.Protocol,
	}
	if err := c.send(ctx, welcome); err != nil {
		return nil, err
	}
	r.broadcast(ctx, protocol.Presence{Type: protocol.TypePresence, User: c.user, Host: c.host, Online: true}, c.user)
	r.drainTo(ctx, c)
	return c, nil
}

// drainTo replays any pokes queued for a just-connected client, in send order,
// dropping ones past the ttl.
func (r *Relay) drainTo(ctx context.Context, c *client) {
	pokes, err := r.queue.Drain(c.user, r.cfg.QueueTTL, time.Now())
	if err != nil {
		r.log.Error("drain failed", "user", c.user, "err", err)
		return
	}
	if len(pokes) == 0 {
		return
	}
	r.log.Info("draining queue", "user", c.user, "count", len(pokes))
	for _, poked := range pokes {
		if err := c.send(ctx, poked); err != nil {
			return
		}
	}
}

// dispatch routes one inbound frame from a connected client.
func (r *Relay) dispatch(ctx context.Context, from *client, frame []byte) {
	var env protocol.Envelope
	if err := json.Unmarshal(frame, &env); err != nil {
		return
	}
	switch env.Type {
	case protocol.TypePoke:
		var p protocol.Poke
		if err := json.Unmarshal(frame, &p); err != nil {
			return
		}
		r.route(ctx, from, p)
	case protocol.TypeAck:
		var a protocol.Ack
		if err := json.Unmarshal(frame, &a); err != nil {
			return
		}
		r.routeAck(ctx, from, a)
	}
}

// routeAck forwards a seen ack back to the original sender, stamping From with
// the authenticated user so it cannot be spoofed. a sender who has gone offline
// simply misses it.
func (r *Relay) routeAck(ctx context.Context, from *client, a protocol.Ack) {
	if a.To == "" {
		return
	}
	r.mu.Lock()
	target := r.clients[a.To]
	r.mu.Unlock()
	if target == nil {
		return
	}
	a.From = from.user
	a.To = ""
	_ = target.send(ctx, a)
}

// route delivers a poke to a connected target, or queues it for an offline one.
func (r *Relay) route(ctx context.Context, from *client, p protocol.Poke) {
	r.mu.Lock()
	target := r.clients[p.To]
	r.mu.Unlock()

	poked := protocol.Poked{
		Type: protocol.TypePoked, ID: p.ID, From: from.user,
		Strength: p.Strength, Note: p.Note, TS: p.TS,
	}

	if target == nil {
		if err := r.queue.Enqueue(p.To, poked); err != nil {
			r.log.Error("enqueue failed", "to", p.To, "err", err)
			_ = from.send(ctx, protocol.Error{Type: protocol.TypeError, ID: p.ID, Code: "enqueue_failed", Message: err.Error()})
			return
		}
		r.log.Info("poke queued", "to", p.To, "from", from.user)
		_ = from.send(ctx, protocol.QueuedNotice{Type: protocol.TypeQueued, ID: p.ID})
		return
	}
	if err := target.send(ctx, poked); err != nil {
		_ = from.send(ctx, protocol.Error{Type: protocol.TypeError, ID: p.ID, Code: "deliver_failed", Message: err.Error()})
		return
	}
	// tell the sender it landed on the target daemon
	_ = from.send(ctx, protocol.Ack{Type: protocol.TypeAck, ID: p.ID, Seen: false})
}

func (r *Relay) register(c *client) {
	r.mu.Lock()
	if old := r.clients[c.user]; old != nil {
		_ = old.conn.Close() // newest connection wins
	}
	r.clients[c.user] = c
	r.mu.Unlock()
}

func (r *Relay) drop(ctx context.Context, c *client) {
	r.mu.Lock()
	if r.clients[c.user] == c {
		delete(r.clients, c.user)
	}
	r.mu.Unlock()
	_ = c.conn.Close()
	r.log.Info("daemon left", "user", c.user)
	r.broadcast(ctx, protocol.Presence{Type: protocol.TypePresence, User: c.user, Online: false}, "")
}

func (r *Relay) roster() []protocol.RosterEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]protocol.RosterEntry, 0, len(r.clients))
	for _, c := range r.clients {
		out = append(out, protocol.RosterEntry{User: c.user, Host: c.host})
	}
	return out
}

// broadcast sends a frame to every client except the named one.
func (r *Relay) broadcast(ctx context.Context, v any, except string) {
	r.mu.Lock()
	targets := make([]*client, 0, len(r.clients))
	for u, c := range r.clients {
		if u != except {
			targets = append(targets, c)
		}
	}
	r.mu.Unlock()
	for _, c := range targets {
		_ = c.send(ctx, v)
	}
}

// errClose sends a typed close reason and returns a sentinel error.
func errClose(conn transport.Conn, msg string) error {
	b, _ := json.Marshal(protocol.Error{Type: protocol.TypeError, Code: "handshake", Message: msg})
	_ = conn.WriteFrame(context.Background(), b)
	return &handshakeError{msg}
}

type handshakeError struct{ msg string }

func (e *handshakeError) Error() string { return e.msg }
