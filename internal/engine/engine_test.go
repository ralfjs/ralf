package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
)

func TestNew(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if eng == nil {
			t.Fatal("engine is nil")
		}
	})

	t.Run("invalid regex returns errors", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"bad": {Severity: config.SeverityError, Regex: `[invalid`, Message: "Bad"},
			},
		}
		eng, errs := New(cfg)
		if len(errs) == 0 {
			t.Fatal("expected errors")
		}
		if eng != nil {
			t.Fatal("engine should be nil on errors")
		}
	})

	t.Run("compiles pattern regex and import rules", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
				"ast":    {Severity: config.SeverityError, Pattern: "console.log($$$)", Message: "AST"},
				"imports": {Severity: config.SeverityError, Imports: &config.ImportsMatcher{
					Groups: []string{"builtin", "external"},
				}, Message: "Imports"},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(eng.regexRules) != 1 {
			t.Fatalf("expected 1 regex rule, got %d", len(eng.regexRules))
		}
		if len(eng.patternRules) != 1 {
			t.Fatalf("expected 1 pattern rule, got %d", len(eng.patternRules))
		}
		if len(eng.importRules) != 1 {
			t.Fatalf("expected 1 import rule, got %d", len(eng.importRules))
		}
	})
}

func TestLintFileWithTree_ParityWithLintFile(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var":         {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			"no-console-log": {Severity: config.SeverityWarn, Pattern: "console.log($$$)", Message: "No console.log"},
		},
	}
	eng, errs := New(cfg)
	if len(errs) != 0 {
		t.Fatalf("compile: %v", errs)
	}

	source := []byte("var x = 1;\nconsole.log(x);\nvar y = 2;")

	// Baseline: let the engine parse.
	baseline := eng.LintFile(context.Background(), "test.js", source)

	// WithTree: parse separately and pass the tree in.
	p := parser.NewParser(parser.LangJS)
	t.Cleanup(p.Close)
	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Cleanup(tree.Close)

	withTree := eng.LintFileWithTree(context.Background(), "test.js", source, tree)

	if !reflect.DeepEqual(baseline, withTree) {
		t.Errorf("diagnostics differ\nLintFile:         %+v\nLintFileWithTree: %+v", baseline, withTree)
	}
}

func TestLintFile(t *testing.T) {
	t.Run("matches found", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			},
		}
		eng, _ := New(cfg)

		source := []byte("var x = 1;\nlet y = 2;\nvar z = 3;")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 2 {
			t.Fatalf("expected 2 diagnostics, got %d", len(diags))
		}
		if diags[0].File != "test.js" {
			t.Errorf("expected file=test.js, got %s", diags[0].File)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			},
		}
		eng, _ := New(cfg)

		source := []byte("let x = 1;\nconst y = 2;")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("override disables rule", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			},
			Overrides: []config.Override{
				{
					Files: []string{"*.test.js"},
					Rules: map[string]config.RuleConfig{
						"no-var": {Severity: config.SeverityOff, Regex: `\bvar\b`},
					},
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("var x = 1;")
		diags := eng.LintFile(context.Background(), "foo.test.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics (disabled by override), got %d", len(diags))
		}
	})

	t.Run("where predicate filters rule", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {
					Severity: config.SeverityError,
					Regex:    `\bvar\b`,
					Message:  "No var",
					Where:    &config.WherePredicate{File: "src/**/*.js"},
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("var x = 1;")

		// File matches where predicate.
		diags := eng.LintFile(context.Background(), "src/index.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic for matching path, got %d", len(diags))
		}

		// File does not match where predicate.
		diags = eng.LintFile(context.Background(), "test/index.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics for non-matching path, got %d", len(diags))
		}
	})
}

