package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/matthewtole/devtree/internal/config"
	"github.com/matthewtole/devtree/internal/tui"
)

// Version is the current release. Set at build time via
// -ldflags "-X main.Version=x.y.z".
var Version = "dev"

const usage = `devtree — control which git worktree your dev servers run in

Usage:
  devtree              launch the TUI
  devtree service add  add a new service (interactive)
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
	case "service":
		return runService(args[1:])
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
	p := tea.NewProgram(tui.New(cfg, Version), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

func runService(args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return fmt.Errorf("usage: devtree service add")
	}
	return runServiceAdd()
}

func runServiceAdd() error {
	sc := bufio.NewScanner(os.Stdin)

	name, err := promptLine(sc, "Service name: ", "")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("service name is required")
	}

	cwd, _ := os.Getwd()
	repo, err := promptLine(sc, "Repository path: ", cwd)
	if err != nil {
		return err
	}
	if repo == "" {
		return fmt.Errorf("repository path is required")
	}
	if !filepath.IsAbs(repo) {
		return fmt.Errorf("repository path must be absolute, got %q", repo)
	}

	command, err := promptLine(sc, "Command: ", "")
	if err != nil {
		return err
	}
	if command == "" {
		return fmt.Errorf("command is required")
	}

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}

	existing, err := config.LoadServices(cfgPath)
	if err != nil {
		return err
	}
	for _, s := range existing {
		if s.Name == name {
			return fmt.Errorf("service %q already exists in %s", name, cfgPath)
		}
	}

	if err := config.AppendService(cfgPath, config.Service{
		Name:    name,
		Repo:    repo,
		Command: command,
	}); err != nil {
		return err
	}

	fmt.Printf("Added service %q to %s\n", name, cfgPath)
	return nil
}

// promptLine prints label (with default in brackets if non-empty), reads a
// line from sc, and returns the trimmed input or def if the input is blank.
func promptLine(sc *bufio.Scanner, label, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s[%s] ", label, def)
	} else {
		fmt.Print(label)
	}
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return "", err
		}
		return def, nil // EOF — accept the default
	}
	if line := strings.TrimSpace(sc.Text()); line != "" {
		return line, nil
	}
	return def, nil
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
