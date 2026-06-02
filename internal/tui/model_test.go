package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matthewtole/devtree/internal/config"
	"github.com/matthewtole/devtree/internal/git"
)

func applyMsg(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

// --- helpers ------------------------------------------------------------

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

func withState(m Model) Model {
	next, _ := m.Update(stateMsg{
		{svc: m.rows[0].svc, status: statusRunning, worktree: "/work/backend", command: "node"},
		{svc: m.rows[1].svc, status: statusIdle, worktree: "/work/web-feature"},
		{svc: m.rows[2].svc, status: statusAbsent},
	})
	return next.(Model)
}

func pressKey(m Model, key string) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next.(Model)
}

func pressSpecial(m Model, t tea.KeyType) Model {
	next, _ := m.Update(tea.KeyMsg{Type: t})
	return next.(Model)
}

// --- navigation ---------------------------------------------------------

func TestModel_initialCursor(t *testing.T) {
	if testModel().cursor != 0 {
		t.Error("initial cursor should be 0")
	}
}

func TestModel_navigation(t *testing.T) {
	m := withState(testModel())

	m = pressKey(m, "j")
	if m.cursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", m.cursor)
	}
	m = pressKey(m, "j")
	m = pressKey(m, "j") // clamp
	if m.cursor != 2 {
		t.Errorf("past bottom: cursor = %d, want 2", m.cursor)
	}
	m = pressKey(m, "k")
	m = pressKey(m, "k")
	m = pressKey(m, "k") // clamp
	if m.cursor != 0 {
		t.Errorf("past top: cursor = %d, want 0", m.cursor)
	}
}

func TestModel_arrowNavigation(t *testing.T) {
	m := withState(testModel())
	m = pressSpecial(m, tea.KeyDown)
	if m.cursor != 1 {
		t.Errorf("↓: cursor = %d, want 1", m.cursor)
	}
	m = pressSpecial(m, tea.KeyUp)
	if m.cursor != 0 {
		t.Errorf("↑: cursor = %d, want 0", m.cursor)
	}
}

// --- state updates ------------------------------------------------------

func TestModel_stateUpdates(t *testing.T) {
	m := withState(testModel())
	if !m.ready {
		t.Error("not ready after stateMsg")
	}
	if m.rows[0].status != statusRunning {
		t.Errorf("rows[0]: want running, got %v", m.rows[0].status)
	}
	if m.rows[2].status != statusAbsent {
		t.Errorf("rows[2]: want absent, got %v", m.rows[2].status)
	}
}

func TestModel_errClears(t *testing.T) {
	m := withState(testModel())
	m2, _ := m.Update(errMsg{fmt.Errorf("boom")})
	m = m2.(Model)
	if m.err == nil {
		t.Error("error should be set")
	}
	// next state poll clears it
	m3, _ := m.Update(stateMsg(m.rows))
	if m3.(Model).err != nil {
		t.Error("error should clear after stateMsg")
	}
}

// --- mutations ----------------------------------------------------------

func TestModel_startDispatchesCmd(t *testing.T) {
	m := withState(testModel())
	m.cursor = 2 // storybook is absent
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Error("s on absent service should dispatch a command")
	}
}

func TestModel_startIgnoredWhenRunning(t *testing.T) {
	m := withState(testModel())
	m.cursor = 0 // backend is running
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd != nil {
		t.Error("s on a running service should not dispatch a command")
	}
}

func TestModel_stopDispatchesCmd(t *testing.T) {
	m := withState(testModel())
	m.cursor = 0 // running
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd == nil {
		t.Error("x on running service should dispatch a command")
	}
}

func TestModel_stopIgnoredWhenAbsent(t *testing.T) {
	m := withState(testModel())
	m.cursor = 2 // absent
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if cmd != nil {
		t.Error("x on absent service should not dispatch a command")
	}
}

func TestModel_restartDispatchesCmd(t *testing.T) {
	m := withState(testModel())
	m.cursor = 1 // idle — has a window, so restart should work
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Error("r on idle service should dispatch a command")
	}
}

func TestModel_restartIgnoredWhenAbsent(t *testing.T) {
	m := withState(testModel())
	m.cursor = 2 // absent
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd != nil {
		t.Error("r on absent service should not dispatch a command")
	}
}

