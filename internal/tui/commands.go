package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matthewtole/devtree/internal/git"
	"github.com/matthewtole/devtree/internal/tmux"
)

// --- result messages ----------------------------------------------------

type startedMsg struct{ window string }
type stoppedMsg struct{ window string }
type switchedMsg struct {
	window   string
	worktree string
}

// worktreesLoadedMsg is sent when the async git worktree list completes.
type worktreesLoadedMsg struct {
	worktrees []git.Worktree
	err       error
}

// --- commands -----------------------------------------------------------

// cmdStart creates the session/window if needed and runs the service command
// in the given worktree. Safe to call when the window already exists at an
// idle shell prompt — it just sends the command.
func cmdStart(tc *tmux.Client, session, window, command, worktreePath string) tea.Cmd {
	return func() tea.Msg {
		if err := tc.EnsureSession(session); err != nil {
			return errMsg{err}
		}
		if err := tc.EnsureWindow(session, window); err != nil {
			return errMsg{err}
		}
		keys := fmt.Sprintf("cd %s && clear && %s", shellEscape(worktreePath), command)
		if err := tc.SendKeys(session, window, keys, "C-m"); err != nil {
			return errMsg{err}
		}
		if err := tc.SetUserOption(session, window, "worktree", worktreePath); err != nil {
			return errMsg{err}
		}
		return startedMsg{window: window}
	}
}

// cmdStop sends C-c twice with a short pause and lets the next poll confirm
// the process actually stopped.
func cmdStop(tc *tmux.Client, session, window string) tea.Cmd {
	return func() tea.Msg {
		_ = tc.SendKeys(session, window, "C-c")
		time.Sleep(200 * time.Millisecond)
		_ = tc.SendKeys(session, window, "C-c")
		return stoppedMsg{window: window}
	}
}

// cmdRestart stops then immediately starts in the same worktree.
func cmdRestart(tc *tmux.Client, session, window, command, worktreePath string) tea.Cmd {
	return func() tea.Msg {
		_ = tc.SendKeys(session, window, "C-c")
		time.Sleep(300 * time.Millisecond)
		_ = tc.SendKeys(session, window, "C-c")
		time.Sleep(200 * time.Millisecond)

		keys := fmt.Sprintf("cd %s && clear && %s", shellEscape(worktreePath), command)
		if err := tc.SendKeys(session, window, keys, "C-m"); err != nil {
			return errMsg{err}
		}
		if err := tc.SetUserOption(session, window, "worktree", worktreePath); err != nil {
			return errMsg{err}
		}
		return startedMsg{window: window}
	}
}

// cmdSwitch kills whatever is running and starts the service in the new
// worktree. Handles the running and idle cases uniformly — a no-op C-c on
// an idle shell is harmless.
func cmdSwitch(tc *tmux.Client, session, window, command, worktreePath string) tea.Cmd {
	return func() tea.Msg {
		_ = tc.SendKeys(session, window, "C-c")
		time.Sleep(300 * time.Millisecond)
		_ = tc.SendKeys(session, window, "C-c")
		time.Sleep(200 * time.Millisecond)

		keys := fmt.Sprintf("cd %s && clear && %s", shellEscape(worktreePath), command)
		if err := tc.SendKeys(session, window, keys, "C-m"); err != nil {
			return errMsg{err}
		}
		if err := tc.SetUserOption(session, window, "worktree", worktreePath); err != nil {
			return errMsg{err}
		}
		return switchedMsg{window: window, worktree: worktreePath}
	}
}

// cmdAttach focuses the tmux window containing the service.
func cmdAttach(tc *tmux.Client, session, window string) tea.Cmd {
	return func() tea.Msg {
		if err := tc.Attach(session, window); err != nil {
			return errMsg{err}
		}
		return nil
	}
}

// cmdLoadWorktrees fetches the worktree list for a repo asynchronously.
func cmdLoadWorktrees(repoPath string) tea.Cmd {
	return func() tea.Msg {
		wts, err := git.ListWorktrees(repoPath)
		return worktreesLoadedMsg{worktrees: wts, err: err}
	}
}

// shellEscape single-quotes a string for safe use in a shell command,
// handling embedded single quotes via the '\'' idiom.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
