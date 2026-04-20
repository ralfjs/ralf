package lsp

import (
	"context"
	"sync"
	"testing"

	"github.com/ralfjs/ralf/internal/parser"
)

func TestParseCache_Get_CachesTreeAcrossCalls(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	src := []byte("const x = 1;")
	t1, r1 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	defer r1()
	if t1 == nil {
		t.Fatal("first Get returned nil")
	}
	t2, r2 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	defer r2()
	if t1 != t2 {
		t.Errorf("expected same tree pointer on cache hit, got different (%p vs %p)", t1, t2)
	}
}

func TestParseCache_Get_ReparsesOnNewGeneration(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	ctx := context.Background()
	t1, r1 := pc.Get(ctx, "/tmp/a.js", []byte("const x = 1;"), 1)
	defer r1()
	if t1 == nil {
		t.Fatal("gen 1: nil tree")
	}
	t2, r2 := pc.Get(ctx, "/tmp/a.js", []byte("const x = 2;"), 2)
	defer r2()
	if t2 == nil {
		t.Fatal("gen 2: nil tree")
	}
	if t1 == t2 {
		t.Errorf("expected new tree at new generation, got same pointer")
	}
}

func TestParseCache_Get_UnsupportedLanguage(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	tree, release := pc.Get(context.Background(), "/tmp/readme.txt", []byte("hello"), 1)
	defer release()
	if tree != nil {
		t.Errorf("expected nil for unsupported extension, got tree %p", tree)
	}
	// release must be a usable no-op even when tree is nil.
}

func TestParseCache_Invalidate_ClearsEntry(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	src := []byte("const x = 1;")
	t1, r1 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	if t1 == nil {
		t.Fatal("Get returned nil")
	}
	r1()

	pc.Invalidate("/tmp/a.js")

	// After invalidation, Get with the same (path, gen) must parse fresh.
	t2, r2 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	defer r2()
	if t2 == nil {
		t.Fatal("Get after invalidate returned nil")
	}
	if t1 == t2 {
		t.Errorf("expected new tree after Invalidate, got same pointer")
	}
}

func TestParseCache_Invalidate_UnknownPath(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	// Must not panic when invalidating a path that was never cached.
	pc.Invalidate("/tmp/nonexistent.js")
}

// TestParseCache_InvalidateWhileHeld_DefersClose exercises the refcount
// invariant: when Invalidate runs while a reader still holds a handle, the
// tree must remain live until the reader releases. A tree-sitter call on a
// prematurely-closed tree would SIGSEGV inside CGo.
func TestParseCache_InvalidateWhileHeld_DefersClose(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	tree, release := pc.Get(context.Background(), "/tmp/a.js", []byte("const x = 1;"), 1)
	if tree == nil {
		t.Fatal("Get returned nil")
	}

	if tree.RootNode().IsNull() {
		t.Fatal("fresh tree root is null")
	}

	// Invalidate while we still hold the handle. The tree must stay alive.
	pc.Invalidate("/tmp/a.js")

	// Would crash inside tree-sitter if the tree had been closed.
	if tree.RootNode().IsNull() {
		t.Fatal("tree root null after invalidate-while-held (expected live tree)")
	}

	// Dropping the last reference finishes the deferred close. We cannot
	// directly observe the close without crashing tree-sitter, but after
	// this point the tree pointer is unsafe to use.
	release()
}

// TestParseCache_Get_ReplaceWhileOldHeld tests that caching a newer
// generation while the older tree's handle is still outstanding does not
// close the old tree prematurely.
func TestParseCache_Get_ReplaceWhileOldHeld(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	ctx := context.Background()
	oldTree, oldRelease := pc.Get(ctx, "/tmp/a.js", []byte("const x = 1;"), 1)
	if oldTree == nil {
		t.Fatal("gen 1 Get nil")
	}

	newTree, newRelease := pc.Get(ctx, "/tmp/a.js", []byte("const x = 2;"), 2)
	defer newRelease()
	if newTree == nil {
		t.Fatal("gen 2 Get nil")
	}
	if oldTree == newTree {
		t.Fatal("expected distinct trees across generations")
	}

	// Old tree is still reachable while the old handle is outstanding.
	if oldTree.RootNode().IsNull() {
		t.Fatal("old tree closed while handle still held")
	}

	oldRelease()

	// New tree is always usable.
	if newTree.RootNode().IsNull() {
		t.Fatal("new tree root is null")
	}
}

func TestParseCache_Close_ClearsAllEntries(t *testing.T) {
	t.Parallel()

	pc := newParseCache()

	ctx := context.Background()
	_, r1 := pc.Get(ctx, "/tmp/a.js", []byte("const x = 1;"), 1)
	r1()
	_, r2 := pc.Get(ctx, "/tmp/b.js", []byte("const y = 2;"), 1)
	r2()

	pc.Close()

	// Close is idempotent.
	pc.Close()

	// After Close, the cache is still usable: Get re-parses.
	tree, release := pc.Get(ctx, "/tmp/a.js", []byte("const x = 1;"), 1)
	defer release()
	if tree == nil {
		t.Fatal("Get after Close returned nil")
	}
}

func TestParseCache_Get_ConcurrentSameGen(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	const goroutines = 32
	src := []byte("const x = 1;")

	trees := make([]*parser.Tree, goroutines)
	releases := make([]func(), goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	start := make(chan struct{})
	for i := range goroutines {
		go func() {
			defer wg.Done()
			<-start
			trees[i], releases[i] = pc.Get(context.Background(), "/tmp/a.js", src, 1)
		}()
	}
	close(start)
	wg.Wait()

	for i := range goroutines {
		if trees[i] == nil {
			t.Fatalf("goroutine %d got nil", i)
		}
	}
	canonical, canonicalRelease := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	defer canonicalRelease()
	for i, got := range trees {
		if got != canonical {
			t.Errorf("goroutine %d tree %p != canonical %p", i, got, canonical)
		}
	}
	for _, r := range releases {
		r()
	}
}
