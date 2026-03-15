package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

func BenchmarkMatchRegex(b *testing.B) {
	source := bytes.Repeat([]byte("var x = 1;\nlet y = 2;\nconst z = 3;\n"), 10000)
	rules := map[string]config.RuleConfig{
		"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
	}
	compiled, _ := compileRegexRules(rules)
	lineStarts := buildLineIndex(source)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchRegex(compiled[0], source, lineStarts, 0)
	}
}

// BenchmarkLintE2E exercises the full Engine.Lint path: disk I/O, config
// merging, parallel file processing via errgroup, CGo semaphore, and regex
// matching. 100 files × 300 lines × 5 rules approximates a mid-size project.
func BenchmarkLintE2E(b *testing.B) {
	const (
		numFiles     = 100
		linesPerFile = 300
	)

	// Synthetic source: mix of patterns that hit different rules.
	line := "var x = 1; console.log(x); eval('y'); debugger; alert('z');\n"
	source := bytes.Repeat([]byte(line), linesPerFile)

	dir := b.TempDir()
	files := make([]string, numFiles)
	for i := range numFiles {
		p := filepath.Join(dir, fmt.Sprintf("file_%03d.js", i))
		if err := os.WriteFile(p, source, 0o600); err != nil {
			b.Fatal(err)
		}
		files[i] = p
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var":      {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "Use let or const"},
			"no-console":  {Severity: config.SeverityWarn, Regex: `console\.log`, Message: "Remove console.log"},
			"no-eval":     {Severity: config.SeverityError, Regex: `\beval\b`, Message: "No eval"},
			"no-debugger": {Severity: config.SeverityError, Regex: `\bdebugger\b`, Message: "No debugger"},
			"no-alert":    {Severity: config.SeverityWarn, Regex: `\balert\b`, Message: "No alert"},
		},
	}
	eng, errs := New(cfg)
	if len(errs) > 0 {
		b.Fatalf("engine init: %v", errs)
	}

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := eng.Lint(ctx, files, 0)
		if len(result.Errors) > 0 {
			b.Fatalf("lint errors: %v", result.Errors)
		}
	}
}
