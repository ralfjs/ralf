package engine

import (
	"context"
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

func TestParseRuleList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty string", "", []string{""}},
		{"whitespace only", "   ", []string{""}},
		{"single rule", "no-console", []string{"no-console"}},
		{"two rules", "no-console, no-var", []string{"no-console", "no-var"}},
		{"whitespace trimmed", " no-console , no-var ", []string{"no-console", "no-var"}},
		{"trailing comma", "no-console,", []string{"no-console"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseRuleList(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("parseRuleList(%q) = %v, want %v", tt.raw, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseRuleList(%q)[%d] = %q, want %q", tt.raw, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseSuppressComments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		wantFile   map[string]bool
		wantLines  map[int]map[string]bool
		wantBlocks int
	}{
		{
			name:     "next-line single rule",
			source:   "// lint-disable-next-line no-console\nconsole.log('hi');",
			wantFile: map[string]bool{},
			wantLines: map[int]map[string]bool{
				2: {"no-console": true},
			},
		},
		{
			name:     "next-line all rules",
			source:   "// lint-disable-next-line\nconsole.log('hi');",
			wantFile: map[string]bool{},
			wantLines: map[int]map[string]bool{
				2: {"": true},
			},
		},
		{
			name:      "same-line single rule",
			source:    "console.log('hi'); // lint-disable no-console",
			wantFile:  map[string]bool{},
			wantLines: map[int]map[string]bool{1: {"no-console": true}},
		},
		{
			name:      "same-line multiple rules",
			source:    "var x = console.log('hi'); // lint-disable no-console, no-var",
			wantFile:  map[string]bool{},
			wantLines: map[int]map[string]bool{1: {"no-console": true, "no-var": true}},
		},
		{
			name:       "block disable/enable single rule",
			source:     "// lint-disable no-console\nconsole.log(1);\nconsole.log(2);\n// lint-enable no-console",
			wantFile:   map[string]bool{},
			wantBlocks: 1,
		},
		{
			name:       "block unclosed extends to EOF",
			source:     "// lint-disable no-var\nvar x = 1;\nvar y = 2;",
			wantFile:   map[string]bool{},
			wantBlocks: 1,
		},
		{
			name:     "file-level with rules",
			source:   "/* lint-disable-file no-console */\nconsole.log('hi');",
			wantFile: map[string]bool{"no-console": true},
		},
		{
			name:     "file-level without rules",
			source:   "/* lint-disable-file */\nconsole.log('hi');",
			wantFile: map[string]bool{"": true},
		},
		{
			name:     "block comment style disable-file",
			source:   "/* lint-disable-file no-var, no-console */\nvar x = 1;",
			wantFile: map[string]bool{"no-var": true, "no-console": true},
		},
		{
			name:     "enable with no matching disable ignored",
			source:   "// lint-enable no-console\nconsole.log('hi');",
			wantFile: map[string]bool{},
		},
		{
			name:     "mixed directives in one file",
			source:   "/* lint-disable-file no-eval */\n// lint-disable-next-line no-console\nconsole.log(1);\nvar x = 1; // lint-disable no-var\n// lint-disable no-debugger\ndebugger;\n// lint-enable no-debugger",
			wantFile: map[string]bool{"no-eval": true},
			wantLines: map[int]map[string]bool{
				3: {"no-console": true},
				4: {"no-var": true},
			},
			wantBlocks: 1,
		},
		{
			name:       "nested blocks same rule",
			source:     "// lint-disable no-var\n// lint-disable no-var\nvar x;\n// lint-enable no-var\nvar y;\n// lint-enable no-var",
			wantFile:   map[string]bool{},
			wantBlocks: 2,
		},
		{
			name:       "overlapping blocks different rules",
			source:     "// lint-disable no-var\n// lint-disable no-console\nvar x;\nconsole.log(x);\n// lint-enable no-var\nconsole.log(1);\n// lint-enable no-console",
			wantFile:   map[string]bool{},
			wantBlocks: 2,
		},
		{
			name:     "line comment disable-file",
			source:   "// lint-disable-file no-console\nconsole.log('hi');",
			wantFile: map[string]bool{"no-console": true},
		},
		{
			name:     "next-line multiple rules",
			source:   "// lint-disable-next-line no-console, no-var\nvar x = console.log(1);",
			wantFile: map[string]bool{},
			wantLines: map[int]map[string]bool{
				2: {"no-console": true, "no-var": true},
			},
		},
		{
			name:       "block all rules",
			source:     "// lint-disable\nvar x;\nconsole.log(x);\n// lint-enable",
			wantFile:   map[string]bool{},
			wantBlocks: 1,
		},
		{
			name:       "bare enable closes specific-rule blocks",
			source:     "// lint-disable no-console\nconsole.log(1);\n// lint-enable",
			wantFile:   map[string]bool{},
			wantBlocks: 1,
		},
		{
			name:       "bare enable closes multiple specific-rule blocks",
			source:     "// lint-disable no-console\n// lint-disable no-var\nvar x;\n// lint-enable",
			wantFile:   map[string]bool{},
			wantBlocks: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSuppressComments([]byte(tt.source))

			if len(got.file) != len(tt.wantFile) {
				t.Errorf("file suppressions: got %v, want %v", got.file, tt.wantFile)
			}
			for k, v := range tt.wantFile {
				if got.file[k] != v {
					t.Errorf("file[%q] = %v, want %v", k, got.file[k], v)
				}
			}

			if len(got.lines) != len(tt.wantLines) {
				t.Errorf("line suppressions: got %d lines, want %d", len(got.lines), len(tt.wantLines))
			}
			for line, wantRules := range tt.wantLines {
				gotRules := got.lines[line]
				if gotRules == nil {
					t.Errorf("lines[%d] is nil, want %v", line, wantRules)
					continue
				}
				if len(gotRules) != len(wantRules) {
					t.Errorf("lines[%d]: got %d rules %v, want %d rules %v", line, len(gotRules), gotRules, len(wantRules), wantRules)
					continue
				}
				for r, v := range wantRules {
					if gotRules[r] != v {
						t.Errorf("lines[%d][%q] = %v, want %v", line, r, gotRules[r], v)
					}
				}
			}

			if len(got.blocks) != tt.wantBlocks {
				t.Errorf("blocks: got %d, want %d", len(got.blocks), tt.wantBlocks)
			}
		})
	}
}

func TestIsSuppressed(t *testing.T) {
	t.Parallel()

	sup := suppressions{
		file:  map[string]bool{"file-rule": true},
		lines: map[int]map[string]bool{5: {"no-var": true}, 10: {"": true}},
		blocks: []blockRange{
			{startLine: 20, endLine: 30, rule: "no-console"},
			{startLine: 40, endLine: 50, rule: ""},
		},
	}

	tests := []struct {
		name string
		line int
		rule string
		want bool
	}{
		{"file-level match", 1, "file-rule", true},
		{"file-level no match", 1, "other", false},
		{"line-level specific rule match", 5, "no-var", true},
		{"line-level specific rule no match", 5, "other", false},
		{"line-level all rules", 10, "anything", true},
		{"block match inside range", 25, "no-console", true},
		{"block no match wrong rule", 25, "no-var", false},
		{"block no match outside range", 15, "no-console", false},
		{"block all rules match", 45, "anything", true},
		{"no suppression", 100, "some-rule", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isSuppressed(&sup, tt.line, tt.rule)
			if got != tt.want {
				t.Errorf("isSuppressed(line=%d, rule=%q) = %v, want %v", tt.line, tt.rule, got, tt.want)
			}
		})
	}
}

func TestFilterSuppressed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		diags     []Diagnostic
		sup       suppressions
		wantCount int
		wantRules []string // rules of remaining diagnostics
	}{
		{
			name: "mixed suppression types",
			diags: []Diagnostic{
				{Line: 1, Rule: "no-var", Message: "a"},
				{Line: 2, Rule: "no-console", Message: "b"},
				{Line: 3, Rule: "no-eval", Message: "c"},
			},
			sup: suppressions{
				file:  map[string]bool{},
				lines: map[int]map[string]bool{1: {"no-var": true}},
				blocks: []blockRange{
					{startLine: 3, endLine: 3, rule: "no-eval"},
				},
			},
			wantCount: 1,
			wantRules: []string{"no-console"},
		},
		{
			name: "file-level all suppresses everything",
			diags: []Diagnostic{
				{Line: 1, Rule: "no-var"},
				{Line: 5, Rule: "no-console"},
				{Line: 10, Rule: "no-eval"},
			},
			sup: suppressions{
				file:  map[string]bool{"": true},
				lines: map[int]map[string]bool{},
			},
			wantCount: 0,
		},
		{
			name:  "empty diagnostics",
			diags: nil,
			sup: suppressions{
				file:  map[string]bool{"": true},
				lines: map[int]map[string]bool{},
			},
			wantCount: 0,
		},
		{
			name: "no suppressions keeps all",
			diags: []Diagnostic{
				{Line: 1, Rule: "no-var"},
				{Line: 2, Rule: "no-console"},
			},
			sup: suppressions{
				file:  map[string]bool{},
				lines: map[int]map[string]bool{},
			},
			wantCount: 2,
			wantRules: []string{"no-var", "no-console"},
		},
		{
			name: "block range boundary inclusive",
			diags: []Diagnostic{
				{Line: 10, Rule: "no-var"},
				{Line: 11, Rule: "no-var"},
				{Line: 20, Rule: "no-var"},
				{Line: 21, Rule: "no-var"},
			},
			sup: suppressions{
				file:  map[string]bool{},
				lines: map[int]map[string]bool{},
				blocks: []blockRange{
					{startLine: 11, endLine: 20, rule: "no-var"},
				},
			},
			wantCount: 2,
			wantRules: []string{"no-var", "no-var"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Copy so we don't mutate across subtests.
			diags := make([]Diagnostic, len(tt.diags))
			copy(diags, tt.diags)

			got := filterSuppressed(diags, tt.sup)
			if len(got) != tt.wantCount {
				t.Fatalf("got %d diagnostics, want %d", len(got), tt.wantCount)
			}
			for i, r := range tt.wantRules {
				if got[i].Rule != r {
					t.Errorf("got[%d].Rule = %q, want %q", i, got[i].Rule, r)
				}
			}
		})
	}
}

