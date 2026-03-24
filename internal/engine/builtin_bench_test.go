package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
)

// BenchmarkMatchBuiltin exercises a single builtin checker (no-empty) via
// matchBuiltins against a pre-parsed tree. Measures single-walk dispatch +
// diagnostic construction cost, isolated from parsing and CGo overhead.
func BenchmarkMatchBuiltin(b *testing.B) {
	source := bytes.Repeat([]byte("if (x) {}\nif (y) { z(); }\nfor (;;) {}\n"), 1000)
	lineStarts := buildLineIndex(source)

	acquireCGo()
	defer releaseCGo()

	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	rules, _ := compileBuiltinRules(map[string]config.RuleConfig{
		"no-empty": {Severity: config.SeverityError, Builtin: true, Message: "Empty block statement."},
	})

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchBuiltins(rules, tree, source, lineStarts)
	}
}

// BenchmarkMatchBuiltin_MultiRule runs all 33 builtin checkers via a single
// matchBuiltins walk against a source that triggers multiple rules.
func BenchmarkMatchBuiltin_MultiRule(b *testing.B) {
	line := `if (x) {}
var obj = { a: 1, a: 2 };
typeof x === "strng";
if (x = 1) {}
x = x;
delete y;
`
	source := bytes.Repeat([]byte(line), 200)
	lineStarts := buildLineIndex(source)

	acquireCGo()
	defer releaseCGo()

	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	rules := config.BuiltinRules()
	builtinOnly := make(map[string]config.RuleConfig)
	for name := range rules {
		if rules[name].Builtin {
			builtinOnly[name] = rules[name]
		}
	}
	compiled, _ := compileBuiltinRules(builtinOnly)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchBuiltins(compiled, tree, source, lineStarts)
	}
}

// BenchmarkLintE2E_Builtin exercises the full Engine.Lint path with only
// builtin rules enabled. 50 files × 100 lines, measuring end-to-end
// including parsing, file I/O, and parallel scheduling.
func BenchmarkLintE2E_Builtin(b *testing.B) {
	const (
		numFiles     = 50
		linesPerFile = 100
	)

	line := "if (x) {}\nvar obj = { a: 1, a: 2 };\ntypeof x === \"strng\";\n"
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
			"no-empty":       {Severity: config.SeverityError, Builtin: true, Message: "Empty block statement."},
			"no-dupe-keys":   {Severity: config.SeverityError, Builtin: true, Message: "Duplicate key."},
			"valid-typeof":   {Severity: config.SeverityError, Builtin: true, Message: "Invalid typeof comparison value."},
			"no-self-assign": {Severity: config.SeverityError, Builtin: true, Message: "Self-assignment."},
			"no-delete-var":  {Severity: config.SeverityError, Builtin: true, Message: "Variables should not be deleted."},
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
		result := eng.Lint(ctx, files, 0)
		if len(result.Errors) > 0 {
			b.Fatalf("lint errors: %v", result.Errors)
		}
	}
}

// BenchmarkLintE2E_AllRules exercises all 61 built-in rules (regex, pattern,
// structural, builtin) via RecommendedConfig. 50 files × 100 lines.
// This is the closest benchmark to real-world zero-config usage.
func BenchmarkLintE2E_AllRules(b *testing.B) {
	const (
		numFiles     = 50
		linesPerFile = 100
	)

	line := `var x = 1; console.log(x); eval("y");
if (x) {}
var obj = { a: 1, a: 2 };
typeof x === "strng";
x = x;
delete y;
`
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

	cfg := config.RecommendedConfig()
	eng, errs := New(cfg)
	if len(errs) > 0 {
		b.Fatalf("engine init: %v", errs)
	}

	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result := eng.Lint(ctx, files, 0)
		if len(result.Errors) > 0 {
			b.Fatalf("lint errors: %v", result.Errors)
		}
	}
}
