package tui

import "github.com/charmbracelet/lipgloss"

var (
	styleTitle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99")).Underline(true)
	styleVersion     = lipgloss.NewStyle().Faint(true)
	styleHeaderRow   = lipgloss.NewStyle().Faint(true)
	styleDivider     = lipgloss.NewStyle().Faint(true)
	styleSelected    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	styleSelectedRow = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("255")).Bold(true)
	styleNormal      = lipgloss.NewStyle()
	styleHelp        = lipgloss.NewStyle().Faint(true).Italic(true)
	styleError       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleSpinner     = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	styleCursor      = lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)

	styleRunning  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleIdle     = lipgloss.NewStyle().Faint(true)
	styleAbsent   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleUnknown  = lipgloss.NewStyle().Faint(true)
)