func TestLintFileWithSuppression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		source    string
		wantDiags int
	}{
		{
			name:      "same-line suppression",
			source:    "var x = 1; // lint-disable use-let",
			wantDiags: 0,
		},
		{
			name:      "next-line suppression",
			source:    "// lint-disable-next-line use-let\nvar x = 1;",
			wantDiags: 0,
		},
		{
			name:      "file-level suppression all rules",
			source:    "/* lint-disable-file */\nvar x = 1;\nvar y = 2;",
			wantDiags: 0,
		},
		{
			name:      "file-level suppression specific rule",
			source:    "/* lint-disable-file use-let */\nvar x = 1;\nvar y = 2;\nvar z = 3;",
			wantDiags: 0,
		},
		{
			name:      "block suppression",
			source:    "// lint-disable use-let\nvar x = 1;\nvar y = 2;\n// lint-enable use-let\nvar z = 3;",
			wantDiags: 1, // only var z on line 5
		},
		{
			name:      "non-matching suppression still produces diagnostics",
			source:    "var x = 1; // lint-disable no-console",
			wantDiags: 1, // use-let not suppressed
		},
		{
			name:      "next-line all rules suppression",
			source:    "// lint-disable-next-line\nvar x = 1;",
			wantDiags: 0,
		},
		{
			name:      "same-line all rules suppression",
			source:    "var x = 1; // lint-disable",
			wantDiags: 0,
		},
		{
			name:      "unclosed block extends to EOF",
			source:    "// lint-disable use-let\nvar x = 1;\nvar y = 2;\nvar z = 3;",
			wantDiags: 0,
		},
		{
			name:      "block only suppresses inside range",
			source:    "var a = 1;\n// lint-disable use-let\nvar b = 2;\n// lint-enable use-let\nvar c = 3;",
			wantDiags: 2, // var a on line 1 and var c on line 5
		},
		{
			name:      "multiple next-line directives",
			source:    "// lint-disable-next-line use-let\nvar x = 1;\nvar y = 2;\n// lint-disable-next-line use-let\nvar z = 3;",
			wantDiags: 1, // var y on line 3
		},
		{
			name:      "next-line does not suppress two lines later",
			source:    "// lint-disable-next-line use-let\nlet x = 1;\nvar y = 2;",
			wantDiags: 1, // var y on line 3, next-line only covers line 2
		},
		{
			name:      "bare enable closes specific-rule block",
			source:    "// lint-disable use-let\nvar x = 1;\n// lint-enable\nvar y = 2;",
			wantDiags: 1, // var y on line 4, only after enable
		},
	}

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"use-let": {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "Use let or const"},
		},
	}
	eng, errs := New(cfg)
	if len(errs) > 0 {
		t.Fatalf("engine creation failed: %v", errs)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			diags := eng.LintFile(context.Background(), "test.js", []byte(tt.source))
			if len(diags) != tt.wantDiags {
				t.Errorf("got %d diagnostics, want %d\ndiags: %v", len(diags), tt.wantDiags, diags)
			}
		})
	}
}