// effectiveWorktree falls back to svc.Repo when worktree is unset
func TestServiceRow_effectiveWorktree(t *testing.T) {
	svc := config.Service{Repo: "/r/backend"}
	row := serviceRow{svc: svc}
	if got, want := row.effectiveWorktree(), "/r/backend"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	row.worktree = "/work/backend-feat"
	if got, want := row.effectiveWorktree(), "/work/backend-feat"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- picker -------------------------------------------------------------

func testWorktrees() []git.Worktree {
	return []git.Worktree{
		{Path: "/work/backend", Branch: "main"},
		{Path: "/work/backend-feat", Branch: "feature/checkout"},
		{Path: "/work/backend-fix", Branch: "fix/auth"},
	}
}

func TestModel_enterOpensPicker(t *testing.T) {
	m := withState(testModel())
	m.cursor = 0
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next.(Model).picker == nil {
		t.Error("enter should open picker")
	}
}

func TestModel_wOpensPicker(t *testing.T) {
	m := withState(testModel())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if next.(Model).picker == nil {
		t.Error("w should open picker")
	}
}

func TestModel_pickerReceivesWorktrees(t *testing.T) {
	m := withState(testModel())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	// worktrees arrive
	m3, _ := m.Update(worktreesLoadedMsg{worktrees: testWorktrees()})
	m = m3.(Model)
	if m.picker == nil {
		t.Fatal("picker should still be open")
	}
	if m.picker.loading {
		t.Error("picker should not be loading after worktreesLoadedMsg")
	}
	if len(m.picker.worktrees) != 3 {
		t.Errorf("picker worktrees = %d, want 3", len(m.picker.worktrees))
	}
}

func TestModel_pickerEscCloses(t *testing.T) {
	m := withState(testModel())
	m, _ = applyMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(m, worktreesLoadedMsg{worktrees: testWorktrees()})
	m, _ = applyMsg(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.picker != nil {
		t.Error("esc should close picker")
	}
}

func TestModel_pickerSelectDispatchesSwitch(t *testing.T) {
	m := withState(testModel())
	m, _ = applyMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = applyMsg(m, worktreesLoadedMsg{worktrees: testWorktrees()})

	// Enter: picker marks selected, root detects it and returns cmdSwitch
	m2, cmd := applyMsg(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m2.picker != nil {
		t.Error("picker should be closed after selection")
	}
	if cmd == nil {
		t.Error("selection should dispatch cmdSwitch")
	}
}

func TestModel_pickerErrorClosesPickerAndSetsErr(t *testing.T) {
	m := withState(testModel())
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	m3, _ := m.Update(worktreesLoadedMsg{err: fmt.Errorf("git exploded")})
	m = m3.(Model)
	if m.picker != nil {
		t.Error("picker should close on error")
	}
	if m.err == nil {
		t.Error("err should be set")
	}
}

// --- picker model -------------------------------------------------------

func TestPickerNavigation(t *testing.T) {
	p := newPicker(config.Service{Name: "backend", Repo: "/r"}).
		withWorktrees(testWorktrees())

	down := func(p pickerModel) pickerModel {
		next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		return next
	}
	up := func(p pickerModel) pickerModel {
		next, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		return next
	}

	p = down(p)
	if p.cursor != 1 {
		t.Errorf("after j: cursor = %d, want 1", p.cursor)
	}
	p = down(p)
	p = down(p) // clamp
	if p.cursor != 2 {
		t.Errorf("past bottom: cursor = %d, want 2", p.cursor)
	}
	p = up(p)
	p = up(p)
	p = up(p) // clamp
	if p.cursor != 0 {
		t.Errorf("past top: cursor = %d, want 0", p.cursor)
	}
}

func TestPickerSelect(t *testing.T) {
	p := newPicker(config.Service{Name: "backend", Repo: "/r"}).
		withWorktrees(testWorktrees())

	next, _ := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if next.Selected == nil {
		t.Fatal("enter should set Selected")
	}
	if want := "/work/backend"; next.Selected.Path != want {
		t.Errorf("Selected.Path = %q, want %q", next.Selected.Path, want)
	}
}

func TestPickerEsc(t *testing.T) {
	p := newPicker(config.Service{Name: "backend", Repo: "/r"}).
		withWorktrees(testWorktrees())

	next, _ := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !next.Cancelled {
		t.Error("esc should set Cancelled")
	}
}

func TestPickerView_containsWorktreeLabels(t *testing.T) {
	p := newPicker(config.Service{Name: "backend", Repo: "/r"}).
		withWorktrees(testWorktrees())
	view := p.View()
	for _, label := range []string{"main", "feature/checkout", "fix/auth"} {
		if !strings.Contains(view, label) {
			t.Errorf("picker view missing label %q", label)
		}
	}
}

func TestPickerView_loadingState(t *testing.T) {
	p := newPicker(config.Service{Name: "backend", Repo: "/r"})
	if !strings.Contains(p.View(), "loading") {
		t.Error("loading state should mention loading")
	}
}

// --- helpers ------------------------------------------------------------

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

func TestTrimTrailingBlanks(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"a\nb\n\n\n", []string{"a", "b"}},
		{"a\n\nb\n", []string{"a", "", "b"}}, // internal blank preserved
		{"\n\n", nil},
		{"a", []string{"a"}},
	}
	for _, tc := range tests {
		got := trimTrailingBlanks(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("trimTrailingBlanks(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("[%d] got %q, want %q", i, got[i], tc.want[i])
			}
		}
	}
}

func TestPadToWidth(t *testing.T) {
	got := padToWidth("hi", 10)
	if lipgloss.Width(got) != 10 {
		t.Errorf("padToWidth visible width = %d, want 10", lipgloss.Width(got))
	}
	// Should not shrink strings that are already wide enough.
	long := strings.Repeat("x", 20)
	if padToWidth(long, 10) != long {
		t.Error("padToWidth should not truncate")
	}
}

func TestModel_splitViewContainsServiceNames(t *testing.T) {
	m := withState(testModel())
	m.width = 120
	m.height = 30
	view := m.View()
	for _, name := range []string{"backend", "web", "storybook"} {
		if !strings.Contains(view, name) {
			t.Errorf("split view missing service name %q", name)
		}
	}
	// Right panel header for the selected service (backend) should be visible.
	if !strings.Contains(view, "│") {
		t.Error("split view should contain the panel divider │")
	}
}

func TestLastNonEmptyLines(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want []string
	}{
		{"a\nb\nc\n\n\n", 2, []string{"b", "c"}},
		{"a\nb\n", 5, []string{"a", "b"}},
		{"\n\n\n", 3, nil},
		{"a\n  \nb\n", 3, []string{"a", "b"}},
	}
	for _, tc := range tests {
		got := lastNonEmptyLines(tc.in, tc.n)
		if len(got) != len(tc.want) {
			t.Errorf("lastNonEmptyLines(%q, %d) = %v, want %v", tc.in, tc.n, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("lastNonEmptyLines result[%d] = %q, want %q", i, got[i], tc.want[i])
			}
		}
	}
}

