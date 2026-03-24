package engine

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
)

// BenchmarkParseSuppressComments measures comment scanning overhead on a large
// file with no suppression directives — the common case. This is the cost
// added to every LintFile call.
func BenchmarkParseSuppressComments_NoDirectives(b *testing.B) {
	source := bytes.Repeat([]byte("var x = 1; // some comment\n"), 10000)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseSuppressComments(source)
	}
}

// BenchmarkParseSuppressComments_WithDirectives measures parsing when every
// other line has a suppression directive (worst-case density).
func BenchmarkParseSuppressComments_WithDirectives(b *testing.B) {
	var buf bytes.Buffer
	for i := range 5000 {
		fmt.Fprintf(&buf, "// lint-disable-next-line rule-%d\n", i)
		buf.WriteString("var x = 1;\n")
	}
	source := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parseSuppressComments(source)
	}
}

// BenchmarkFilterSuppressed measures the in-place filter over many diagnostics
// where half are suppressed.
func BenchmarkFilterSuppressed(b *testing.B) {
	const n = 10000
	diags := make([]Diagnostic, n)
	for i := range n {
		diags[i] = Diagnostic{Line: i + 1, Rule: "no-var"}
	}

	sup := suppressions{
		file:  map[string]bool{},
		lines: make(map[int]map[string]bool, n/2),
	}
	// Suppress even-numbered lines.
	for i := range n {
		if i%2 == 0 {
			sup.lines[i+1] = map[string]bool{"no-var": true}
		}
	}

	// Preallocate a buffer and reslice each iteration to avoid measuring
	// allocation overhead instead of the filter itself.
	buf := make([]Diagnostic, n)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d := buf[:n]
		copy(d, diags)
		filterSuppressed(d, sup)
	}
}

// BenchmarkFilterSuppressed_Blocks measures block-range lookups with many
// active block ranges — exercises the linear scan path.
func BenchmarkFilterSuppressed_Blocks(b *testing.B) {
	const n = 10000
	diags := make([]Diagnostic, n)
	for i := range n {
		diags[i] = Diagnostic{Line: i + 1, Rule: "no-var"}
	}

	// 100 block ranges covering alternating 50-line chunks.
	sup := suppressions{
		file:   map[string]bool{},
		lines:  map[int]map[string]bool{},
		blocks: make([]blockRange, 100),
	}
	for i := range 100 {
		sup.blocks[i] = blockRange{
			startLine: i*100 + 1,
			endLine:   i*100 + 50,
			rule:      "no-var",
		}
	}

	buf := make([]Diagnostic, n)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		d := buf[:n]
		copy(d, diags)
		filterSuppressed(d, sup)
	}
}

// BenchmarkLintFile_WithSuppression exercises the full LintFile path including
// suppression parsing and filtering — mirrors BenchmarkLintWithFix.
func BenchmarkLintFile_WithSuppression(b *testing.B) {
	const numLines = 1000
	var buf bytes.Buffer
	for range numLines / 2 {
		buf.WriteString("var x = 1; // lint-disable use-let\n")
		buf.WriteString("var y = 2;\n")
	}
	source := buf.Bytes()

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"use-let": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "Use let or const",
			},
		},
	}
	eng, errs := New(cfg)
	if len(errs) > 0 {
		b.Fatalf("engine init: %v", errs)
	}

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng.LintFile(ctx, "bench.js", source)
	}
}
