package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matthewtole/devtree/internal/config"
	"github.com/matthewtole/devtree/internal/tui"
)

const usage = `devtree — control which git worktree your dev servers run in

Usage:
  devtree              launch the TUI
  devtree config check validate the config file and print parsed services
  devtree help         show this message
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "devtree:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runTUI()
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "config":
		return runConfig(args[1:])
	default:
		return fmt.Errorf("unknown command %q (try `devtree help`)", args[0])
	}
}

func runTUI() error {
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	p := tea.NewProgram(tui.New(cfg), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func runConfig(args []string) error {
	if len(args) == 0 || args[0] != "check" {
		return fmt.Errorf("usage: devtree config check")
	}

	path, err := config.DefaultPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	fmt.Printf("loaded %d service(s) from %s\n\n", len(cfg.Services), path)
	for _, s := range cfg.Services {
		fmt.Printf("  %s\n", s.Name)
		fmt.Printf("    repo:    %s\n", s.Repo)
		fmt.Printf("    command: %s\n\n", s.Command)
	}
	return nil
}
