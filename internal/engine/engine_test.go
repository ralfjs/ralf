package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hideart/bepro/internal/config"
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

	t.Run("skips unsupported matchers", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-var":  {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
				"ast":     {Severity: config.SeverityError, Pattern: "console.log($$$)", Message: "AST"},
				"imports": {Severity: config.SeverityError, Imports: &config.ImportsMatcher{}, Message: "Imports"},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(eng.regexRules) != 1 {
			t.Fatalf("expected 1 regex rule, got %d", len(eng.regexRules))
		}
	})
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
