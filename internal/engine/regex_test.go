package engine

import (
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

func TestCompileRegexRules(t *testing.T) {
	t.Run("valid patterns compile", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"no-var":     {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
			"no-console": {Severity: config.SeverityWarn, Regex: `console\.log`, Message: "No console"},
		}
		compiled, errs := compileRegexRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 2 {
			t.Fatalf("expected 2 compiled rules, got %d", len(compiled))
		}
	})

	t.Run("invalid pattern returns error", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"bad": {Severity: config.SeverityError, Regex: `[invalid`, Message: "Bad"},
		}
		compiled, errs := compileRegexRules(rules)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d", len(errs))
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled rules, got %d", len(compiled))
		}
	})

	t.Run("collects all errors", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"bad1": {Severity: config.SeverityError, Regex: `[a`, Message: "Bad1"},
			"bad2": {Severity: config.SeverityError, Regex: `[b`, Message: "Bad2"},
		}
		_, errs := compileRegexRules(rules)
		if len(errs) != 2 {
			t.Fatalf("expected 2 errors, got %d", len(errs))
		}
	})

	t.Run("skips off severity", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"off-rule": {Severity: config.SeverityOff, Regex: `\bvar\b`, Message: "Off"},
		}
		compiled, errs := compileRegexRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled rules, got %d", len(compiled))
		}
	})

	t.Run("skips non-regex rules", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"ast-rule": {Severity: config.SeverityError, Pattern: "console.log($$$)", Message: "AST"},
		}
		compiled, errs := compileRegexRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if len(compiled) != 0 {
			t.Fatalf("expected 0 compiled rules, got %d", len(compiled))
		}
	})
}

func TestMatchRegex(t *testing.T) {
	t.Run("single match", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
		}
		compiled, _ := compileRegexRules(rules)
		source := []byte("var x = 1;")
		lineStarts := buildLineIndex(source)

		diags := matchRegex(compiled[0], source, lineStarts, 0)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
		if diags[0].Line != 1 || diags[0].Col != 0 {
			t.Errorf("got line=%d col=%d, want line=1 col=0", diags[0].Line, diags[0].Col)
		}
	})

	t.Run("multiple matches across lines", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
		}
		compiled, _ := compileRegexRules(rules)
		source := []byte("var x = 1;\nvar y = 2;\nlet z = 3;")
		lineStarts := buildLineIndex(source)

		diags := matchRegex(compiled[0], source, lineStarts, 0)
		if len(diags) != 2 {
			t.Fatalf("expected 2 diagnostics, got %d", len(diags))
		}
	})

	t.Run("deduplicates per line", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"find-a": {Severity: config.SeverityWarn, Regex: `a`, Message: "Found a"},
		}
		compiled, _ := compileRegexRules(rules)
		source := []byte("aaa bbb aaa")
		lineStarts := buildLineIndex(source)

		diags := matchRegex(compiled[0], source, lineStarts, 0)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic (deduped), got %d", len(diags))
		}
	})

	t.Run("respects max matches", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"find-x": {Severity: config.SeverityError, Regex: `x`, Message: "Found x"},
		}
		compiled, _ := compileRegexRules(rules)
		// Each line has an x, but we cap at 2.
		source := []byte("x\nx\nx\nx\nx\n")
		lineStarts := buildLineIndex(source)

		diags := matchRegex(compiled[0], source, lineStarts, 2)
		if len(diags) != 2 {
			t.Fatalf("expected 2 diagnostics (capped), got %d", len(diags))
		}
	})

	t.Run("no matches", func(t *testing.T) {
		rules := map[string]config.RuleConfig{
			"no-var": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "No var"},
		}
		compiled, _ := compileRegexRules(rules)
		source := []byte("let x = 1;\nconst y = 2;")
		lineStarts := buildLineIndex(source)

		diags := matchRegex(compiled[0], source, lineStarts, 0)
		if len(diags) != 0 {
			t.Fatalf("expected 0 diagnostics, got %d", len(diags))
		}
	})

	t.Run("unicode word boundary via rure", func(t *testing.T) {
		// Verify rure-go engine handles Unicode word boundaries correctly.
		// Rust regex treats \b as Unicode-aware by default.
		rules := map[string]config.RuleConfig{
			"find-café": {Severity: config.SeverityError, Regex: `\bcafé\b`, Message: "Found café"},
		}
		compiled, errs := compileRegexRules(rules)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		source := []byte("I love café and coffee")
		lineStarts := buildLineIndex(source)

		diags := matchRegex(compiled[0], source, lineStarts, 0)
		if len(diags) != 1 {
			t.Fatalf("expected 1 diagnostic, got %d", len(diags))
		}
	})
}
