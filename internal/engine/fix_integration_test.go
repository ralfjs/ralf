package engine

import (
	"context"
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

func TestFix_RegexRule(t *testing.T) {
	t.Run("regex fix replaces match", func(t *testing.T) {
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
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("var x = 1;\nlet y = 2;\nvar z = 3;")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 2 {
			t.Fatalf("expected 2 diagnostics, got %d", len(diags))
		}

		// Collect fixes.
		fixes := make([]Fix, 0, len(diags))
		for _, d := range diags {
			if d.Fix == nil {
				t.Fatal("expected fix on diagnostic")
			}
			fixes = append(fixes, *d.Fix)
		}

		result, conflicts := ApplyFixes(source, fixes)
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}

		want := "let x = 1;\nlet y = 2;\nlet z = 3;"
		if string(result) != want {
			t.Errorf("got %q, want %q", result, want)
		}
	})

	t.Run("regex delete-statement fix", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-debugger": {
					Severity: config.SeverityError,
					Regex:    `\bdebugger\b`,
					Message:  "No debugger",
					Fix:      "delete-statement",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("let x = 1;\n  debugger;\nlet y = 2;\n")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}

		if diags[0].Fix == nil {
			t.Fatal("expected fix on diagnostic")
		}

		result, conflicts := ApplyFixes(source, []Fix{*diags[0].Fix})
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}

		want := "let x = 1;\nlet y = 2;\n"
		if string(result) != want {
			t.Errorf("got %q, want %q", result, want)
		}
	})

	t.Run("regex empty fix deletes match", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"rm-word": {
					Severity: config.SeverityError,
					Regex:    `\bdebugger\b`,
					Message:  "Remove debugger",
					Fix:      "",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("debugger;")
		diags := eng.LintFile(context.Background(), "test.js", source)

		// Empty fix string means "no fix" (not "delete").
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Fix != nil {
			t.Error("expected nil fix for empty fix string")
		}
	})
}

func TestFix_PatternRule(t *testing.T) {
	t.Run("pattern fix replaces match", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"no-console": {
					Severity: config.SeverityWarn,
					Pattern:  "console.log($$$ARGS)",
					Message:  "No console.log",
					Fix:      "// removed",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("console.log(\"test\");\nlet x = 1;")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Fix == nil {
			t.Fatal("expected fix on diagnostic")
		}

		result, conflicts := ApplyFixes(source, []Fix{*diags[0].Fix})
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}

		want := "// removed;\nlet x = 1;"
		if string(result) != want {
			t.Errorf("got %q, want %q", result, want)
		}
	})

	t.Run("pattern fix with capture substitution", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"log-to-debug": {
					Severity: config.SeverityWarn,
					Pattern:  "console.log($$$ARGS)",
					Message:  "Use debug instead",
					Fix:      "console.debug($$$ARGS)",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("console.log(\"hello\", x);")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Fix == nil {
			t.Fatal("expected fix on diagnostic")
		}

		result, conflicts := ApplyFixes(source, []Fix{*diags[0].Fix})
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}

		want := `console.debug("hello", x);`
		if string(result) != want {
			t.Errorf("got %q, want %q", result, want)
		}
	})

	t.Run("pattern delete-statement fix", func(t *testing.T) {
		cfg := &config.Config{
			Rules: map[string]config.RuleConfig{
				"rm-console": {
					Severity: config.SeverityWarn,
					Pattern:  "console.log($$$ARGS)",
					Message:  "Remove console.log",
					Fix:      "delete-statement",
				},
			},
		}
		eng, errs := New(cfg)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}

		source := []byte("let x = 1;\nconsole.log(x);\nlet y = 2;\n")
		diags := eng.LintFile(context.Background(), "test.js", source)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Fix == nil {
			t.Fatal("expected fix on diagnostic")
		}

		result, conflicts := ApplyFixes(source, []Fix{*diags[0].Fix})
		if len(conflicts) != 0 {
			t.Fatalf("unexpected conflicts: %v", conflicts)
		}

		want := "let x = 1;\nlet y = 2;\n"
		if string(result) != want {
			t.Errorf("got %q, want %q", result, want)
		}
	})
}
