package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matthewtole/devtree/internal/config"
)

func testModel() Model {
	cfg := &config.Config{
		Services: []config.Service{
			{Name: "backend", Repo: "/r/backend", Command: "npm run dev"},
			{Name: "web", Repo: "/r/web", Command: "npm run dev"},
			{Name: "storybook", Repo: "/r/web", Command: "npm run storybook"},
		},
	}
	return New(cfg)
}

func applyState(m Model) Model {
	next, _ := m.Update(stateMsg{
		{svc: m.rows[0].svc, status: statusRunning, worktree: "/work/backend", command: "node"},
		{svc: m.rows[1].svc, status: statusIdle, worktree: "/work/web-feature"},
		{svc: m.rows[2].svc, status: statusAbsent},
	})
	return next.(Model)
}

func TestModel_initialCursor(t *testing.T) {
	m := testModel()
	if m.cursor != 0 {
		t.Errorf("initial cursor = %d, want 0", m.cursor)
	}
}

func TestModel_navigation(t *testing.T) {
	m := applyState(testModel())

	down := func(m Model) Model {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		return next.(Model)
	}
	up := func(m Model) Model {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		return next.(Model)
	}

	// j moves down
	m = down(m)
	if m.cursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", m.cursor)
	}
	m = down(m)
	if m.cursor != 2 {
		t.Errorf("after jj: cursor = %d, want 2", m.cursor)
	}
	// clamps at bottom
	m = down(m)
	if m.cursor != 2 {
		t.Errorf("past bottom: cursor = %d, want 2", m.cursor)
	}
	// k moves up
	m = up(m)
	if m.cursor != 1 {
		t.Errorf("after k: cursor = %d, want 1", m.cursor)
	}
	// clamps at top
	m = up(m)
	m = up(m)
	if m.cursor != 0 {
		t.Errorf("past top: cursor = %d, want 0", m.cursor)
	}
}

func TestModel_arrowNavigation(t *testing.T) {
	m := applyState(testModel())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := next.(Model).cursor; got != 1 {
		t.Errorf("↓: cursor = %d, want 1", got)
	}
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := next.(Model).cursor; got != 0 {
		t.Errorf("↑: cursor = %d, want 0", got)
	}
}

func TestModel_stateUpdates(t *testing.T) {
	m := applyState(testModel())

	if !m.ready {
		t.Error("model not ready after stateMsg")
	}
	if m.rows[0].status != statusRunning {
		t.Errorf("rows[0].status = %v, want running", m.rows[0].status)
	}
	if m.rows[1].status != statusIdle {
		t.Errorf("rows[1].status = %v, want idle", m.rows[1].status)
	}
	if m.rows[2].status != statusAbsent {
		t.Errorf("rows[2].status = %v, want absent", m.rows[2].status)
	}
}

func TestModel_viewContainsServiceNames(t *testing.T) {
	m := applyState(testModel())
	view := m.View()
	for _, name := range []string{"backend", "web", "storybook"} {
		if !strings.Contains(view, name) {
			t.Errorf("view missing service name %q", name)
		}
	}
}

func TestModel_viewShowsWorktreeBase(t *testing.T) {
	m := applyState(testModel())
	view := m.View()
	// worktree "/work/backend" → base = "backend"
	if !strings.Contains(view, "backend") {
		t.Errorf("view missing worktree base; view:\n%s", view)
	}
}

func TestModel_viewNotReadyShowsLoading(t *testing.T) {
	m := testModel()
	view := m.View()
	if !strings.Contains(view, "loading") {
		t.Errorf("pre-ready view should mention loading; got:\n%s", view)
	}
}

func TestModel_errorView(t *testing.T) {
	m := testModel()
	next, _ := m.Update(errMsg{err: fmt.Errorf("tmux exploded")})
	view := next.(Model).View()
	if !strings.Contains(view, "tmux exploded") {
		t.Errorf("error view should contain error message; got:\n%s", view)
	}
}

func TestIsShell(t *testing.T) {
	for _, s := range []string{"bash", "zsh", "fish", "sh", "dash"} {
		if !isShell(s) {
			t.Errorf("isShell(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"node", "python3", "go", "ruby", "npm"} {
		if isShell(s) {
			t.Errorf("isShell(%q) = true, want false", s)
		}
	}
}
