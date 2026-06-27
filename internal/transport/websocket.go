package transport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// the http path daemons dial on the relay.
const wsPath = "/poke"

// readLimit caps a single frame; pokes are tiny.
const readLimit = 64 * 1024

// wsConn adapts a coder/websocket connection to Conn. on the relay side it also
// holds a release channel: closing the conn lets the http handler return, which
// is what actually tears the websocket down.
type wsConn struct {
	c       *websocket.Conn
	release chan struct{}
	once    sync.Once
}

func (w *wsConn) ReadFrame(ctx context.Context) ([]byte, error) {
	_, data, err := w.c.Read(ctx)
	return data, err
}

func (w *wsConn) WriteFrame(ctx context.Context, frame []byte) error {
	return w.c.Write(ctx, websocket.MessageText, frame)
}

func (w *wsConn) Close() error {
	err := w.c.Close(websocket.StatusNormalClosure, "")
	w.once.Do(func() {
		if w.release != nil {
			close(w.release)
		}
	})
	return err
}

// wsDialer dials relays over ws://.
type wsDialer struct{}

// NewDialer returns a websocket-backed Dialer.
func NewDialer() Dialer { return wsDialer{} }

func (wsDialer) Dial(ctx context.Context, addr string) (Conn, error) {
	c, _, err := websocket.Dial(ctx, "ws://"+addr+wsPath, nil)
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(readLimit)
	return &wsConn{c: c}, nil
}

// wsListener serves websocket upgrades over an http server and hands each
// accepted connection to Accept. each http handler blocks until its conn is
// closed, so the consumer owns the connection lifetime.
type wsListener struct {
	ln     net.Listener
	srv    *http.Server
	conns  chan *wsConn
	closed chan struct{}
}

// Listen serves the relay endpoint on ln and returns a Listener.
func Listen(ln net.Listener) Listener {
	l := &wsListener{
		ln:     ln,
		conns:  make(chan *wsConn),
		closed: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(wsPath, l.handle)
	l.srv = &http.Server{Handler: mux}
	go l.srv.Serve(ln)
	return l
}

func (l *wsListener) handle(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	c.SetReadLimit(readLimit)
	conn := &wsConn{c: c, release: make(chan struct{})}
	select {
	case l.conns <- conn:
		<-conn.release // hold the handler open until the consumer closes the conn
	case <-l.closed:
		c.CloseNow()
	}
}

func (l *wsListener) Accept(ctx context.Context) (Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, errors.New("transport: listener closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *wsListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return l.srv.Close()
}
