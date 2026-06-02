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
	snippet  []string      // last few lines of pane output
}

// worktreeLabel returns a short label for the worktree column.
func (r serviceRow) worktreeLabel() string {
	if r.worktree == "" {
		return styleIdle.Render("—")
	}
	return filepath.Base(r.worktree)
}

// effectiveWorktree returns the worktree to use for start/restart — the
// stored one if set, otherwise the service's repo root.
func (r serviceRow) effectiveWorktree() string {
	if r.worktree != "" {
		return r.worktree
	}
	return r.svc.Repo
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
	picker *pickerModel
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
	// When the picker is open, intercept worktree-load results and route
	// everything else through it. Check Cancelled/Selected synchronously
	// after each update so there's no extra message round-trip.
	if m.picker != nil {
		if wt, ok := msg.(worktreesLoadedMsg); ok {
			if wt.err != nil {
				m.picker = nil
				m.err = wt.err
				return m, nil
			}
			p := m.picker.withWorktrees(wt.worktrees)
			m.picker = &p
			return m, nil
		}

		p, cmd := m.picker.Update(msg)
		if p.Cancelled {
			m.picker = nil
			return m, nil
		}
		if p.Selected != nil {
			row := m.rows[m.cursor]
			m.picker = nil
			return m, cmdSwitch(m.tc, SessionName, row.svc.Name, row.svc.Command, p.Selected.Path)
		}
		m.picker = &p
		return m, cmd
	}

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

		case "enter", "w":
			if m.ready && len(m.rows) > 0 {
				p := newPicker(m.rows[m.cursor].svc)
				m.picker = &p
				return m, cmdLoadWorktrees(m.rows[m.cursor].svc.Repo)
			}

		case "s":
			if m.ready && len(m.rows) > 0 {
				row := m.rows[m.cursor]
				if row.status != statusRunning {
					return m, cmdStart(m.tc, SessionName, row.svc.Name,
						row.svc.Command, row.effectiveWorktree())
				}
			}

		case "x":
			if m.ready && len(m.rows) > 0 {
				row := m.rows[m.cursor]
				if row.status == statusRunning || row.status == statusIdle {
					return m, cmdStop(m.tc, SessionName, row.svc.Name)
				}
			}

		case "r":
			if m.ready && len(m.rows) > 0 {
				row := m.rows[m.cursor]
				if row.status != statusAbsent {
					return m, cmdRestart(m.tc, SessionName, row.svc.Name,
						row.svc.Command, row.effectiveWorktree())
				}
			}

		case "a":
			if m.ready && len(m.rows) > 0 {
				row := m.rows[m.cursor]
				if row.status != statusAbsent {
					return m, cmdAttach(m.tc, SessionName, row.svc.Name)
				}
			}
		}

	case stateMsg:
		m.rows = []serviceRow(msg)
		m.ready = true
		m.err = nil

	case errMsg:
		m.err = msg.err

	// Mutation result messages don't need to update model state directly —
	// the next poll tick will reflect the new tmux state.
	case startedMsg, stoppedMsg, switchedMsg:

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

	if m.picker != nil {
		return m.picker.View()
	}

	return m.mainView()
}

func (m Model) mainView() string {
	divWidth := colService + colWorktree + 16
	var b strings.Builder

	b.WriteString("\n  " + styleTitle.Render("devtree") + "\n\n")

	// Service table
	header := fmt.Sprintf("  %-*s  %-*s  %s", colService, "service", colWorktree, "worktree", "status")
	b.WriteString(styleHeaderRow.Render(header) + "\n")
	b.WriteString(styleDivider.Render("  "+strings.Repeat("─", divWidth)) + "\n")

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

	// Output panel for selected service
	b.WriteString("\n")
	if len(m.rows) > 0 {
		sel := m.rows[m.cursor]
		label := sel.svc.Name
		dashes := strings.Repeat("─", max(0, divWidth-len(label)-5))
		b.WriteString(styleDivider.Render("  ─── "+label+" "+dashes) + "\n")

		maxWidth := m.width - 4 // 2-char indent + 2 margin
		if maxWidth < 20 {
			maxWidth = 76
		}

		if sel.status == statusAbsent {
			b.WriteString("  " + styleIdle.Render("not started") + "\n")
		} else if len(sel.snippet) == 0 {
			b.WriteString("  " + styleIdle.Render("no output") + "\n")
		} else {
			for _, line := range sel.snippet {
				b.WriteString("  " + styleNormal.Render(truncateLine(line, maxWidth)) + "\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString("  " + styleHelp.Render(
		"↑/k  ↓/j  navigate    ↵/w  switch worktree    s  start    x  stop    r  restart    a  attach    q  quit",
	) + "\n")

	return b.String()
}

// lastNonEmptyLines returns the last n non-empty lines from s.
func lastNonEmptyLines(s string, n int) []string {
	all := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var lines []string
	for _, l := range all {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// truncateLine clips s to max runes, appending … if trimmed.
func truncateLine(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// --- poll ---------------------------------------------------------------

func cmdTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func cmdPoll(tc *tmux.Client, rows []serviceRow) tea.Cmd {
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

			if content, err := tc.CapturePane(SessionName, row.svc.Name); err == nil {
				updated.snippet = lastNonEmptyLines(content, 5)
			}

			result[i] = updated
		}
		return stateMsg(result)
	}
}

var shellNames = map[string]bool{
	"bash": true, "zsh": true, "fish": true, "sh":  true,
	"dash": true, "ksh": true, "tcsh": true, "csh": true,
}

func isShell(cmd string) bool {
	return shellNames[cmd]
}
