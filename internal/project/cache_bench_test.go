package project

import (
	"context"
	"fmt"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

func BenchmarkCacheLookup_Hit(b *testing.B) {
	dir := b.TempDir()
	ctx := context.Background()
	c, err := Open(ctx, dir, 1)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	diags := []engine.Diagnostic{
		{File: "a.js", Line: 3, Col: 4, Rule: "no-var", Message: "Use let", Severity: config.SeverityError},
	}
	if err := c.Store(ctx, CacheEntry{Path: "a.js", ContentHash: 42, ModTimeNS: 1000, Diagnostics: diags}); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Lookup(ctx, "a.js", 42) //nolint:errcheck // benchmark hot path
	}
}

func BenchmarkCacheLookup_Miss(b *testing.B) {
	dir := b.TempDir()
	ctx := context.Background()
	c, err := Open(ctx, dir, 1)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Lookup(ctx, "nonexistent.js", 1) //nolint:errcheck // benchmark hot path
	}
}

func BenchmarkCacheStore_Single(b *testing.B) {
	dir := b.TempDir()
	ctx := context.Background()
	c, err := Open(ctx, dir, 1)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	entry := CacheEntry{
		Path: "a.js", ContentHash: 42, ModTimeNS: 1000,
		Diagnostics: []engine.Diagnostic{
			{File: "a.js", Line: 3, Rule: "no-var", Severity: config.SeverityError},
		},
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Store(ctx, entry) //nolint:errcheck // benchmark hot path
	}
}

func benchStoreBatch(b *testing.B, n int) {
	dir := b.TempDir()
	ctx := context.Background()
	c, err := Open(ctx, dir, 1)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	entries := make([]CacheEntry, n)
	for i := range n {
		entries[i] = CacheEntry{
			Path:        fmt.Sprintf("file_%d.js", i),
			ContentHash: uint64(i), //nolint:gosec // test data, no overflow risk
			ModTimeNS:   int64(i * 1000),
			Diagnostics: []engine.Diagnostic{
				{File: fmt.Sprintf("file_%d.js", i), Rule: "no-var", Severity: config.SeverityError},
			},
		}
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.StoreBatch(ctx, entries) //nolint:errcheck // benchmark hot path
	}
}

func BenchmarkCacheStoreBatch_100(b *testing.B)  { benchStoreBatch(b, 100) }
func BenchmarkCacheStoreBatch_1000(b *testing.B) { benchStoreBatch(b, 1000) }

func BenchmarkHashFile_1KB(b *testing.B) {
	data := make([]byte, 1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		HashFile(data)
	}
}

func BenchmarkHashFile_100KB(b *testing.B) {
	data := make([]byte, 100*1024)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		HashFile(data)
	}
}