func TestTruncateLine(t *testing.T) {
	if got := truncateLine("hello", 10); got != "hello" {
		t.Errorf("short string modified: %q", got)
	}
	got := truncateLine("hello world", 8)
	if len([]rune(got)) != 8 {
		t.Errorf("truncated length = %d, want 8", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated string should end with …, got %q", got)
	}
}

func TestShellEscape(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"/simple/path", "'/simple/path'"},
		{"/path with spaces", "'/path with spaces'"},
		{"/path/with'quote", `'/path/with'\''quote'`},
	}
	for _, tc := range tests {
		if got := shellEscape(tc.in); got != tc.want {
			t.Errorf("shellEscape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- view ---------------------------------------------------------------

func TestModel_viewContainsServiceNames(t *testing.T) {
	m := withState(testModel())
	view := m.View()
	for _, name := range []string{"backend", "web", "storybook"} {
		if !strings.Contains(view, name) {
			t.Errorf("view missing %q", name)
		}
	}
}

func TestModel_viewShowsHelp(t *testing.T) {
	m := withState(testModel())
	view := m.View()
	for _, hint := range []string{"switch worktree", "start", "stop", "attach"} {
		if !strings.Contains(view, hint) {
			t.Errorf("help footer missing %q", hint)
		}
	}
}

func TestModel_viewNotReadyShowsLoading(t *testing.T) {
	if !strings.Contains(testModel().View(), "loading") {
		t.Error("pre-ready view should show loading")
	}
}

func TestModel_errorView(t *testing.T) {
	m := testModel()
	next, _ := m.Update(errMsg{err: fmt.Errorf("tmux exploded")})
	if !strings.Contains(next.(Model).View(), "tmux exploded") {
		t.Error("error view should contain the error message")
	}
}
