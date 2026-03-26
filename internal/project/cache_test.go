package project

import (
	"context"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

func openTestCache(t *testing.T, configHash uint64) *Cache {
	t.Helper()
	dir := t.TempDir()
	c, err := Open(context.Background(), dir, configHash)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestCacheOpen_CreatesDatabase(t *testing.T) {
	c := openTestCache(t, 42)
	if c.db == nil {
		t.Fatal("expected db to be non-nil")
	}
}

func TestCacheOpen_ConfigHashInvalidation(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// Open with hash A, store an entry.
	c1, err := Open(ctx, dir, 100)
	if err != nil {
		t.Fatal(err)
	}
	err = c1.Store(ctx, CacheEntry{
		Path: "a.js", ContentHash: 1, ModTimeNS: 1000,
		Diagnostics: []engine.Diagnostic{{File: "a.js", Rule: "r1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c1.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen with hash B — entry should be gone.
	c2, err := Open(ctx, dir, 200)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c2.Close(); err != nil {
			t.Error(err)
		}
	}()

	_, hit, err := c2.Lookup(ctx, "a.js", 1)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("expected cache miss after config change")
	}
}

func TestCacheLookup_Hit(t *testing.T) {
	c := openTestCache(t, 1)
	ctx := context.Background()

	diags := []engine.Diagnostic{
		{File: "a.js", Line: 3, Col: 4, Rule: "no-var", Message: "Use let", Severity: config.SeverityError},
	}
	err := c.Store(ctx, CacheEntry{Path: "a.js", ContentHash: 42, ModTimeNS: 1000, Diagnostics: diags})
	if err != nil {
		t.Fatal(err)
	}

	got, hit, err := c.Lookup(ctx, "a.js", 42)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("expected cache hit")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(got))
	}
	if got[0].Rule != "no-var" {
		t.Errorf("expected rule no-var, got %q", got[0].Rule)
	}
}

func TestCacheLookup_Miss_HashChanged(t *testing.T) {
	c := openTestCache(t, 1)
	ctx := context.Background()

	err := c.Store(ctx, CacheEntry{Path: "a.js", ContentHash: 42, ModTimeNS: 1000})
	if err != nil {
		t.Fatal(err)
	}

	_, hit, err := c.Lookup(ctx, "a.js", 99)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("expected cache miss for different hash")
	}
}

func TestCacheLookup_Miss_NotFound(t *testing.T) {
	c := openTestCache(t, 1)

	_, hit, err := c.Lookup(context.Background(), "nonexistent.js", 1)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("expected cache miss for unknown path")
	}
}

func TestCacheStore_Upsert(t *testing.T) {
	c := openTestCache(t, 1)
	ctx := context.Background()

	// Store with hash 1.
	err := c.Store(ctx, CacheEntry{
		Path: "a.js", ContentHash: 1, ModTimeNS: 1000,
		Diagnostics: []engine.Diagnostic{{Rule: "r1"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Upsert with hash 2.
	err = c.Store(ctx, CacheEntry{
		Path: "a.js", ContentHash: 2, ModTimeNS: 2000,
		Diagnostics: []engine.Diagnostic{{Rule: "r2"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Lookup with old hash — miss.
	_, hit, err := c.Lookup(ctx, "a.js", 1)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("expected miss for old hash")
	}

	// Lookup with new hash — hit.
	got, hit, err := c.Lookup(ctx, "a.js", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("expected hit for new hash")
	}
	if got[0].Rule != "r2" {
		t.Errorf("expected rule r2, got %q", got[0].Rule)
	}
}

func TestCacheStore_NilDiagnostics(t *testing.T) {
	c := openTestCache(t, 1)
	ctx := context.Background()

	err := c.Store(ctx, CacheEntry{Path: "a.js", ContentHash: 1, ModTimeNS: 1000, Diagnostics: nil})
	if err != nil {
		t.Fatal(err)
	}

	got, hit, err := c.Lookup(ctx, "a.js", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("expected hit")
	}
	if got == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(got))
	}
}

func TestCacheStoreBatch_MultipleFiles(t *testing.T) {
	c := openTestCache(t, 1)
	ctx := context.Background()

	entries := []CacheEntry{
		{Path: "a.js", ContentHash: 1, ModTimeNS: 1000, Diagnostics: []engine.Diagnostic{{Rule: "r1"}}},
		{Path: "b.js", ContentHash: 2, ModTimeNS: 2000, Diagnostics: []engine.Diagnostic{{Rule: "r2"}}},
		{Path: "c.js", ContentHash: 3, ModTimeNS: 3000},
	}
	if err := c.StoreBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		_, hit, err := c.Lookup(ctx, e.Path, e.ContentHash)
		if err != nil {
			t.Fatal(err)
		}
		if !hit {
			t.Errorf("expected hit for %s", e.Path)
		}
	}
}

func TestCacheStoreBatch_Empty(t *testing.T) {
	c := openTestCache(t, 1)
	if err := c.StoreBatch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestCacheRemove(t *testing.T) {
	c := openTestCache(t, 1)
	ctx := context.Background()

	err := c.Store(ctx, CacheEntry{Path: "a.js", ContentHash: 1, ModTimeNS: 1000})
	if err != nil {
		t.Fatal(err)
	}

	if err := c.Remove(ctx, "a.js"); err != nil {
		t.Fatal(err)
	}

	_, hit, err := c.Lookup(ctx, "a.js", 1)
	if err != nil {
		t.Fatal(err)
	}
	if hit {
		t.Error("expected miss after remove")
	}
}

func TestCacheRemove_NotFound(t *testing.T) {
	c := openTestCache(t, 1)
	if err := c.Remove(context.Background(), "nonexistent.js"); err != nil {
		t.Fatal(err)
	}
}

func TestCacheDiagnosticRoundTrip(t *testing.T) {
	c := openTestCache(t, 1)
	ctx := context.Background()

	fix := &engine.Fix{StartByte: 10, EndByte: 13, NewText: "const"}
	diags := []engine.Diagnostic{
		{
			File: "a.js", Line: 3, Col: 4, EndLine: 3, EndCol: 7,
			Rule: "no-var", Message: "Use let or const",
			Severity: config.SeverityError, Fix: fix,
		},
	}
	err := c.Store(ctx, CacheEntry{Path: "a.js", ContentHash: 1, ModTimeNS: 1000, Diagnostics: diags})
	if err != nil {
		t.Fatal(err)
	}

	got, hit, err := c.Lookup(ctx, "a.js", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !hit {
		t.Fatal("expected hit")
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(got))
	}

	d := got[0]
	if d.File != "a.js" || d.Line != 3 || d.Col != 4 || d.Rule != "no-var" {
		t.Errorf("diagnostic fields mismatch: %+v", d)
	}
	if d.Fix == nil {
		t.Fatal("expected Fix to be non-nil")
	}
	if d.Fix.StartByte != 10 || d.Fix.EndByte != 13 || d.Fix.NewText != "const" {
		t.Errorf("fix fields mismatch: %+v", d.Fix)
	}
}