func TestLint(t *testing.T) {
	t.Run("parallel lint with sorting", func(t *testing.T) {
		dir := t.TempDir()

		// Create test files.
		writeFile(t, filepath.Join(dir, "b.js"), "var x = 1;\nvar y = 2;")
		writeFile(t, filepath.Join(dir, "a.js"), "var z = 3;")

		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			},
		}
		eng, _ := New(cfg)

		files := []string{
			filepath.Join(dir, "b.js"),
			filepath.Join(dir, "a.js"),
		}
		result := eng.Lint(context.Background(), files, 2)

		if len(result.Errors) != 0 {
			t.Fatalf("unexpected errors: %v", result.Errors)
		}
		if len(result.Diagnostics) != 3 {
			t.Fatalf("expected 3 diagnostics, got %d", len(result.Diagnostics))
		}

		// Should be sorted by file then line.
		if result.Diagnostics[0].File != filepath.Join(dir, "a.js") {
			t.Errorf("first diagnostic should be from a.js, got %s", result.Diagnostics[0].File)
		}
	})

	t.Run("file read error", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			},
		}
		eng, _ := New(cfg)

		result := eng.Lint(context.Background(), []string{"/nonexistent/file.js"}, 1)
		if len(result.Errors) != 1 {
			t.Fatalf("expected 1 error, got %d", len(result.Errors))
		}
	})

	t.Run("context cancellation", func(_ *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			},
		}
		eng, _ := New(cfg)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		result := eng.Lint(ctx, []string{"/some/file.js"}, 1)
		// Cancellation may or may not propagate depending on timing.
		// Either errors or no output is acceptable.
		_ = result
	})
}

func TestLintFile_PatternRules(t *testing.T) {
	t.Run("basic pattern match", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-console": {
					Severity: config.SeverityError,
					Pattern:  "console.log($$$ARGS)",
					Message:  "No console.log",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("console.log(\"debug\");\nconst x = 1;")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Rule != "no-console" {
			t.Errorf("rule = %q, want %q", diags[0].Rule, "no-console")
		}
		if diags[0].File != "test.js" {
			t.Errorf("file = %q, want %q", diags[0].File, "test.js")
		}
	})

	t.Run("no match", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-console": {
					Severity: config.SeverityError,
					Pattern:  "console.log($$$ARGS)",
					Message:  "No console.log",
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("console.warn(\"ok\");\nconst x = 1;")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("override disables pattern rule", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-console": {
					Severity: config.SeverityError,
					Pattern:  "console.log($$$ARGS)",
					Message:  "No console.log",
				},
			},
			Overrides: []config.Override{
				{
					Files: []string{"*.test.js"},
					Rules: map[string]config.RuleConfig{
						"no-console": {Severity: config.SeverityOff, Pattern: "console.log($$$ARGS)"},
					},
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("console.log(\"debug\");")
		diags := eng.LintFile(context.Background(), "foo.test.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics (disabled by override), got %d", len(diags))
		}
	})

	t.Run("where predicate filters pattern rule", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-console": {
					Severity: config.SeverityError,
					Pattern:  "console.log($$$ARGS)",
					Message:  "No console.log",
					Where:    &config.WherePredicate{File: "src/**/*.js"},
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("console.log(\"debug\");")

		// File matches where predicate.
		diags := eng.LintFile(context.Background(), "src/index.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic for matching path, got %d", len(diags))
		}

		// File does not match where predicate.
		diags = eng.LintFile(context.Background(), "test/index.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics for non-matching path, got %d", len(diags))
		}
	})

	t.Run("non-JS file extension skips pattern rules", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-console": {
					Severity: config.SeverityError,
					Pattern:  "console.log($$$ARGS)",
					Message:  "No console.log",
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("console.log(\"debug\");")
		diags := eng.LintFile(context.Background(), "test.py", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics for .py file, got %d", len(diags))
		}
	})
}

func TestLint_MixedRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "test.js"), "var x = 1;\nconsole.log(x);")

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "No var",
			},
			"no-console": {
				Severity: config.SeverityWarn,
				Pattern:  "console.log($$$ARGS)",
				Message:  "No console.log",
			},
		},
	}
	eng, errs := New(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	result := eng.Lint(context.Background(), []string{filepath.Join(dir, "test.js")}, 1)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("expected 2 diagnostics (1 regex + 1 pattern), got %d", len(result.Diagnostics))
	}

	rules := map[string]bool{}
	for _, d := range result.Diagnostics {
		rules[d.Rule] = true
	}
	if !rules["no-var"] {
		t.Error("missing diagnostic for no-var (regex)")
	}
	if !rules["no-console"] {
		t.Error("missing diagnostic for no-console (pattern)")
	}
}

func TestNew_StructuralRules(t *testing.T) {
	t.Run("compiles structural rules", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-nested-fn": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Kind: "function_declaration", Parent: &config.ASTMatcher{Kind: "function_declaration"}},
					Message:  "No nested functions",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(eng.structuralRules) != 1 {
			t.Fatalf("expected 1 structural rule, got %d", len(eng.structuralRules))
		}
	})

	t.Run("invalid AST rule returns error", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"bad-ast": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Name: 42},
					Message:  "Bad",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) == 0 {
			t.Fatal("expected errors")
		}
		if eng != nil {
			t.Fatal("engine should be nil on errors")
		}
	})
}

