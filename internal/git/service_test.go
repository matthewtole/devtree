package git

import (
	"fmt"
	"testing"
	"time"
)

// testService returns a Service with a stubbed git call and a controllable
// clock. The returned *int counts fetches; *time.Time is the fake now.
func testService(ttl time.Duration) (*Service, *int, *time.Time) {
	calls := 0
	now := time.Unix(1000, 0)

	s := NewService(ttl)
	s.list = func(repo string) ([]Worktree, error) {
		calls++
		if repo == "/broken" {
			return nil, fmt.Errorf("git exploded")
		}
		return []Worktree{{Path: repo + "/wt", Branch: "main"}}, nil
	}
	s.now = func() time.Time { return now }

	return s, &calls, &now
}

func TestService_cachesWithinTTL(t *testing.T) {
	s, calls, _ := testService(200 * time.Millisecond)

	first, err := s.Worktrees("/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, _ := s.Worktrees("/repo")

	if *calls != 1 {
		t.Errorf("fetch count = %d, want 1 (second call should hit cache)", *calls)
	}
	if len(first) != 1 || len(second) != 1 || second[0].Branch != "main" {
		t.Errorf("cached result mismatch: first %v, second %v", first, second)
	}
}

func TestService_refreshesAfterTTL(t *testing.T) {
	s, calls, now := testService(200 * time.Millisecond)

	s.Worktrees("/repo")
	*now = now.Add(201 * time.Millisecond)
	s.Worktrees("/repo")

	if *calls != 2 {
		t.Errorf("fetch count = %d, want 2 (TTL expired)", *calls)
	}
}

func TestService_cachesErrors(t *testing.T) {
	s, calls, _ := testService(200 * time.Millisecond)

	if _, err := s.Worktrees("/broken"); err == nil {
		t.Fatal("expected error from broken repo")
	}
	if _, err := s.Worktrees("/broken"); err == nil {
		t.Fatal("cached call should return the cached error")
	}
	if *calls != 1 {
		t.Errorf("fetch count = %d, want 1 (error should be cached, not retried)", *calls)
	}
}

func TestService_cachesPerRepo(t *testing.T) {
	s, calls, _ := testService(200 * time.Millisecond)

	s.Worktrees("/repo-a")
	s.Worktrees("/repo-b")
	s.Worktrees("/repo-a")

	if *calls != 2 {
		t.Errorf("fetch count = %d, want 2 (one per distinct repo)", *calls)
	}
}
