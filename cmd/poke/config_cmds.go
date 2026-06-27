package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/undont/poke/internal/config"
)

// setName prints the current name, or persists a new one to the config file.
func setName(cfg *config.Config, args []string) error {
	if len(args) == 0 {
		fmt.Println(cfg.User)
		return nil
	}
	name := args[0]
	if err := config.ValidateName(name); err != nil {
		return err
	}
	if err := config.SetValue("user", name); err != nil {
		return err
	}
	fmt.Printf("name set to %s\n", name)
	if os.Getenv("POKE_USER") != "" {
		fmt.Fprintln(os.Stderr, "note: POKE_USER is set and overrides the config file")
	}
	nudgeRestart(cfg)
	return nil
}

// setSecret stores the shared team secret in the config file. with --generate
// it mints a strong secret and copies it to the clipboard; otherwise, like `gh
// secret set`, it reads from a hidden prompt on a terminal or from stdin when
// piped, and never echoes the value back.
func setSecret(cfg *config.Config, args []string) error {
	if len(args) > 0 && (args[0] == "--generate" || args[0] == "-g") {
		return generateSecret(cfg)
	}
	secret, err := readSecret(args)
	if err != nil {
		return err
	}
	secret = strings.TrimRight(secret, "\r\n")
	if secret == "" {
		return fmt.Errorf("empty secret")
	}
	if strings.ContainsAny(secret, "\r\n") {
		return fmt.Errorf("secret must be a single line")
	}
	if err := config.SetValue("secret", secret); err != nil {
		return err
	}
	fmt.Println("secret saved")
	if os.Getenv("POKE_SECRET") != "" {
		fmt.Fprintln(os.Stderr, "note: POKE_SECRET is set and overrides the config file")
	}
	nudgeRestart(cfg)
	return nil
}

// generateSecret mints a 256-bit secret, stores it, and copies it to the
// clipboard so it never lands in terminal scrollback. without a clipboard tool
// it falls back to printing, the only way to still hand it off.
func generateSecret(cfg *config.Config) error {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return err
	}
	secret := hex.EncodeToString(b[:])
	if err := config.SetValue("secret", secret); err != nil {
		return err
	}
	if err := copyClipboard(secret); err == nil {
		fmt.Println("secret generated and copied to the clipboard")
		fmt.Fprintln(os.Stderr, "share it with your team out of band (e.g. a password manager); on each machine run `poke secret` and paste it")
	} else {
		fmt.Fprintln(os.Stderr, "no clipboard tool found; printing the secret so you can share it:")
		fmt.Println(secret)
	}
	if os.Getenv("POKE_SECRET") != "" {
		fmt.Fprintln(os.Stderr, "note: POKE_SECRET is set and overrides the config file")
	}
	nudgeRestart(cfg)
	return nil
}

// copyClipboard pipes s to the platform clipboard tool, if one is present.
func copyClipboard(s string) error {
	var cmd *exec.Cmd
	switch {
	case hasExec("pbcopy"):
		cmd = exec.Command("pbcopy")
	case hasExec("wl-copy"):
		cmd = exec.Command("wl-copy")
	case hasExec("xclip"):
		cmd = exec.Command("xclip", "-selection", "clipboard")
	case hasExec("xsel"):
		cmd = exec.Command("xsel", "--clipboard", "--input")
	default:
		return fmt.Errorf("no clipboard tool")
	}
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

func hasExec(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// readSecret gets the secret value without ever printing it: an explicit
// argument (with a history warning), a hidden terminal prompt, or piped stdin.
func readSecret(args []string) (string, error) {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "warning: passing the secret as an argument can leave it in your shell history; prefer `poke secret` (prompts) or pipe it in")
		return args[0], nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "paste your secret: ")
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	b, err := io.ReadAll(os.Stdin)
	return string(b), err
}

// nudgeRestart reminds the user to restart a running daemon so it picks up the
// new config, which it only reads at startup.
func nudgeRestart(cfg *config.Config) {
	if daemonUp(cfg.SocketPath) {
		fmt.Fprintln(os.Stderr, "restart the daemon to apply: stop poked, then run `poke connect`")
	}
}
