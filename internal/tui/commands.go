package tui

import (
	"fmt"
	"os"
	"os/exec"
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
// worktree. Uses respawn-pane -k to atomically kill the current process and
// get a clean shell, which is more reliable than timed C-c signals.
// Only call this when the window already exists (running or idle).
func cmdSwitch(tc *tmux.Client, session, window, command, worktreePath string) tea.Cmd {
	return func() tea.Msg {
		if err := tc.RespawnPane(session, window); err != nil {
			return errMsg{err}
		}
		time.Sleep(100 * time.Millisecond)

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
//
// switch-client and attach-session both need to inspect the calling process's
// controlling terminal to find the tmux client. Our normal run() method
// captures stdin into a buffer, which makes tmux report "not a terminal".
// tea.ExecProcess suspends the TUI and gives the subprocess the real terminal,
// so tmux can identify the client and switch immediately.
func cmdAttach(tc *tmux.Client, session, window string) tea.Cmd {
	target := session + ":" + window

	var args []string
	if tc.Socket != "" {
		args = append(args, "-L", tc.Socket)
	}
	if os.Getenv("TMUX") != "" {
		args = append(args, "switch-client", "-t", target)
	} else {
		args = append(args, "attach-session", "-t", target)
	}

	return tea.ExecProcess(exec.Command("tmux", args...), func(err error) tea.Msg {
		if err != nil {
			return errMsg{err}
		}
		return nil
	})
}

// cmdResizeWindows sets all existing tmux windows to the given width so that
// capture-pane returns lines formatted at the viewport's actual width.
func cmdResizeWindows(tc *tmux.Client, session string, rows []serviceRow, width int) tea.Cmd {
	snapshot := make([]serviceRow, len(rows))
	copy(snapshot, rows)
	return func() tea.Msg {
		for _, row := range snapshot {
			if row.status != statusAbsent {
				_ = tc.ResizeWindow(session, row.svc.Name, width)
			}
		}
		return nil
	}
}

// cmdLoadWorktrees fetches the worktree list for a repo asynchronously via
// the shared git service, so it benefits from the same cache as the poll.
func cmdLoadWorktrees(gitSvc *git.Service, repoPath string) tea.Cmd {
	return func() tea.Msg {
		wts, err := gitSvc.Worktrees(repoPath)
		return worktreesLoadedMsg{worktrees: wts, err: err}
	}
}

// shellEscape single-quotes a string for safe use in a shell command,
// handling embedded single quotes via the '\'' idiom.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
