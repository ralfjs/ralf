package lsp

import (
	"context"
	"sync"

	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/parser"
)

// parseCache maps open-document paths to their most recent parse tree.
// Keying is by absolute path; freshness is tracked via the docStore
// generation counter passed alongside source.
//
// Trees are refcounted so concurrent readers are safe across didClose /
// generation bumps. Get hands back a release closure that the caller must
// invoke when done; Invalidate and Close mark entries stale and defer the
// actual tree-sitter Close until every outstanding handle has released,
// preventing use-after-close on in-flight reads.
type parseCache struct {
	mu      sync.Mutex
	entries map[string]*parsedDoc
}

type parsedDoc struct {
	tree  *parser.Tree
	gen   uint64
	refs  int  // outstanding Get handles that have not released
	stale bool // marked for close once refs drops to 0
}

func newParseCache() *parseCache {
	return &parseCache{entries: make(map[string]*parsedDoc)}
}

// noopRelease is returned alongside a nil tree so callers can unconditionally
// defer the release without a nil check.
func noopRelease() {}

// Get returns a parse tree for (path, source) at the given generation,
// along with a release closure that the caller MUST invoke (typically via
// defer) when done with the tree.
//
// When an entry at exactly the requested generation exists, the cached
// tree is returned with an incremented refcount. Otherwise Get parses
// source, installs the new tree, and marks any superseded entry stale —
// the superseded tree closes when its last outstanding handle releases.
//
// Returns (nil, noopRelease) if the path has an unsupported language,
// parse fails, or ctx is cancelled. Callers may still defer the returned
// release safely in either branch.
func (pc *parseCache) Get(ctx context.Context, path string, source []byte, gen uint64) (tree *parser.Tree, release func()) {
	pc.mu.Lock()
	if entry, ok := pc.entries[path]; ok && entry.gen == gen {
		entry.refs++
		tree := entry.tree
		pc.mu.Unlock()
		return tree, pc.releaseFn(entry)
	}
	pc.mu.Unlock()

	lang, ok := parser.LangFromPath(path)
	if !ok {
		return nil, noopRelease
	}

	// Share the engine's CGo concurrency budget so we don't exceed NumCPU
	// across concurrent lint + cache parses.
	engine.AcquireCGo()
	p := parser.NewParser(lang)
	parsed, err := p.Parse(ctx, source, nil)
	p.Close()
	engine.ReleaseCGo()
	if err != nil {
		return nil, noopRelease
	}

	pc.mu.Lock()
	defer pc.mu.Unlock()
	// Race resolution: if another goroutine cached this generation while
	// we were parsing, discard our parse and piggy-back on theirs.
	if entry, ok := pc.entries[path]; ok && entry.gen == gen {
		parsed.Close()
		entry.refs++
		return entry.tree, pc.releaseFn(entry)
	}
	// Supersede any older entry. Its tree stays alive until outstanding
	// handles release.
	if existing, ok := pc.entries[path]; ok {
		existing.stale = true
		if existing.refs == 0 {
			existing.tree.Close()
		}
	}
	newEntry := &parsedDoc{tree: parsed, gen: gen, refs: 1}
	pc.entries[path] = newEntry
	return parsed, pc.releaseFn(newEntry)
}

// releaseFn returns a closure that decrements entry's refcount and closes
// the tree if the entry is stale and unreferenced.
func (pc *parseCache) releaseFn(entry *parsedDoc) func() {
	return func() {
		pc.mu.Lock()
		defer pc.mu.Unlock()
		entry.refs--
		if entry.stale && entry.refs == 0 {
			entry.tree.Close()
		}
	}
}

// Invalidate removes the cache entry for a path and marks its tree stale.
// The tree closes once all outstanding handles release. No-op if the path
// is not cached.
func (pc *parseCache) Invalidate(path string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if entry, ok := pc.entries[path]; ok {
		entry.stale = true
		if entry.refs == 0 {
			entry.tree.Close()
		}
		delete(pc.entries, path)
	}
}

// Close marks every entry stale and empties the map. Trees still held by
// outstanding handles close on their final Release. Safe to call multiple
// times; callers should ensure no new Get calls race with Close.
func (pc *parseCache) Close() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	for _, entry := range pc.entries {
		entry.stale = true
		if entry.refs == 0 {
			entry.tree.Close()
		}
	}
	pc.entries = map[string]*parsedDoc{}
}
