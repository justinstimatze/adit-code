package mcp

import (
	"sync"
	"time"

	"github.com/justinstimatze/adit-code/internal/score"
)

// repoCache caches ScoreRepo results by directory path with a TTL.
// Within an MCP session, repeated calls to score the same directory
// reuse the cached result instead of re-parsing all files.
type repoCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

type cacheEntry struct {
	result  *score.RepoScore
	created time.Time
}

func newRepoCache(ttl time.Duration) *repoCache {
	return &repoCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

// Get returns a cached result if it exists and hasn't expired.
func (c *repoCache) Get(key string) (*score.RepoScore, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || time.Since(entry.created) > c.ttl {
		if ok {
			delete(c.entries, key)
		}
		return nil, false
	}
	return entry.result, true
}

// Set stores a result in the cache.
func (c *repoCache) Set(key string, result *score.RepoScore) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cacheEntry{
		result:  result,
		created: time.Now(),
	}
}

// Invalidate clears all cached entries.
func (c *repoCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}
