package engine

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
)

// BenchmarkApplyFixes measures fix application on a file with many
// non-overlapping fixes — the typical --fix hot path.
func BenchmarkApplyFixes(b *testing.B) {
	const numLines = 10000

	// Build source: "var x_N = N;\n" repeated.
	var src bytes.Buffer
	for i := range numLines {
		fmt.Fprintf(&src, "var x_%d = %d;\n", i, i)
	}
	source := src.Bytes()

	// Build fixes: replace "var" with "const" on every line.
	// Each line is 14+ bytes: "var x_N = N;\n"
	fixes := make([]Fix, 0, numLines)
	offset := 0
	for i := range numLines {
		lineLen := len(fmt.Sprintf("var x_%d = %d;\n", i, i))
		fixes = append(fixes, Fix{
			StartByte: offset,
			EndByte:   offset + 3, // "var"
			NewText:   "const",
		})
		offset += lineLen
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyFixes(source, fixes)
	}
}

// BenchmarkApplyFixes_Overlapping measures conflict detection with many
// overlapping fixes where only every other fix is accepted.
func BenchmarkApplyFixes_Overlapping(b *testing.B) {
	source := bytes.Repeat([]byte("abcdefghij"), 1000)

	// Create overlapping fix pairs: [0,5), [3,8), [10,15), [13,18), ...
	fixes := make([]Fix, 0, 2000)
	for i := 0; i < 10000; i += 10 {
		fixes = append(fixes,
			Fix{StartByte: i, EndByte: i + 5, NewText: "XX"},
			Fix{StartByte: i + 3, EndByte: i + 8, NewText: "YY"},
		)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyFixes(source, fixes)
	}
}

// BenchmarkLintWithFix exercises the full lint + fix collection path:
// engine produces diagnostics with Fix attached, then ApplyFixes processes them.
func BenchmarkLintWithFix(b *testing.B) {
	const numLines = 1000
	line := "var x = 1;\n"
	source := bytes.Repeat([]byte(line), numLines)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "Use let or const",
				Fix:      "let",
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
		diags := eng.LintFile(ctx, "bench.js", source)
		fixes := make([]Fix, 0, len(diags))
		for j := range diags {
			if diags[j].Fix != nil {
				fixes = append(fixes, *diags[j].Fix)
			}
		}
		ApplyFixes(source, fixes)
	}
}
