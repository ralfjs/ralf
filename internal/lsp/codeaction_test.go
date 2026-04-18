package lsp

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

// newHarnessWithConfig creates a test harness with a custom config.
func newHarnessWithConfig(t *testing.T, cfg *config.Config) *testHarness {
	t.Helper()

	eng, errs := engine.New(cfg)
	if len(errs) > 0 {
		t.Fatalf("engine init: %v", errs)
	}

	srvR, clientW := io.Pipe()
	clientR, srvW := io.Pipe()
	t.Cleanup(func() {
		_ = srvR.Close()
		_ = clientW.Close()
		_ = clientR.Close()
		_ = srvW.Close()
	})

	srv := NewServer(eng, cfg, nil)
	h := &testHarness{
		srv:    srv,
		client: NewTransport(clientR, clientW),
		done:   make(chan error, 1),
	}

	go func() {
		h.done <- srv.Run(context.Background(), srvR, srvW)
	}()

	return h
}

// fixConfig returns a config with a "no-var" rule that has a fix.
func fixConfig() *config.Config {
	return &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "Use let or const instead of var",
				Fix:      "let",
			},
		},
	}
}

// twoRuleFixConfig returns a config with two rules that both have fixes.
func twoRuleFixConfig() *config.Config {
	return &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {
				Severity: config.SeverityError,
				Regex:    `\bvar\b`,
				Message:  "Use let or const instead of var",
				Fix:      "let",
			},
			"no-alert": {
				Severity: config.SeverityWarn,
				Regex:    `\balert\b`,
				Message:  "Do not use alert",
				Fix:      "console.log",
			},
		},
	}
}

// openFileAndGetDiags opens a document and reads the publishDiagnostics notification.
func openFileAndGetDiags(t *testing.T, h *testHarness, uri, text string) PublishDiagnosticsParams {
	t.Helper()
	h.notifyWithParams(t, "textDocument/didOpen", DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: "javascript",
			Version:    1,
			Text:       text,
		},
	})
	msg := h.readMessageTimeout(t, 2*time.Second)
	if msg.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics, got %q", msg.Method)
	}
	var params PublishDiagnosticsParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		t.Fatalf("unmarshal diag params: %v", err)
	}
	return params
}

// requestCodeActions sends textDocument/codeAction and decodes the result.
func requestCodeActions(t *testing.T, h *testHarness, params *CodeActionParams) []CodeAction {
	t.Helper()
	resp := h.request(t, 2, "textDocument/codeAction", params)
	if resp.Error != nil {
		t.Fatalf("codeAction error: %s", resp.Error.Message)
	}
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	var actions []CodeAction
	if err := json.Unmarshal(raw, &actions); err != nil {
		t.Fatalf("unmarshal actions: %v", err)
	}
	return actions
}

