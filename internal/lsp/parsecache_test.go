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
	t1 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	if t1 == nil {
		t.Fatal("first Get returned nil")
	}
	t2 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	if t1 != t2 {
		t.Errorf("expected same tree pointer on cache hit, got different (%p vs %p)", t1, t2)
	}
}

func TestParseCache_Get_ReparsesOnNewGeneration(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	ctx := context.Background()
	t1 := pc.Get(ctx, "/tmp/a.js", []byte("const x = 1;"), 1)
	if t1 == nil {
		t.Fatal("gen 1: nil tree")
	}
	t2 := pc.Get(ctx, "/tmp/a.js", []byte("const x = 2;"), 2)
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

	tree := pc.Get(context.Background(), "/tmp/readme.txt", []byte("hello"), 1)
	if tree != nil {
		t.Errorf("expected nil for unsupported extension, got tree %p", tree)
	}
}

func TestParseCache_Invalidate_ClearsEntry(t *testing.T) {
	t.Parallel()

	pc := newParseCache()
	t.Cleanup(pc.Close)

	src := []byte("const x = 1;")
	t1 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	if t1 == nil {
		t.Fatal("Get returned nil")
	}

	pc.Invalidate("/tmp/a.js")

	// After invalidation, Get with the same (path, gen) must return a new
	// tree — the old one was closed.
	t2 := pc.Get(context.Background(), "/tmp/a.js", src, 1)
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

func TestParseCache_Close_ClearsAllEntries(t *testing.T) {
	t.Parallel()

	pc := newParseCache()

	ctx := context.Background()
	if pc.Get(ctx, "/tmp/a.js", []byte("const x = 1;"), 1) == nil {
		t.Fatal("seed a.js: nil")
	}
	if pc.Get(ctx, "/tmp/b.js", []byte("const y = 2;"), 1) == nil {
		t.Fatal("seed b.js: nil")
	}

	pc.Close()

	// Close is idempotent.
	pc.Close()

	// After Close, the cache is still usable: Get re-parses.
	if pc.Get(ctx, "/tmp/a.js", []byte("const x = 1;"), 1) == nil {
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
	var wg sync.WaitGroup
	wg.Add(goroutines)
	start := make(chan struct{})
	for i := range goroutines {
		go func() {
			defer wg.Done()
			<-start
			trees[i] = pc.Get(context.Background(), "/tmp/a.js", src, 1)
		}()
	}
	close(start)
	wg.Wait()

	// All goroutines must ultimately observe the same cached tree pointer
	// (the cache resolves races by closing the loser's tree and returning
	// the winner's).
	for i := range goroutines {
		if trees[i] == nil {
			t.Fatalf("goroutine %d got nil", i)
		}
	}
	canonical := pc.Get(context.Background(), "/tmp/a.js", src, 1)
	for i, got := range trees {
		if got != canonical {
			t.Errorf("goroutine %d tree %p != canonical %p", i, got, canonical)
		}
	}
}
