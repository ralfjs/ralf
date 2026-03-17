package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/parser"
)

// BenchmarkMatchStructural measures structural AST matching on a file with
// many function declarations — the hot path for structural rules.
func BenchmarkMatchStructural(b *testing.B) {
	const numFunctions = 500

	var src bytes.Buffer
	for i := range numFunctions {
		fmt.Fprintf(&src, "function fn_%d() {}\n", i)
	}
	source := src.Bytes()

	rules := []compiledStructural{{
		name:     "no-fn",
		matcher:  compiledASTMatcher{kind: "function_declaration"},
		message:  "No functions",
		severity: config.SeverityError,
	}}

	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchStructural(ctx, rules, tree, source, lineStarts)
	}
}

// BenchmarkMatchStructural_NestedParent measures structural matching with a
// parent constraint on deeply nested code.
func BenchmarkMatchStructural_NestedParent(b *testing.B) {
	const numFunctions = 200

	var src bytes.Buffer
	for i := range numFunctions {
		fmt.Fprintf(&src, "function outer_%d() { function inner_%d() {} }\n", i, i)
	}
	source := src.Bytes()

	rules := []compiledStructural{{
		name: "no-nested-fn",
		matcher: compiledASTMatcher{
			kind: "function_declaration",
			parent: &compiledASTMatcher{
				parent: &compiledASTMatcher{kind: "function_declaration"},
			},
		},
		message:  "No nested functions",
		severity: config.SeverityError,
	}}

	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchStructural(ctx, rules, tree, source, lineStarts)
	}
}

// BenchmarkWalkNamedOnly measures the raw cost of walking the AST with a
// single Kind() call per node — the irreducible CGo floor.
func BenchmarkWalkNamedOnly(b *testing.B) {
	const numFunctions = 500

	var src bytes.Buffer
	for i := range numFunctions {
		fmt.Fprintf(&src, "function fn_%d() {}\n", i)
	}
	source := src.Bytes()

	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
			_ = node.Kind()
			return true
		})
	}
}

// BenchmarkMatchStructural_NameRegex measures structural matching with a
// name regex constraint — exercises the ChildByFieldID + rure path.
func BenchmarkMatchStructural_NameRegex(b *testing.B) {
	const numFunctions = 500

	var src bytes.Buffer
	for i := range numFunctions {
		fmt.Fprintf(&src, "function debug_%d() {}\n", i)
	}
	source := src.Bytes()

	nm, err := compileNameMatch("test", "/^debug/")
	if err != nil {
		b.Fatal(err)
	}

	rules := []compiledStructural{{
		name:     "no-debug",
		matcher:  compiledASTMatcher{kind: "function_declaration", name: &nm},
		message:  "No debug functions",
		severity: config.SeverityError,
	}}

	p := parser.NewParser(parser.LangJS)
	tree, parseErr := p.Parse(context.Background(), source, nil)
	p.Close()
	if parseErr != nil {
		b.Fatal(parseErr)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchStructural(ctx, rules, tree, source, lineStarts)
	}
}

// BenchmarkMatchStructural_Naming measures structural matching with a naming
// convention constraint — exercises extractNameField + rure naming regex
// on every AST-matched node.
func BenchmarkMatchStructural_Naming(b *testing.B) {
	const numFunctions = 500

	var src bytes.Buffer
	for i := range numFunctions {
		// Half uppercase (violating), half lowercase (conforming).
		if i%2 == 0 {
			fmt.Fprintf(&src, "function Fn_%d() {}\n", i)
		} else {
			fmt.Fprintf(&src, "function fn_%d() {}\n", i)
		}
	}
	source := src.Bytes()

	nm, err := compileNaming("bench", &config.NamingMatcher{
		Match:   "^[a-z]",
		Message: "must be camelCase",
	})
	if err != nil {
		b.Fatal(err)
	}

	rules := []compiledStructural{{
		name:     "camelcase-fn",
		matcher:  compiledASTMatcher{kind: "function_declaration"},
		naming:   nm,
		message:  "default",
		severity: config.SeverityError,
	}}

	p := parser.NewParser(parser.LangJS)
	tree, parseErr := p.Parse(context.Background(), source, nil)
	p.Close()
	if parseErr != nil {
		b.Fatal(parseErr)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchStructural(ctx, rules, tree, source, lineStarts)
	}
}

// BenchmarkMatchImports measures import ordering analysis on a file with
// many imports — the hot path for import rules.
func BenchmarkMatchImports(b *testing.B) {
	var src bytes.Buffer
	// 10 builtin, 10 external, 5 sibling — interleaved to trigger violations.
	builtins := []string{"fs", "path", "http", "crypto", "os", "net", "url", "tls", "zlib", "events"}
	externals := []string{"react", "lodash", "express", "axios", "moment", "chalk", "debug", "uuid", "dotenv", "cors"}
	siblings := []string{"./utils", "./config", "./helpers", "./types", "./constants"}
	for i := range 10 {
		fmt.Fprintf(&src, "import %s from %q;\n", builtins[i], builtins[i])
		fmt.Fprintf(&src, "import %s from %q;\n", externals[i], externals[i])
		if i < 5 {
			fmt.Fprintf(&src, "import { x%d } from %q;\n", i, siblings[i])
		}
	}
	source := src.Bytes()

	rules := []compiledImport{{
		name:     "import-order",
		groups:   []importGroup{groupBuiltin, groupExternal, groupSibling},
		alpha:    true,
		newline:  true,
		severity: config.SeverityWarn,
		message:  "wrong import order",
	}}

	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), source, nil)
	p.Close()
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchImports(ctx, rules, tree, source, lineStarts)
	}
}

// BenchmarkLintE2E_AllFiveTypes exercises the full Engine.Lint path with all
// five rule types: regex, pattern, structural, AST+naming, and imports.
// 50 files × 100 lines approximates a mid-size project with diverse rule configuration.
func BenchmarkLintE2E_AllFiveTypes(b *testing.B) {
	const (
		numFiles     = 50
		linesPerFile = 100
	)

	line := "import fs from \"fs\";\nimport React from \"react\";\nvar x = 1; console.log(x); function f() {}\n"
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
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "Use let or const",
			},
			"no-console": {
				Severity: config.SeverityWarn,
				Pattern:  "console.log($$$ARGS)",
				Message:  "No console.log",
			},
			"no-fn-decl": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Message:  "No function declarations",
			},
			"camelcase-fn": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Naming:   &config.NamingMatcher{Match: "^[a-z]", Message: "must be camelCase"},
			},
			"import-order": {
				Severity: config.SeverityWarn,
				Imports: &config.ImportsMatcher{
					Groups:      []string{"builtin", "external"},
					Alphabetize: true,
				},
				Message: "wrong import order",
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
		result := eng.Lint(ctx, files, 0)
		if len(result.Errors) > 0 {
			b.Fatalf("lint errors: %v", result.Errors)
		}
	}
}
