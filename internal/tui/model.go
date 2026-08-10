// Package tui implements the devtree interactive terminal UI.
package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/matthewtole/devtree/internal/config"
	"github.com/matthewtole/devtree/internal/tmux"
)

const (
	SessionName  = "devtree"
	pollInterval = 500 * time.Millisecond

	// leftPanelWidth is the fixed width of the service list panel.
	leftPanelWidth = 40
	// divider is the string placed between the two panels.
	divider = " │ "
)

// These are kept for the table layout inside the picker, which is still a
// single-column view.
const (
	colService  = 18
	colWorktree = 30
)

type serviceStatus int

const (
	statusUnknown serviceStatus = iota
	statusRunning               // pane has a non-shell foreground process
	statusIdle                  // pane is at a shell prompt
	statusAbsent                // window doesn't exist in tmux yet
)

func (s serviceStatus) dot() string {
	switch s {
	case statusRunning:
		return styleRunning.Render("●")
	case statusIdle:
		return styleIdle.Render("○")
	case statusAbsent:
		return styleAbsent.Render("◌")
	default:
		return styleUnknown.Render("?")
	}
}

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
	command  string   // current pane_current_command
	snippet  []string // lines from capture-pane, trailing blanks stripped
}

func (r serviceRow) worktreeLabel() string {
	if r.worktree == "" {
		return styleIdle.Render("—")
	}
	return filepath.Base(r.worktree)
}

func (r serviceRow) effectiveWorktree() string {
	if r.worktree != "" {
		return r.worktree
	}
	return r.svc.Repo
}

// Model is the root bubbletea model.
type Model struct {
	rows    []serviceRow
	cursor  int
	tc      *tmux.Client
	ready   bool
	err     error
	width   int
	height  int
	picker  *pickerModel
	version string
	vp      viewport.Model
	spin    spinner.Model
}

func New(cfg *config.Config, version string) Model {
	rows := make([]serviceRow, len(cfg.Services))
	for i, s := range cfg.Services {
		rows[i] = serviceRow{svc: s, status: statusUnknown}
	}

	vp := viewport.New(44, 16) // dimensions updated on WindowSizeMsg
	vp.KeyMap = viewportKeyMap()

	spin := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(styleSpinner),
	)

	return Model{rows: rows, tc: &tmux.Client{}, version: version, vp: vp, spin: spin}
}

// viewportKeyMap returns a KeyMap that uses only PageUp/PageDown for scrolling,
// leaving j/k/up/down free for service-list navigation.
func viewportKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithKeys("pgdown")),
		PageUp:       key.NewBinding(key.WithKeys("pgup")),
		HalfPageUp:   key.NewBinding(key.WithKeys()),
		HalfPageDown: key.NewBinding(key.WithKeys()),
		Up:           key.NewBinding(key.WithKeys()),
		Down:         key.NewBinding(key.WithKeys()),
		Left:         key.NewBinding(key.WithKeys()),
		Right:        key.NewBinding(key.WithKeys()),
	}
}

// --- tea messages -------------------------------------------------------

type tickMsg struct{}
type stateMsg []serviceRow
type errMsg struct{ err error }

// --- tea.Model ----------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return tea.Batch(cmdPoll(m.tc, m.rows, 0), cmdTick(), m.spin.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// tickMsg and stateMsg must be handled before the picker block so the
	// poll/render cycle keeps running even while the picker is open.
	switch msg := msg.(type) {
	case tickMsg:
		return m, tea.Batch(cmdPoll(m.tc, m.rows, m.height), cmdTick())
	case stateMsg:
		m.rows = []serviceRow(msg)
		m.ready = true
		m.err = nil
		atBottom := m.vp.AtBottom()
		m.syncViewport(atBottom)
		return m, nil
	case spinner.TickMsg:
		if !m.ready {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil
	}

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
		contentH := m.height - 5
		if contentH < 5 || m.height == 0 {
			contentH = 18
		}
		rightW := m.width - leftPanelWidth - len(divider)
		if rightW < 20 || m.width == 0 {
			rightW = 44
		}
		m.vp.Width = rightW
		m.vp.Height = contentH - 2
		if m.ready {
			m.syncViewport(true)
			return m, cmdResizeWindows(m.tc, SessionName, m.rows, rightW)
		}

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.syncViewport(true)
			}
		case "down", "j":
			if m.cursor < len(m.rows)-1 {
				m.cursor++
				m.syncViewport(true)
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
		// Forward remaining key events to the viewport (handles pgup/pgdn).
		var vpCmd tea.Cmd
		m.vp, vpCmd = m.vp.Update(msg)
		return m, vpCmd

	case errMsg:
		m.err = msg.err

	case startedMsg, stoppedMsg:

	case switchedMsg:
		for i, row := range m.rows {
			if row.svc.Name == msg.window {
				m.rows[i].worktree = msg.worktree
				break
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if m.err != nil {
		return "\n  " + styleError.Render("error: "+m.err.Error()) + "\n\n  press q to quit\n"
	}
	if !m.ready {
		return "\n  " + m.spin.View() + " loading…\n"
	}
	if m.picker != nil {
		return m.picker.View()
	}
	return m.mainView()
}

// --- split layout -------------------------------------------------------

func (m Model) mainView() string {
	// 3 lines: "\n  devtree\n\n" before panels; 2 lines: "\n  help\n" after.
	contentH := m.height - 5
	if contentH < 5 || m.height == 0 {
		contentH = 18 // sensible fallback before WindowSizeMsg arrives
	}

	rightW := m.width - leftPanelWidth - len(divider)
	if rightW < 20 || m.width == 0 {
		rightW = 44
	}

	leftLines := m.buildLeftPanel(contentH, leftPanelWidth)
	rightLines := m.buildRightPanel(contentH, rightW)

	var b strings.Builder
	b.WriteString("\n  " + styleTitle.Render("devtree") + "  " + styleVersion.Render("v"+m.version) + "\n\n")
	for i := 0; i < contentH; i++ {
		l := padToWidth(leftLines[i], leftPanelWidth)
		r := rightLines[i]
		b.WriteString(l + divider + r + "\n")
	}
	b.WriteString("\n")
	b.WriteString("  " + styleHelp.Render(
		"↑/k  ↓/j  navigate    ↵/w  switch worktree    s  start    x  stop    r  restart    a  attach    pgup/pgdn  scroll    q  quit",
	) + "\n")
	return b.String()
}

func (m Model) buildLeftPanel(height, width int) []string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = ""
	}
	if height < 2 {
		return lines
	}

	lines[0] = styleHeaderRow.Render("  services")
	lines[1] = styleDivider.Render("  " + strings.Repeat("─", width-2))

	// Layout: cursor(2) + name(20) + space(1) + worktree(14) + space(1) + dot(1) = 39.
	const nameW = 20
	const wtW = 14

	for i, row := range m.rows {
		li := i + 2
		if li >= height {
			break
		}

		name := fmt.Sprintf("%-*s", nameW, truncateLine(row.svc.Name, nameW))
		var wtText string
		if row.worktree != "" {
			wtText = truncateLine(filepath.Base(row.worktree), wtW)
		}
		wtPadded := fmt.Sprintf("%-*s", wtW, wtText)
		dot := row.status.dot()

		if i == m.cursor {
			// Build the highlighted region without nested lipgloss renders so the
			// background colour covers the full row width without gaps from inner
			// \x1b[0m resets. The status dot is placed outside the highlight to
			// preserve its semantic colour.
			prefix := padToWidth("▸ "+name+" "+wtPadded+" ", width-1)
			lines[li] = styleSelectedRow.Render(prefix) + dot
		} else {
			wt := styleIdle.Render(wtPadded)
			lines[li] = "  " + name + " " + wt + " " + dot
		}
	}
	return lines
}

