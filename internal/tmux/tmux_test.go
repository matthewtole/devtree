package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// newTestClient gives each test an isolated tmux server via a unique -L socket.
// The server is killed in cleanup so we never touch the user's real tmux.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	socket := fmt.Sprintf("devtree-test-%s-%d",
		sanitize(t.Name()), time.Now().UnixNano())
	c := &Client{Socket: socket}
	t.Cleanup(func() {
		// Capture the socket path before we kill the server; tmux can no
		// longer tell us once it's gone, and the file lingers if we don't.
		path, _ := c.run("display-message", "-p", "#{socket_path}")
		if err := c.KillServer(); err != nil {
			t.Logf("kill-server cleanup: %v", err)
		}
		if path = strings.TrimRight(path, "\n"); path != "" {
			_ = os.Remove(path)
		}
	})
	return c
}

func sanitize(name string) string {
	return strings.NewReplacer("/", "-", " ", "-").Replace(name)
}

func TestListSessions_noServer(t *testing.T) {
	c := newTestClient(t)
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected no sessions, got %v", sessions)
	}
}

func TestHasSession(t *testing.T) {
	c := newTestClient(t)

	has, err := c.HasSession("missing")
	if err != nil || has {
		t.Fatalf("HasSession(missing) = (%v, %v), want (false, nil)", has, err)
	}

	if err := c.EnsureSession("hello"); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	has, err = c.HasSession("hello")
	if err != nil || !has {
		t.Fatalf("HasSession(hello) = (%v, %v), want (true, nil)", has, err)
	}
}

func TestEnsureSession_idempotent(t *testing.T) {
	c := newTestClient(t)
	for i := 0; i < 3; i++ {
		if err := c.EnsureSession("dt"); err != nil {
			t.Fatalf("EnsureSession #%d: %v", i, err)
		}
	}
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0] != "dt" {
		t.Errorf("sessions = %v, want [dt]", sessions)
	}
}

func TestEnsureWindow_idempotent(t *testing.T) {
	c := newTestClient(t)
	if err := c.EnsureSession("dt"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := c.EnsureWindow("dt", "backend"); err != nil {
			t.Fatalf("EnsureWindow #%d: %v", i, err)
		}
	}
	has, err := c.HasWindow("dt", "backend")
	if err != nil || !has {
		t.Errorf("HasWindow(backend) = (%v, %v), want (true, nil)", has, err)
	}
}

func TestUserOption_roundtrip(t *testing.T) {
	c := newTestClient(t)
	if err := c.EnsureSession("dt"); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureWindow("dt", "web"); err != nil {
		t.Fatal(err)
	}

	got, err := c.GetUserOption("dt", "web", "devtree-worktree")
	if err != nil {
		t.Fatalf("Get unset: %v", err)
	}
	if got != "" {
		t.Errorf("unset option = %q, want empty", got)
	}

	if err := c.SetUserOption("dt", "web", "devtree-worktree", "/path/to/main"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err = c.GetUserOption("dt", "web", "devtree-worktree")
	if err != nil {
		t.Fatalf("Get after set: %v", err)
	}
	if want := "/path/to/main"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSendKeys_andCapture(t *testing.T) {
	c := newTestClient(t)
	if err := c.EnsureSession("dt"); err != nil {
		t.Fatal(err)
	}
	if err := c.EnsureWindow("dt", "shell"); err != nil {
		t.Fatal(err)
	}

	if err := c.SendKeys("dt", "shell", "echo devtree-marker-123", "C-m"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	// Shells take a beat to render — poll capture-pane briefly.
	deadline := time.Now().Add(3 * time.Second)
	for {
		out, err := c.CapturePane("dt", "shell")
		if err != nil {
			t.Fatalf("CapturePane: %v", err)
		}
		if strings.Contains(out, "devtree-marker-123") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never saw marker in pane output; last capture:\n%s", out)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestPaneCurrentCommand(t *testing.T) {
	c := newTestClient(t)
	if err := c.EnsureSession("dt"); err != nil {
		t.Fatal(err)
	}
	got, err := c.PaneCurrentCommand("dt", "")
	if err != nil {
		t.Fatalf("PaneCurrentCommand: %v", err)
	}
	// Should be the user's shell — we don't assert which one, just non-empty.
	if got == "" {
		t.Errorf("PaneCurrentCommand returned empty")
	}
}
