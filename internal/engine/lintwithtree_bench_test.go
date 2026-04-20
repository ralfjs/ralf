package engine

import (
	"bytes"
	"context"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
)

// benchLintWithTreeSetup builds the source, tree, and engine shared by the
// LintFile / LintFileWithTree parity benchmarks.
func benchLintWithTreeSetup(b *testing.B) (ctx context.Context, eng *Engine, source []byte, tree *parser.Tree) {
	b.Helper()

	// Realistic medium file: 100 lines triggering several builtin rules.
	line := "if (x) {}\nvar obj = { a: 1, a: 2 };\ntypeof x === \"strng\";\nx = x;\ndelete y;\n"
	source = bytes.Repeat([]byte(line), 100)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-empty":       {Severity: config.SeverityError, Builtin: true, Message: "Empty block."},
			"no-dupe-keys":   {Severity: config.SeverityError, Builtin: true, Message: "Duplicate key."},
			"valid-typeof":   {Severity: config.SeverityError, Builtin: true, Message: "Invalid typeof."},
			"no-self-assign": {Severity: config.SeverityError, Builtin: true, Message: "Self-assign."},
			"no-delete-var":  {Severity: config.SeverityError, Builtin: true, Message: "Delete var."},
		},
	}
	var errs []error
	eng, errs = New(cfg)
	if len(errs) > 0 {
		b.Fatalf("engine init: %v", errs)
	}

	p := parser.NewParser(parser.LangJS)
	b.Cleanup(p.Close)

	var err error
	tree, err = p.Parse(context.Background(), source, nil)
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	b.Cleanup(tree.Close)

	return context.Background(), eng, source, tree
}

// BenchmarkLintFile_Parses measures LintFile on a single file with builtin
// rules enabled. Each iteration re-parses tree-sitter internally. Baseline
// for BenchmarkLintFileWithTree_PreParsed.
func BenchmarkLintFile_Parses(b *testing.B) {
	ctx, eng, source, _ := benchLintWithTreeSetup(b)

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		eng.LintFile(ctx, "bench.js", source)
	}
}

// BenchmarkLintFileWithTree_PreParsed measures LintFileWithTree on the same
// file using a pre-parsed tree reused across iterations. The delta against
// BenchmarkLintFile_Parses is the per-call parsing cost that the LSP parse
// cache eliminates on cache hit.
func BenchmarkLintFileWithTree_PreParsed(b *testing.B) {
	ctx, eng, source, tree := benchLintWithTreeSetup(b)

	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		eng.LintFileWithTree(ctx, "bench.js", source, tree)
	}
}