func (m Model) buildRightPanel(height, width int) []string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = ""
	}
	if height < 2 || len(m.rows) == 0 {
		return lines
	}

	sel := m.rows[m.cursor]

	// Header: service name, optional worktree, scroll position when not following.
	header := styleSelected.Render(sel.svc.Name)
	if sel.worktree != "" {
		header += styleHeaderRow.Render("  " + filepath.Base(sel.worktree))
	}
	if !m.vp.AtBottom() {
		pct := int(m.vp.ScrollPercent() * 100)
		header += styleHelp.Render(fmt.Sprintf("  ↑ %d%%  pgup/pgdn", pct))
	}
	lines[0] = header
	lines[1] = styleDivider.Render(strings.Repeat("─", width))

	// Split the viewport view into individual lines for the side-by-side layout.
	vpLines := strings.Split(m.vp.View(), "\n")
	for i, line := range vpLines {
		if 2+i >= height {
			break
		}
		lines[2+i] = line
	}
	return lines
}

// --- viewport helpers ---------------------------------------------------

// syncViewport refreshes the viewport's content from the currently selected
// service. If follow is true the view jumps to the bottom (tail mode); if
// false the current scroll offset is preserved so the user can read history.
func (m *Model) syncViewport(follow bool) {
	if len(m.rows) == 0 {
		return
	}
	sel := m.rows[m.cursor]
	var sb strings.Builder
	switch {
	case sel.status == statusAbsent:
		sb.WriteString(styleIdle.Render("not started"))
	case len(sel.snippet) == 0:
		sb.WriteString(styleIdle.Render("no output"))
	default:
		for i, line := range sel.snippet {
			if i > 0 {
				sb.WriteByte('\n')
			}
			// Per-line reset prevents colour from one log line bleeding into the next.
			sb.WriteString(line + "\x1b[0m")
		}
	}
	m.vp.SetContent(sb.String())
	if follow {
		m.vp.GotoBottom()
	}
}

// --- poll ---------------------------------------------------------------

func cmdTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func cmdPoll(tc *tmux.Client, rows []serviceRow, termHeight int) tea.Cmd {
	// Request enough lines to fill the log panel plus headroom.
	logLines := termHeight - 7
	if logLines < 50 {
		logLines = 50
	}

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
				updated.snippet = nil
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

			if content, err := tc.CapturePaneN(SessionName, row.svc.Name, logLines); err == nil {
				updated.snippet = trimTrailingBlanks(content)
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

func isShell(cmd string) bool { return shellNames[cmd] }

// --- helpers ------------------------------------------------------------

// trimTrailingBlanks splits s into lines and removes only the trailing empty
// ones, preserving internal blank lines that are part of log structure.
// ANSI escape sequences are stripped before the blank check so that a line
// containing only colour codes (e.g. a reset) is treated as blank.
func trimTrailingBlanks(s string) []string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// lastNonEmptyLines returns the last n lines that are not blank.
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

// truncateLine clips s to max visible columns, appending … if trimmed.
// ANSI escape sequences are counted as zero-width so colour is preserved.
func truncateLine(s string, max int) string {
	return ansi.Truncate(s, max, "…")
}

// padToWidth pads s with spaces to reach the target visible width,
// accounting for ANSI escape sequences already in s.
func padToWidth(s string, width int) string {
	vis := lipgloss.Width(s)
	if vis >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vis)
}
