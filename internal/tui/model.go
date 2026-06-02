// Package tui implements the devtree interactive terminal UI.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matthewtole/devtree/internal/config"
	"github.com/matthewtole/devtree/internal/tmux"
)

const (
	SessionName  = "devtree"
	pollInterval = 500 * time.Millisecond

	colService  = 18
	colWorktree = 30
)

type serviceStatus int

const (
	statusUnknown  serviceStatus = iota
	statusRunning                // pane has a non-shell foreground process
	statusIdle                   // pane is at a shell prompt
	statusAbsent                 // window doesn't exist in tmux yet
)

func (s serviceStatus) String() string {
	switch s {
	case statusRunning:
		return styleRunning.Render("● running")
	case statusIdle:
		return styleIdle.Render("○ idle")
	case statusAbsent:
		return styleAbsent.Render("◌ not started")
	default:
		return styleUnknown.Render("? …")
	}
}

type serviceRow struct {
	svc      config.Service
	worktree string        // full path stored in tmux user option; empty = unset
	status   serviceStatus
	command  string        // current pane_current_command
}

// worktreeLabel returns a short label for the worktree column.
func (r serviceRow) worktreeLabel() string {
	if r.worktree == "" {
		return styleIdle.Render("—")
	}
	return filepath.Base(r.worktree)
}

// Model is the root bubbletea model.
type Model struct {
	rows   []serviceRow
	cursor int
	tc     *tmux.Client
	ready  bool
	err    error
	width  int
	height int
}

// New returns a Model initialised from cfg. It uses the default tmux server.
func New(cfg *config.Config) Model {
	rows := make([]serviceRow, len(cfg.Services))
	for i, s := range cfg.Services {
		rows[i] = serviceRow{svc: s, status: statusUnknown}
	}
	return Model{rows: rows, tc: &tmux.Client{}}
}

// --- tea messages -------------------------------------------------------

type tickMsg struct{}
type stateMsg []serviceRow
type errMsg struct{ err error }

// --- tea.Model ----------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return tea.Batch(cmdPoll(m.tc, m.rows), cmdTick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		}

	case stateMsg:
		m.rows = []serviceRow(msg)
		m.ready = true
		m.err = nil

	case errMsg:
		m.err = msg.err

	case tickMsg:
		return m, tea.Batch(cmdPoll(m.tc, m.rows), cmdTick())
	}

	return m, nil
}

func (m Model) View() string {
	if m.err != nil {
		return "\n  " + styleError.Render("error: "+m.err.Error()) + "\n\n  press q to quit\n"
	}
	if !m.ready {
		return "\n  loading…\n"
	}

	var b strings.Builder

	b.WriteString("\n  " + styleTitle.Render("devtree") + "\n\n")

	// Header
	header := fmt.Sprintf("  %-*s  %-*s  %s", colService, "service", colWorktree, "worktree", "status")
	b.WriteString(styleHeaderRow.Render(header) + "\n")
	b.WriteString(styleDivider.Render("  "+strings.Repeat("─", colService+colWorktree+16)) + "\n")

	for i, row := range m.rows {
		var cursor string
		if i == m.cursor {
			cursor = styleCursor.Render("▸") + " "
		} else {
			cursor = "  "
		}

		wt := fmt.Sprintf("%-*s", colWorktree, row.worktreeLabel())
		svc := fmt.Sprintf("%-*s", colService, row.svc.Name)
		line := cursor + svc + "  " + wt + "  " + row.status.String()

		if i == m.cursor {
			b.WriteString(styleSelected.Render(line))
		} else {
			b.WriteString(styleNormal.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString("  " + styleHelp.Render("↑/k  ↓/j  navigate    q  quit") + "\n")

	return b.String()
}

// --- commands -----------------------------------------------------------

func cmdTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func cmdPoll(tc *tmux.Client, rows []serviceRow) tea.Cmd {
	// Capture a snapshot of the current rows so the goroutine works on
	// immutable data and the model can be updated freely in the meantime.
	snapshot := make([]serviceRow, len(rows))
	copy(snapshot, rows)

	return func() tea.Msg {
		result := make([]serviceRow, len(snapshot))
		for i, row := range snapshot {
			updated := row

			has, err := tc.HasWindow(SessionName, row.svc.Name)
			if err != nil {
				return errMsg{err}
			}
			if !has {
				updated.status = statusAbsent
				updated.worktree = ""
				result[i] = updated
				continue
			}

			cmd, err := tc.PaneCurrentCommand(SessionName, row.svc.Name)
			if err != nil {
				return errMsg{err}
			}
			updated.command = cmd
			if isShell(cmd) {
				updated.status = statusIdle
			} else {
				updated.status = statusRunning
			}

			wt, err := tc.GetUserOption(SessionName, row.svc.Name, "worktree")
			if err != nil {
				return errMsg{err}
			}
			updated.worktree = wt
			result[i] = updated
		}
		return stateMsg(result)
	}
}

var shellNames = map[string]bool{
	"bash": true, "zsh": true, "fish": true, "sh":   true,
	"dash": true, "ksh": true, "tcsh": true, "csh":  true,
}

func isShell(cmd string) bool {
	return shellNames[cmd]
}
