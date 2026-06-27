package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/undont/poke/internal/config"
)

// ensureDaemon makes `poke connect` self-sufficient: if no daemon answers the
// socket, it launches poked detached and waits for it to come up.
func ensureDaemon(cfg *config.Config) error {
	if daemonUp(cfg.SocketPath) {
		return nil
	}
	bin, err := pokedPath()
	if err != nil {
		return err
	}
	if err := spawn(cfg, bin); err != nil {
		return err
	}
	if !waitForSocket(cfg.SocketPath, 5*time.Second) {
		return fmt.Errorf("started poked but its socket never came up")
	}
	return nil
}

// daemonUp reports whether something is listening on the socket.
func daemonUp(path string) bool {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// pokedPath finds the daemon binary: first beside this executable, then $PATH.
func pokedPath() (string, error) {
	if self, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(self), "poked")
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	return exec.LookPath("poked")
}

// spawn starts poked in its own session so it outlives this CLI, with output
// redirected to a log under the state dir.
func spawn(cfg *config.Config, bin string) error {
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(cfg.StateDir, "poked.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logf.Close()

	cmd := exec.Command(bin)
	cmd.Env = os.Environ()
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func waitForSocket(path string, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if daemonUp(path) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
