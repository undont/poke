// package ipc carries the CLI <-> daemon local channel over a unix socket:
// one JSON request per connection, one JSON reply, then close.
package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/undont/poke/internal/protocol"
)

// Handler turns one request into one reply.
type Handler func(protocol.IPCRequest) protocol.IPCResponse

// Server accepts CLI connections on a unix socket.
type Server struct {
	path string
	ln   net.Listener
}

// Listen binds the unix socket, replacing a stale one left by a crash.
func Listen(path string) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := removeStale(path); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	return &Server{path: path, ln: ln}, nil
}

// Serve loops accepting connections until the listener is closed.
func (s *Server) Serve(h Handler) error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go serveConn(conn, h)
	}
}

// Close stops the listener and removes the socket file.
func (s *Server) Close() error {
	err := s.ln.Close()
	_ = os.Remove(s.path)
	return err
}

func serveConn(conn net.Conn, h Handler) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	var req protocol.IPCRequest
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&req); err != nil {
		writeResp(conn, protocol.IPCResponse{OK: false, Error: "bad request"})
		return
	}
	writeResp(conn, h(req))
}

func writeResp(conn net.Conn, resp protocol.IPCResponse) {
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = conn.Write(b)
}

// Send dials the daemon, sends one request, and returns its reply.
func Send(path string, req protocol.IPCRequest) (protocol.IPCResponse, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return protocol.IPCResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	b, _ := json.Marshal(req)
	b = append(b, '\n')
	if _, err := conn.Write(b); err != nil {
		return protocol.IPCResponse{}, err
	}
	var resp protocol.IPCResponse
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return protocol.IPCResponse{}, err
	}
	return resp, nil
}

// removeStale clears a leftover socket only if nothing is listening on it.
func removeStale(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if conn, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		conn.Close()
		return errors.New("daemon already running on " + path)
	}
	return os.Remove(path)
}
