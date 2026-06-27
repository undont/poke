// package transport isolates the framed connection between daemon and relay.
// the protocol rides newline-delimited JSON; how the bytes move (a websocket
// today, TLS-wrapped later) lives behind this interface. the stub keeps the
// tree compiling until coder/websocket is wired in.
package transport

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the placeholder dialer.
var ErrNotImplemented = errors.New("transport: not implemented")

// Conn is one framed bidirectional connection.
type Conn interface {
	// ReadFrame returns the next raw JSON frame, without the delimiter.
	ReadFrame(ctx context.Context) ([]byte, error)
	// WriteFrame sends one raw JSON frame.
	WriteFrame(ctx context.Context, frame []byte) error
	// Close shuts the connection down.
	Close() error
}

// Dialer opens a Conn to a relay address.
type Dialer interface {
	Dial(ctx context.Context, addr string) (Conn, error)
}

// Listener accepts incoming daemon connections on the relay side.
type Listener interface {
	// Accept blocks for the next connection or until ctx is done.
	Accept(ctx context.Context) (Conn, error)
	Close() error
}

// stubDialer is the no-op dialer used before a real transport is wired in.
type stubDialer struct{}

// NewStubDialer returns a placeholder dialer that always fails.
func NewStubDialer() Dialer { return stubDialer{} }

func (stubDialer) Dial(ctx context.Context, addr string) (Conn, error) {
	return nil, ErrNotImplemented
}