// TestLintFileWithSuppression_MultiRule verifies suppression works correctly
// when multiple rules are active and only specific ones are suppressed.
func TestLintFileWithSuppression_MultiRule(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"use-let":     {Severity: config.SeverityError, Regex: `\bvar\b`, Message: "Use let or const"},
			"no-debugger": {Severity: config.SeverityError, Regex: `\bdebugger\b`, Message: "No debugger"},
		},
	}
	eng, errs := New(cfg)
	if len(errs) > 0 {
		t.Fatalf("engine creation failed: %v", errs)
	}

	tests := []struct {
		name      string
		source    string
		wantDiags int
		wantRules []string
	}{
		{
			name:      "suppress one rule, other still fires",
			source:    "var x = 1; // lint-disable use-let\ndebugger;",
			wantDiags: 1,
			wantRules: []string{"no-debugger"},
		},
		{
			name:      "suppress all rules on line",
			source:    "var x = 1; // lint-disable\ndebugger; // lint-disable",
			wantDiags: 0,
		},
		{
			name:      "file-level suppress one rule",
			source:    "/* lint-disable-file use-let */\nvar x = 1;\ndebugger;",
			wantDiags: 1,
			wantRules: []string{"no-debugger"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			diags := eng.LintFile(context.Background(), "test.js", []byte(tt.source))
			if len(diags) != tt.wantDiags {
				t.Errorf("got %d diagnostics, want %d\ndiags: %v", len(diags), tt.wantDiags, diags)
			}
			for i, r := range tt.wantRules {
				if i < len(diags) && diags[i].Rule != r {
					t.Errorf("diag[%d].Rule = %q, want %q", i, diags[i].Rule, r)
				}
			}
		})
	}
}