func TestLintFile_StructuralRules(t *testing.T) {
	t.Run("basic match", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-fn": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Kind: "function_declaration"},
					Message:  "No functions",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("function foo() {}\nconst x = 1;")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Rule != "no-fn" {
			t.Errorf("rule = %q, want %q", diags[0].Rule, "no-fn")
		}
		if diags[0].File != "test.js" {
			t.Errorf("file = %q, want %q", diags[0].File, "test.js")
		}
	})

	t.Run("override disables structural rule", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-fn": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Kind: "function_declaration"},
					Message:  "No functions",
				},
			},
			Overrides: []config.Override{
				{
					Files: []string{"*.test.js"},
					Rules: map[string]config.RuleConfig{
						"no-fn": {
							Severity: config.SeverityOff,
							AST:      &config.ASTMatcher{Kind: "function_declaration"},
						},
					},
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("function foo() {}")
		diags := eng.LintFile(context.Background(), "foo.test.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics (disabled by override), got %d", len(diags))
		}
	})

	t.Run("where predicate filters structural rule", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-fn": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Kind: "function_declaration"},
					Message:  "No functions",
					Where:    &config.WherePredicate{File: "src/**/*.js"},
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("function foo() {}")

		diags := eng.LintFile(context.Background(), "src/index.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic for matching path, got %d", len(diags))
		}

		diags = eng.LintFile(context.Background(), "test/index.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics for non-matching path, got %d", len(diags))
		}
	})

	t.Run("non-JS file extension skips structural rules", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-fn": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Kind: "function_declaration"},
					Message:  "No functions",
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("function foo() {}")
		diags := eng.LintFile(context.Background(), "test.py", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics for .py file, got %d", len(diags))
		}
	})
}

func TestLint_MixedRulesAllThreeTypes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "test.js"), "var x = 1;\nconsole.log(x);\nfunction foo() {}")

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "No var",
			},
			"no-console": {
				Severity: config.SeverityWarn,
				Pattern:  "console.log($$$ARGS)",
				Message:  "No console.log",
			},
			"no-fn": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Message:  "No functions",
			},
		},
	}
	eng, errs := New(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	result := eng.Lint(context.Background(), []string{filepath.Join(dir, "test.js")}, 1)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Diagnostics) != 3 {
		t.Fatalf("expected 3 diagnostics (1 regex + 1 pattern + 1 structural), got %d", len(result.Diagnostics))
	}

	rules := map[string]bool{}
	for _, d := range result.Diagnostics {
		rules[d.Rule] = true
	}
	if !rules["no-var"] {
		t.Error("missing diagnostic for no-var (regex)")
	}
	if !rules["no-console"] {
		t.Error("missing diagnostic for no-console (pattern)")
	}
	if !rules["no-fn"] {
		t.Error("missing diagnostic for no-fn (structural)")
	}
}

func TestLintFile_NamingRules(t *testing.T) {
	t.Run("flags non-camelCase function", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"camelcase-fn": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Kind: "function_declaration"},
					Naming:   &config.NamingMatcher{Match: "^[a-z][a-zA-Z0-9]*$", Message: "must be camelCase"},
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("function BadName() {}\nfunction goodName() {}")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Rule != "camelcase-fn" {
			t.Errorf("rule = %q, want %q", diags[0].Rule, "camelcase-fn")
		}
		if diags[0].Message != "must be camelCase" {
			t.Errorf("message = %q, want %q", diags[0].Message, "must be camelCase")
		}
		if diags[0].Line != 1 {
			t.Errorf("line = %d, want 1", diags[0].Line)
		}
	})

	t.Run("no violations when all names conform", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"camelcase-fn": {
					Severity: config.SeverityError,
					AST:      &config.ASTMatcher{Kind: "function_declaration"},
					Naming:   &config.NamingMatcher{Match: "^[a-z][a-zA-Z0-9]*$"},
					Message:  "must be camelCase",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("function goodName() {}\nfunction anotherGood() {}")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})
}

