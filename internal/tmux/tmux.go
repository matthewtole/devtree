// Package tmux wraps the tmux CLI commands devtree needs to inspect and drive
// a tmux session. Each Client targets one tmux server, selected by the Socket
// field (empty = the default server).
package tmux

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct {
	Socket string
}

func (c *Client) flags(args ...string) []string {
	if c.Socket == "" {
		return args
	}
	return append([]string{"-L", c.Socket}, args...)
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("tmux", c.flags(args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tmux %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// ListSessions returns the names of all sessions on this server. A non-running
// server is treated as an empty list rather than an error — for our purposes
// "no server" and "no sessions" are the same state.
func (c *Client) ListSessions() ([]string, error) {
	out, err := c.run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		if isNoServer(err) {
			return nil, nil
		}
		return nil, err
	}
	return splitLines(out), nil
}

// isNoServer detects the various ways tmux signals "no server is running on
// this socket" — the phrasing varies across versions ("no server running",
// "error connecting to ... No such file or directory").
func isNoServer(err error) bool {
	s := err.Error()
	return strings.Contains(s, "no server running") ||
		strings.Contains(s, "error connecting")
}

func (c *Client) HasSession(name string) (bool, error) {
	sessions, err := c.ListSessions()
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s == name {
			return true, nil
		}
	}
	return false, nil
}

// EnsureSession creates a detached session if one with this name doesn't already exist.
func (c *Client) EnsureSession(name string) error {
	has, err := c.HasSession(name)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.run("new-session", "-d", "-s", name)
	return err
}

func (c *Client) ListWindows(session string) ([]string, error) {
	out, err := c.run("list-windows", "-t", session, "-F", "#{window_name}")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

func (c *Client) HasWindow(session, window string) (bool, error) {
	windows, err := c.ListWindows(session)
	if err != nil {
		return false, err
	}
	for _, w := range windows {
		if w == window {
			return true, nil
		}
	}
	return false, nil
}

// EnsureWindow creates a window in the session if one with this name doesn't exist.
// The session must already exist.
func (c *Client) EnsureWindow(session, window string) error {
	has, err := c.HasWindow(session, window)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = c.run("new-window", "-d", "-t", session+":", "-n", window)
	return err
}

// SendKeys forwards each argument to `tmux send-keys`. Use the tmux key syntax
// for special keys, e.g. SendKeys("foo", "bar", "echo hi", "C-m") sends the string
// then a carriage return.
func (c *Client) SendKeys(session, window string, keys ...string) error {
	args := append([]string{"send-keys", "-t", session + ":" + window}, keys...)
	_, err := c.run(args...)
	return err
}

// GetUserOption reads a window-scoped user option. The leading "@" is added
// automatically. Unset options return "" with no error.
func (c *Client) GetUserOption(session, window, key string) (string, error) {
	out, err := c.run("show-options", "-wqv", "-t", session+":"+window, "@"+key)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

func (c *Client) SetUserOption(session, window, key, value string) error {
	_, err := c.run("set-option", "-w", "-t", session+":"+window, "@"+key, value)
	return err
}

// PaneCurrentCommand returns the foreground command of the window's active pane
// (e.g. "zsh" for an idle shell, "node" while a dev server is running).
func (c *Client) PaneCurrentCommand(session, window string) (string, error) {
	out, err := c.run("display-message", "-p", "-t", session+":"+window, "#{pane_current_command}")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// CapturePane returns the visible contents of the window's active pane.
func (c *Client) CapturePane(session, window string) (string, error) {
	return c.run("capture-pane", "-p", "-t", session+":"+window)
}

// KillServer terminates this tmux server. Useful for tearing down a test-only socket.
func (c *Client) KillServer() error {
	_, err := c.run("kill-server")
	if err != nil && isNoServer(err) {
		return nil
	}
	return err
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
