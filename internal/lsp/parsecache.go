package lsp

import (
	"context"
	"sync"

	"github.com/ralfjs/ralf/internal/parser"
)

// parseCache maps open-document paths to their most recent parse tree.
// Keying is by absolute path; freshness is tracked via the docStore
// generation counter passed alongside source.
//
// The cache owns the trees it returns. Callers must not Close them.
// Invalidate must be called on didClose; Close on server shutdown.
type parseCache struct {
	mu      sync.Mutex
	entries map[string]*parsedDoc
}

type parsedDoc struct {
	tree *parser.Tree
	gen  uint64
}

func newParseCache() *parseCache {
	return &parseCache{entries: make(map[string]*parsedDoc)}
}

// Get returns a parse tree for (path, source) at the given generation.
//
// When an entry at exactly the requested generation exists, the cached tree
// is returned. Otherwise Get parses source and replaces any existing entry.
// Returns nil if the path has an unsupported language, parse fails, or
// ctx is cancelled.
//
// The tree is owned by the cache.
func (pc *parseCache) Get(ctx context.Context, path string, source []byte, gen uint64) *parser.Tree {
	pc.mu.Lock()
	if entry, ok := pc.entries[path]; ok && entry.gen == gen {
		tree := entry.tree
		pc.mu.Unlock()
		return tree
	}
	pc.mu.Unlock()

	lang, ok := parser.LangFromPath(path)
	if !ok {
		return nil
	}

	p := parser.NewParser(lang)
	parsed, err := p.Parse(ctx, source, nil)
	p.Close()
	if err != nil {
		return nil
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()
	// Re-check under the lock: another goroutine may have cached this gen
	// while we were parsing.
	if entry, ok := pc.entries[path]; ok && entry.gen == gen {
		parsed.Close()
		return entry.tree
	}
	if existing, ok := pc.entries[path]; ok {
		existing.tree.Close()
	}
	pc.entries[path] = &parsedDoc{tree: parsed, gen: gen}
	return parsed
}

// Invalidate removes and closes the cache entry for a path.
// No-op if the path is not cached.
func (pc *parseCache) Invalidate(path string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if entry, ok := pc.entries[path]; ok {
		entry.tree.Close()
		delete(pc.entries, path)
	}
}

// Close closes every cached tree and empties the cache.
// Safe to call multiple times.
func (pc *parseCache) Close() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for _, entry := range pc.entries {
		entry.tree.Close()
	}
	pc.entries = map[string]*parsedDoc{}
}
