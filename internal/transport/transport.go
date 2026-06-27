// package transport isolates the framed connection between daemon and relay.
// the protocol rides one JSON object per websocket message; how the bytes move
// (a plain websocket today, TLS-wrapped later) lives behind this interface.
package transport

import "context"

// Conn is one framed bidirectional connection.
type Conn interface {
	// ReadFrame returns the next raw JSON frame.
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