func TestCodeAction_QuickFix_SingleDiag(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, fixConfig())
	h.initialize(t)

	diags := openFileAndGetDiags(t, h, "file:///tmp/fix.js", "var x = 1;\n")
	if len(diags.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic")
	}

	// Request code actions for the diagnostic.
	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/fix.js"},
		Range:        diags.Diagnostics[0].Range,
		Context: CodeActionContext{
			Diagnostics: diags.Diagnostics,
		},
	})

	if len(actions) == 0 {
		t.Fatal("expected at least one code action")
	}

	// First action should be a per-diagnostic quickfix.
	a := actions[0]
	if a.Kind != CodeActionQuickFix {
		t.Fatalf("expected kind quickfix, got %q", a.Kind)
	}
	if !a.IsPreferred {
		t.Fatal("expected IsPreferred to be true")
	}
	if a.Edit == nil {
		t.Fatal("expected edit to be non-nil")
	}

	edits := a.Edit.Changes["file:///tmp/fix.js"]
	if len(edits) != 1 {
		t.Fatalf("expected 1 text edit, got %d", len(edits))
	}
	if edits[0].NewText != "let" {
		t.Fatalf("expected NewText 'let', got %q", edits[0].NewText)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_NoDiagnostics(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, fixConfig())
	h.initialize(t)

	openFileAndGetDiags(t, h, "file:///tmp/clean.js", "const x = 1;\n")

	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/clean.js"},
		Range:        Range{Start: Position{0, 0}, End: Position{0, 5}},
		Context:      CodeActionContext{},
	})

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions for clean file, got %d", len(actions))
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_NoFix(t *testing.T) {
	t.Parallel()
	// Use the default harness (no-var rule without Fix).
	h := newHarness(t)
	h.initialize(t)

	diags := openFileAndGetDiags(t, h, "file:///tmp/nofix.js", "var x = 1;\n")
	if len(diags.Diagnostics) == 0 {
		t.Fatal("expected at least one diagnostic")
	}

	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/nofix.js"},
		Range:        diags.Diagnostics[0].Range,
		Context: CodeActionContext{
			Diagnostics: diags.Diagnostics,
		},
	})

	// No quick fixes expected — the rule has no Fix.
	quickFixes := 0
	for _, a := range actions {
		if a.Kind == CodeActionQuickFix {
			quickFixes++
		}
	}
	if quickFixes != 0 {
		t.Fatalf("expected 0 quickfix actions for rule without fix, got %d", quickFixes)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_MultipleDiags(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, fixConfig())
	h.initialize(t)

	diags := openFileAndGetDiags(t, h, "file:///tmp/multi.js", "var x = 1;\nvar y = 2;\n")
	if len(diags.Diagnostics) < 2 {
		t.Fatalf("expected at least 2 diagnostics, got %d", len(diags.Diagnostics))
	}

	// Request actions for the full range with both diagnostics.
	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/multi.js"},
		Range:        Range{Start: Position{0, 0}, End: Position{1, 10}},
		Context: CodeActionContext{
			Diagnostics: diags.Diagnostics,
		},
	})

	// Should have 2 per-diagnostic quick fixes + 1 "Fix all 'no-var' problems".
	quickFixes := 0
	ruleFixAll := 0
	for _, a := range actions {
		if a.Kind == CodeActionQuickFix {
			if a.Title == "Fix all 'no-var' problems" {
				ruleFixAll++
			} else {
				quickFixes++
			}
		}
	}

	if quickFixes != 2 {
		t.Fatalf("expected 2 per-diagnostic quick fixes, got %d", quickFixes)
	}
	if ruleFixAll != 1 {
		t.Fatalf("expected 1 'Fix all no-var' action, got %d", ruleFixAll)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_SourceFixAll(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, fixConfig())
	h.initialize(t)

	openFileAndGetDiags(t, h, "file:///tmp/fixall.js", "var x = 1;\nvar y = 2;\n")

	// Request only source.fixAll actions.
	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/fixall.js"},
		Range:        Range{Start: Position{0, 0}, End: Position{1, 10}},
		Context: CodeActionContext{
			Only: []CodeActionKind{CodeActionSourceFixAll},
		},
	})

	if len(actions) != 1 {
		t.Fatalf("expected 1 source.fixAll action, got %d", len(actions))
	}

	a := actions[0]
	if a.Kind != CodeActionSourceFixAll {
		t.Fatalf("expected kind source.fixAll, got %q", a.Kind)
	}
	if a.Edit == nil {
		t.Fatal("expected edit to be non-nil")
	}

	edits := a.Edit.Changes["file:///tmp/fixall.js"]
	if len(edits) != 2 {
		t.Fatalf("expected 2 text edits (one per var), got %d", len(edits))
	}

	for _, e := range edits {
		if e.NewText != "let" {
			t.Fatalf("expected NewText 'let', got %q", e.NewText)
		}
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_FilterByOnly(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, fixConfig())
	h.initialize(t)

	diags := openFileAndGetDiags(t, h, "file:///tmp/only.js", "var x = 1;\nvar y = 2;\n")

	// Request only quickfix — should NOT include source.fixAll.
	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/only.js"},
		Range:        Range{Start: Position{0, 0}, End: Position{1, 10}},
		Context: CodeActionContext{
			Diagnostics: diags.Diagnostics,
			Only:        []CodeActionKind{CodeActionQuickFix},
		},
	})

	for _, a := range actions {
		if a.Kind == CodeActionSourceFixAll {
			t.Fatal("source.fixAll should not be present when Only=quickfix")
		}
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_TwoRules(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, twoRuleFixConfig())
	h.initialize(t)

	diags := openFileAndGetDiags(t, h, "file:///tmp/two.js", "var x = alert(1);\n")
	if len(diags.Diagnostics) < 2 {
		t.Fatalf("expected at least 2 diagnostics, got %d", len(diags.Diagnostics))
	}

	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/two.js"},
		Range:        Range{Start: Position{0, 0}, End: Position{0, 17}},
		Context: CodeActionContext{
			Diagnostics: diags.Diagnostics,
		},
	})

	// Should have at least 2 per-diagnostic quick fixes (one for each rule).
	quickFixes := 0
	for _, a := range actions {
		if a.Kind == CodeActionQuickFix {
			quickFixes++
		}
	}
	if quickFixes < 2 {
		t.Fatalf("expected at least 2 quick fixes for 2 rules, got %d", quickFixes)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_RangeOverlap(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, fixConfig())
	h.initialize(t)

	openFileAndGetDiags(t, h, "file:///tmp/range.js", "var x = 1;\nconst y = 2;\nvar z = 3;\n")

	// Request actions for line 1 only (const y = 2) — no diagnostic.
	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/range.js"},
		Range:        Range{Start: Position{1, 0}, End: Position{1, 12}},
		Context:      CodeActionContext{},
	})

	quickFixes := 0
	for _, a := range actions {
		if a.Kind == CodeActionQuickFix {
			quickFixes++
		}
	}
	if quickFixes != 0 {
		t.Fatalf("expected 0 quick fixes for line with no diagnostic, got %d", quickFixes)
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestCodeAction_ClosedFile(t *testing.T) {
	t.Parallel()
	h := newHarnessWithConfig(t, fixConfig())
	h.initialize(t)

	openFileAndGetDiags(t, h, "file:///tmp/closed.js", "var x = 1;\n")

	// Close the file.
	h.notifyWithParams(t, "textDocument/didClose", DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/closed.js"},
	})
	// Read the clear-diagnostics notification.
	h.readMessageTimeout(t, 2*time.Second)

	// Request actions on a closed file.
	actions := requestCodeActions(t, h, &CodeActionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///tmp/closed.js"},
		Range:        Range{Start: Position{0, 0}, End: Position{0, 10}},
		Context:      CodeActionContext{},
	})

	if len(actions) != 0 {
		t.Fatalf("expected 0 actions for closed file, got %d", len(actions))
	}

	h.notify(t, "exit")
	h.wait(t)
}

