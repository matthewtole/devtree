package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

func TestParseWorktreeList(t *testing.T) {
	src := `worktree /repo/main
HEAD aaaaaaaa
branch refs/heads/main

worktree /repo/feature
HEAD bbbbbbbb
branch refs/heads/feature/checkout

worktree /repo/bare
bare

worktree /repo/detached
HEAD 0123456789abcdef
detached

worktree /repo/locked
HEAD cccccccc
branch refs/heads/lockme
locked

`
	got, err := parseWorktreeList([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5; got %+v", len(got), got)
	}

	want := []Worktree{
		{Path: "/repo/main", HEAD: "aaaaaaaa", Branch: "main"},
		{Path: "/repo/feature", HEAD: "bbbbbbbb", Branch: "feature/checkout"},
		{Path: "/repo/bare", Bare: true},
		{Path: "/repo/detached", HEAD: "0123456789abcdef", Detached: true},
		{Path: "/repo/locked", HEAD: "cccccccc", Branch: "lockme", Locked: true},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseWorktreeList_trailingNewlineAbsent(t *testing.T) {
	// Real git output ends in a blank line, but be defensive: a record without one should still parse.
	src := "worktree /repo/x\nHEAD abc\nbranch refs/heads/x"
	got, err := parseWorktreeList([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 || got[0].Branch != "x" {
		t.Fatalf("got %+v, want one entry with branch x", got)
	}
}

func TestWorktree_Label(t *testing.T) {
	tests := []struct {
		w    Worktree
		want string
	}{
		{Worktree{Branch: "main"}, "main"},
		{Worktree{Branch: "feature/foo"}, "feature/foo"},
		{Worktree{Bare: true}, "(bare)"},
		{Worktree{Detached: true, HEAD: "0123456789abcdef"}, "01234567 (detached)"},
		{Worktree{Detached: true, HEAD: "abc"}, "abc (detached)"},
	}
	for _, tc := range tests {
		if got := tc.w.Label(); got != tc.want {
			t.Errorf("Label(%+v) = %q, want %q", tc.w, got, tc.want)
		}
	}
}

func TestListWorktrees_integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	root := t.TempDir()
	main := filepath.Join(root, "main")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}

	gitDo(t, main, "init", "-q", "-b", "main")
	gitDo(t, main, "config", "user.email", "test@example.com")
	gitDo(t, main, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(main, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitDo(t, main, "add", "f")
	gitDo(t, main, "commit", "-q", "-m", "init")

	feature := filepath.Join(root, "feature")
	gitDo(t, main, "worktree", "add", "-q", "-b", "feature/foo", feature)

	got, err := ListWorktrees(main)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2; got %+v", len(got), got)
	}

	var branches []string
	for _, w := range got {
		branches = append(branches, w.Branch)
	}
	sort.Strings(branches)
	wantBranches := []string{"feature/foo", "main"}
	for i, b := range wantBranches {
		if branches[i] != b {
			t.Errorf("branches = %v, want %v", branches, wantBranches)
			break
		}
	}
}

func TestListWorktrees_notARepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	_, err := ListWorktrees(t.TempDir())
	if err == nil {
		t.Fatal("expected error for non-repo path")
	}
}

func gitDo(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