// TestParseSuppressComments_BlockRanges verifies block range start/end lines
// are set correctly for various scenarios.
func TestParseSuppressComments_BlockRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		wantBlocks []blockRange
	}{
		{
			name:   "closed block",
			source: "// lint-disable no-var\nvar x;\n// lint-enable no-var",
			wantBlocks: []blockRange{
				{startLine: 1, endLine: 3, rule: "no-var"},
			},
		},
		{
			name:   "unclosed extends to last line",
			source: "// lint-disable no-var\nvar x;\nvar y;",
			wantBlocks: []blockRange{
				{startLine: 1, endLine: 3, rule: "no-var"},
			},
		},
		{
			name:   "block all rules closed",
			source: "// lint-disable\nvar x;\n// lint-enable",
			wantBlocks: []blockRange{
				{startLine: 1, endLine: 3, rule: ""},
			},
		},
		{
			name:   "multiple blocks non-overlapping",
			source: "// lint-disable no-var\nvar x;\n// lint-enable no-var\nlet y;\n// lint-disable no-console\nconsole.log(1);\n// lint-enable no-console",
			wantBlocks: []blockRange{
				{startLine: 1, endLine: 3, rule: "no-var"},
				{startLine: 5, endLine: 7, rule: "no-console"},
			},
		},
		{
			name:   "bare enable closes specific-rule block",
			source: "// lint-disable no-var\nvar x;\n// lint-enable",
			wantBlocks: []blockRange{
				{startLine: 1, endLine: 3, rule: "no-var"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSuppressComments([]byte(tt.source))
			if len(got.blocks) != len(tt.wantBlocks) {
				t.Fatalf("got %d blocks, want %d: %+v", len(got.blocks), len(tt.wantBlocks), got.blocks)
			}
			for i, want := range tt.wantBlocks {
				b := got.blocks[i]
				if b.startLine != want.startLine || b.endLine != want.endLine || b.rule != want.rule {
					t.Errorf("block[%d] = %+v, want %+v", i, b, want)
				}
			}
		})
	}
}