func TestFixToTextEdit(t *testing.T) {
	t.Parallel()

	source := []byte("var x = 1;\n")
	lineStarts := buildLineIndex(source)

	fix := &engine.Fix{StartByte: 0, EndByte: 3, NewText: "let"}
	te := fixToTextEdit(fix, source, lineStarts)

	if te.Range.Start.Line != 0 || te.Range.Start.Character != 0 {
		t.Fatalf("expected start (0,0), got (%d,%d)", te.Range.Start.Line, te.Range.Start.Character)
	}
	if te.Range.End.Line != 0 || te.Range.End.Character != 3 {
		t.Fatalf("expected end (0,3), got (%d,%d)", te.Range.End.Line, te.Range.End.Character)
	}
	if te.NewText != "let" {
		t.Fatalf("expected NewText 'let', got %q", te.NewText)
	}
}

func TestByteOffsetToPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		offset int
		want   Position
	}{
		{"start", "hello\n", 0, Position{0, 0}},
		{"mid-line", "hello\n", 3, Position{0, 3}},
		{"second-line", "hello\nworld\n", 6, Position{1, 0}},
		{"second-line-mid", "hello\nworld\n", 9, Position{1, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			source := []byte(tt.source)
			lineStarts := buildLineIndex(source)
			got := byteOffsetToPosition(source, lineStarts, tt.offset)
			if got != tt.want {
				t.Fatalf("byteOffsetToPosition(%d) = %+v, want %+v", tt.offset, got, tt.want)
			}
		})
	}
}

func TestByteOffsetToPosition_UTF16(t *testing.T) {
	t.Parallel()

	// Multi-byte: "é" is 2 bytes in UTF-8, 1 code unit in UTF-16.
	source := []byte("éb\n")
	lineStarts := buildLineIndex(source)

	// Byte offset 2 = after "é" (2 bytes), should be UTF-16 character 1.
	pos := byteOffsetToPosition(source, lineStarts, 2)
	if pos.Character != 1 {
		t.Fatalf("expected UTF-16 character 1, got %d", pos.Character)
	}

	// Byte offset 3 = after "éb", should be UTF-16 character 2.
	pos = byteOffsetToPosition(source, lineStarts, 3)
	if pos.Character != 2 {
		t.Fatalf("expected UTF-16 character 2, got %d", pos.Character)
	}
}

func TestKindAllowed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind CodeActionKind
		only []CodeActionKind
		want bool
	}{
		{"empty filter", CodeActionQuickFix, nil, true},
		{"exact match", CodeActionQuickFix, []CodeActionKind{CodeActionQuickFix}, true},
		{"no match", CodeActionSourceFixAll, []CodeActionKind{CodeActionQuickFix}, false},
		{"prefix match", CodeActionSourceFixAll, []CodeActionKind{"source"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kindAllowed(tt.kind, tt.only); got != tt.want {
				t.Fatalf("kindAllowed(%q, %v) = %v, want %v", tt.kind, tt.only, got, tt.want)
			}
		})
	}
}

func TestRangesOverlap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a, b Range
		want bool
	}{
		{
			"overlap", Range{Position{0, 0}, Position{0, 5}}, Range{Position{0, 3}, Position{0, 8}}, true,
		},
		{
			"no overlap", Range{Position{0, 0}, Position{0, 5}}, Range{Position{1, 0}, Position{1, 5}}, false,
		},
		{
			"touching", Range{Position{0, 0}, Position{0, 5}}, Range{Position{0, 5}, Position{0, 10}}, false,
		},
		{
			"contained", Range{Position{0, 0}, Position{0, 10}}, Range{Position{0, 3}, Position{0, 7}}, true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := rangesOverlap(tt.a, tt.b); got != tt.want {
				t.Fatalf("rangesOverlap = %v, want %v", got, tt.want)
			}
		})
	}
}
