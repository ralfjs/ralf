package engine

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
)

const fixtureDir = "../../testdata/rules"

// expectErrorRe matches "// expect-error: <rule-name>" comments.
var expectErrorRe = regexp.MustCompile(`//\s*expect-error:\s*(\S+)`)

// expectErrorPrevLineRe matches "// expect-error-prev-line: <rule-name>"
// for cases where the diagnostic line can't hold a same-line comment
// (e.g. multiline string continuation with backslash-newline).
var expectErrorPrevLineRe = regexp.MustCompile(`//\s*expect-error-prev-line:\s*(\S+)`)

func TestFixtures(t *testing.T) {
	builtins := config.BuiltinRules()

	// Track which builtins have fixtures — fail if any are missing.
	tested := make(map[string]bool, len(builtins))

	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("read fixture dir: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ruleName := entry.Name()
		rule, ok := builtins[ruleName]
		if !ok {
			continue // skip non-builtin fixture dirs (e.g. no-console-pattern)
		}
		tested[ruleName] = true

		t.Run(ruleName, func(t *testing.T) {
			cfg := &config.Config{
				Rules: map[string]config.RuleConfig{
					ruleName: rule,
				},
			}
			eng, errs := New(cfg)
			if len(errs) > 0 {
				t.Fatalf("compile errors: %v", errs)
			}

			dir := filepath.Join(fixtureDir, ruleName)

			t.Run("invalid", func(t *testing.T) {
				path := filepath.Join(dir, "invalid.js")
				source, err := os.ReadFile(path) //nolint:gosec // test fixture
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}

				diags := eng.LintFile(context.Background(), path, source)
				expected := parseExpectErrors(string(source), ruleName)

				if len(expected) == 0 {
					t.Fatal("invalid.js has no expect-error annotations for this rule — fixture is ineffective")
				}

				if len(diags) != len(expected) {
					t.Errorf("got %d diagnostics, want %d", len(diags), len(expected))
					for _, d := range diags {
						t.Logf("  got: line %d rule %q", d.Line, d.Rule)
					}
					for _, e := range expected {
						t.Logf("  expected: line %d rule %q", e.line, e.rule)
					}
					return
				}

				for i, d := range diags {
					if d.Line != expected[i].line {
						t.Errorf("diag[%d]: line = %d, want %d", i, d.Line, expected[i].line)
					}
					if d.Rule != expected[i].rule {
						t.Errorf("diag[%d]: rule = %q, want %q", i, d.Rule, expected[i].rule)
					}
				}
			})

			t.Run("valid", func(t *testing.T) {
				path := filepath.Join(dir, "valid.js")
				source, err := os.ReadFile(path) //nolint:gosec // test fixture
				if err != nil {
					t.Fatalf("read fixture: %v", err)
				}

				diags := eng.LintFile(context.Background(), path, source)
				if len(diags) != 0 {
					t.Errorf("expected 0 diagnostics for valid.js, got %d", len(diags))
					for _, d := range diags {
						t.Logf("  line %d: %s (%s)", d.Line, d.Message, d.Rule)
					}
				}
			})
		})
	}

	// Fail if any builtin rule is missing fixture tests.
	for name := range builtins {
		if !tested[name] {
			t.Errorf("builtin rule %q has no fixture directory at testdata/rules/%s/", name, name)
		}
	}
}

type expectedError struct {
	line int
	rule string
}

// parseExpectErrors scans source for "// expect-error: <rule>" and
// "// expect-error-prev-line: <rule>" comments and returns the expected
// line numbers. Only errors matching ruleName are returned.
func parseExpectErrors(source, ruleName string) []expectedError {
	lines := strings.Split(source, "\n")
	var expected []expectedError
	for i, line := range lines {
		for _, m := range expectErrorRe.FindAllStringSubmatch(line, -1) {
			if m[1] == ruleName {
				expected = append(expected, expectedError{line: i + 1, rule: ruleName})
			}
		}
		for _, m := range expectErrorPrevLineRe.FindAllStringSubmatch(line, -1) {
			if m[1] == ruleName && i > 0 {
				expected = append(expected, expectedError{line: i, rule: ruleName})
			}
		}
	}
	return expected
}

func TestParseExpectErrors(t *testing.T) {
	source := "var x = 1; // expect-error: no-var\nlet y = 2;\nvar z = 3; // expect-error: no-var\n"
	got := parseExpectErrors(source, "no-var")
	if len(got) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(got))
	}
	if got[0].line != 1 {
		t.Errorf("first error line = %d, want 1", got[0].line)
	}
	if got[1].line != 3 {
		t.Errorf("second error line = %d, want 3", got[1].line)
	}
}
