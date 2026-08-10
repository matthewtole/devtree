package git

import (
	"sync"
	"time"
)

// Service is the central access point for worktree lookups. It caches
// per-repo results so callers (the poll loop, the picker) can query freely
// without spawning git more than once per TTL for the same repo.
type Service struct {
	ttl  time.Duration
	list func(string) ([]Worktree, error) // ListWorktrees; stubbed in tests
	now  func() time.Time                 // time.Now; stubbed in tests

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	worktrees []Worktree
	err       error
	fetched   time.Time
}

func NewService(ttl time.Duration) *Service {
	return &Service{
		ttl:   ttl,
		list:  ListWorktrees,
		now:   time.Now,
		cache: make(map[string]cacheEntry),
	}
}

// Worktrees returns the worktrees of repoPath, refreshing from git at most
// once per TTL. Errors are cached under the same TTL so a broken repo isn't
// retried on every call.
func (s *Service) Worktrees(repoPath string) ([]Worktree, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.cache[repoPath]; ok && s.now().Sub(e.fetched) < s.ttl {
		return e.worktrees, e.err
	}

	wts, err := s.list(repoPath)
	s.cache[repoPath] = cacheEntry{worktrees: wts, err: err, fetched: s.now()}
	return wts, err
}
