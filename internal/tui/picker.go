package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matthewtole/devtree/internal/config"
	"github.com/matthewtole/devtree/internal/git"
)

// pickerModel handles the worktree selection overlay.
// After Update, the caller should check Cancelled and Selected.
type pickerModel struct {
	svc       config.Service
	worktrees []git.Worktree
	cursor    int
	loading   bool
	Cancelled bool
	Selected  *git.Worktree // non-nil when the user confirmed a choice
}

func newPicker(svc config.Service) pickerModel {
	return pickerModel{svc: svc, loading: true}
}

func (p pickerModel) withWorktrees(wts []git.Worktree) pickerModel {
	p.worktrees = wts
	p.loading = false
	return p
}

func (p pickerModel) Update(msg tea.Msg) (pickerModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc", "q":
			p.Cancelled = true
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
			}
		case "down", "j":
			if !p.loading && p.cursor < len(p.worktrees)-1 {
				p.cursor++
			}
		case "enter":
			if !p.loading && len(p.worktrees) > 0 {
				wt := p.worktrees[p.cursor]
				p.Selected = &wt
			}
		}
	}
	return p, nil
}

func (p pickerModel) View() string {
	var b strings.Builder

	b.WriteString("\n  " + styleTitle.Render("switch worktree") + styleHeaderRow.Render("  →  "+p.svc.Name) + "\n\n")

	if p.loading {
		b.WriteString("  " + styleHelp.Render("loading worktrees…") + "\n")
		b.WriteString("\n  " + styleHelp.Render("esc  cancel") + "\n")
		return b.String()
	}

	if len(p.worktrees) == 0 {
		b.WriteString("  " + styleIdle.Render("no worktrees found") + "\n")
		b.WriteString("\n  " + styleHelp.Render("esc  cancel") + "\n")
		return b.String()
	}

	maxLabel := 0
	for _, w := range p.worktrees {
		if l := len(w.Label()); l > maxLabel {
			maxLabel = l
		}
	}

	b.WriteString(styleHeaderRow.Render(fmt.Sprintf("  %-*s  %s", maxLabel+2, "branch/tag", "path")) + "\n")
	b.WriteString(styleDivider.Render("  "+strings.Repeat("─", maxLabel+40)) + "\n")

	for i, w := range p.worktrees {
		var cursor string
		if i == p.cursor {
			cursor = styleCursor.Render("▸") + " "
		} else {
			cursor = "  "
		}

		label := fmt.Sprintf("%-*s", maxLabel, w.Label())
		path := styleHelp.Render(shortenPath(w.Path))
		line := cursor + label + "  " + path

		if i == p.cursor {
			b.WriteString(styleSelected.Render(line))
		} else {
			b.WriteString(styleNormal.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n  " + styleHelp.Render("↑/k  ↓/j  navigate    ↵  select    esc  cancel") + "\n")
	return b.String()
}

// shortenPath returns the last two path components for display.
func shortenPath(path string) string {
	base := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	if parent == "." || parent == "/" {
		return base
	}
	return "…/" + parent + "/" + base
}