func TestLintFile_ImportRules(t *testing.T) {
	t.Run("detects out-of-order imports", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"import-order": {
					Severity: config.SeverityWarn,
					Imports: &config.ImportsMatcher{
						Groups:         []string{"builtin", "external", "sibling"},
						Alphabetize:    true,
						NewlineBetween: true,
					},
					Message: "wrong import order",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("import React from \"react\";\nimport fs from \"fs\";\n")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) == 0 {
			t.Fatal("expected at least 1 diagnostic for out-of-order imports")
		}
		if diags[0].File != "test.js" {
			t.Errorf("file = %q, want %q", diags[0].File, "test.js")
		}
		if diags[0].Rule != "import-order" {
			t.Errorf("rule = %q, want %q", diags[0].Rule, "import-order")
		}
	})

	t.Run("no violations", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"import-order": {
					Severity: config.SeverityWarn,
					Imports: &config.ImportsMatcher{
						Groups: []string{"builtin", "external"},
					},
					Message: "wrong import order",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("import fs from \"fs\";\nimport React from \"react\";\n")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("non-JS file skips import rules", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"import-order": {
					Severity: config.SeverityWarn,
					Imports:  &config.ImportsMatcher{Groups: []string{"builtin", "external"}},
				},
			},
		}
		eng, _ := New(cfg)

		source := []byte("import React from \"react\";\nimport fs from \"fs\";\n")
		diags := eng.LintFile(context.Background(), "test.py", source)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics for .py file, got %d", len(diags))
		}
	})
}

func TestLint_AllFourRuleTypes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "test.js"),
		"var x = 1;\nconsole.log(x);\nfunction Foo() {}\nfunction bar() {}")

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "No var",
			},
			"no-console": {
				Severity: config.SeverityWarn,
				Pattern:  "console.log($$$ARGS)",
				Message:  "No console.log",
			},
			"no-fn": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Message:  "No functions",
			},
			"camelcase-fn": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Naming:   &config.NamingMatcher{Match: "^[a-z]", Message: "must be camelCase"},
			},
		},
	}
	eng, errs := New(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	result := eng.Lint(context.Background(), []string{filepath.Join(dir, "test.js")}, 1)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	rules := map[string]int{}
	for _, d := range result.Diagnostics {
		rules[d.Rule]++
	}

	if rules["no-var"] != 1 {
		t.Errorf("no-var: got %d, want 1", rules["no-var"])
	}
	if rules["no-console"] != 1 {
		t.Errorf("no-console: got %d, want 1", rules["no-console"])
	}
	// no-fn matches both Foo and bar but they're on different lines.
	if rules["no-fn"] != 2 {
		t.Errorf("no-fn: got %d, want 2", rules["no-fn"])
	}
	// camelcase-fn should only flag Foo (doesn't start lowercase).
	if rules["camelcase-fn"] != 1 {
		t.Errorf("camelcase-fn: got %d, want 1", rules["camelcase-fn"])
	}
}

func TestLint_AllFiveRuleTypes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "test.js"),
		"import React from \"react\";\nimport fs from \"fs\";\nvar x = 1;\nconsole.log(x);\nfunction Foo() {}\nfunction bar() {}")

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "No var",
			},
			"no-console": {
				Severity: config.SeverityWarn,
				Pattern:  "console.log($$$ARGS)",
				Message:  "No console.log",
			},
			"no-fn": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Message:  "No functions",
			},
			"camelcase-fn": {
				Severity: config.SeverityError,
				AST:      &config.ASTMatcher{Kind: "function_declaration"},
				Naming:   &config.NamingMatcher{Match: "^[a-z]", Message: "must be camelCase"},
			},
			"import-order": {
				Severity: config.SeverityWarn,
				Imports: &config.ImportsMatcher{
					Groups: []string{"builtin", "external"},
				},
				Message: "wrong import order",
			},
		},
	}
	eng, errs := New(cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	result := eng.Lint(context.Background(), []string{filepath.Join(dir, "test.js")}, 1)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	rules := map[string]int{}
	for _, d := range result.Diagnostics {
		rules[d.Rule]++
	}

	if rules["no-var"] != 1 {
		t.Errorf("no-var: got %d, want 1", rules["no-var"])
	}
	if rules["no-console"] != 1 {
		t.Errorf("no-console: got %d, want 1", rules["no-console"])
	}
	if rules["no-fn"] != 2 {
		t.Errorf("no-fn: got %d, want 2", rules["no-fn"])
	}
	if rules["camelcase-fn"] != 1 {
		t.Errorf("camelcase-fn: got %d, want 1", rules["camelcase-fn"])
	}
	if rules["import-order"] < 1 {
		t.Errorf("import-order: got %d, want >= 1", rules["import-order"])
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
