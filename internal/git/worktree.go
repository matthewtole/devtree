// Package git wraps the subset of `git` we need: listing worktrees.
package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type Worktree struct {
	Path     string
	HEAD     string
	Branch   string // short name, empty if Detached or Bare
	Detached bool
	Bare     bool
	Locked   bool
}

// Label returns a short, human-friendly identifier for the worktree.
func (w Worktree) Label() string {
	switch {
	case w.Bare:
		return "(bare)"
	case w.Detached:
		short := w.HEAD
		if len(short) > 8 {
			short = short[:8]
		}
		return short + " (detached)"
	default:
		return w.Branch
	}
}

// ListWorktrees shells out to `git -C repoPath worktree list --porcelain` and parses the result.
func ListWorktrees(repoPath string) ([]Worktree, error) {
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("git worktree list (%s): %s", repoPath, msg)
	}
	return parseWorktreeList(stdout.Bytes())
}

func parseWorktreeList(porcelain []byte) ([]Worktree, error) {
	var (
		out     []Worktree
		current *Worktree
	)
	flush := func() {
		if current != nil {
			out = append(out, *current)
			current = nil
		}
	}

	scanner := bufio.NewScanner(bytes.NewReader(porcelain))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if current == nil {
			current = &Worktree{}
		}
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			current.Path = value
		case "HEAD":
			current.HEAD = value
		case "branch":
			current.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "detached":
			current.Detached = true
		case "bare":
			current.Bare = true
		case "locked":
			current.Locked = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan porcelain output: %w", err)
	}
	flush()
	return out, nil
}
