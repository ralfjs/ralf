package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/parser"
)

func (s *Server) handleCodeAction(req *Request) {
	var params CodeActionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req, CodeInvalidParams, "invalid codeAction params")
		return
	}

	path := URIToPath(params.TextDocument.URI)
	uri := PathToURI(path)

	content, ok := s.docs.Get(path)
	if !ok {
		s.sendResult(req, []CodeAction{})
		return
	}

	cached := s.getCachedLint(path, content)
	if cached == nil {
		s.sendResult(req, []CodeAction{})
		return
	}

	actions := s.buildCodeActions(cached, &params, uri)
	s.sendResult(req, actions)
}

// getCachedLint returns the cached lint result for the path if it matches the
// current document content. If the cache is stale, it re-lints synchronously.
func (s *Server) getCachedLint(path string, content []byte) *cachedLint {
	s.cacheMu.Lock()
	cl, ok := s.lintCache[path]
	s.cacheMu.Unlock()

	if ok && bytes.Equal(cl.source, content) {
		return cl
	}

	// Cache miss or stale — re-lint synchronously.
	if _, ok := parser.LangFromPath(path); !ok {
		return nil
	}
	engineDiags := s.eng.LintFile(s.ctx, path, content)
	lineStarts := buildLineIndex(content)
	lspDiags := convertDiagnosticsWithIndex(engineDiags, content, lineStarts)

	cl = &cachedLint{
		engineDiags: engineDiags,
		lspDiags:    lspDiags,
		source:      content,
		lineStarts:  lineStarts,
	}

	s.cacheMu.Lock()
	s.lintCache[path] = cl
	s.cacheMu.Unlock()

	return cl
}

// buildCodeActions produces code actions from cached lint results.
func (s *Server) buildCodeActions(cl *cachedLint, params *CodeActionParams, uri string) []CodeAction {
	wantQuickFix := kindAllowed(CodeActionQuickFix, params.Context.Only)
	wantFixAll := kindAllowed(CodeActionSourceFixAll, params.Context.Only)

	var actions []CodeAction

	if wantQuickFix {
		actions = append(actions, s.quickFixActions(cl, params, uri)...)
	}

	if wantFixAll {
		if a, ok := s.fixAllAction(cl, uri); ok {
			actions = append(actions, a)
		}
	}

	if len(actions) == 0 {
		return []CodeAction{}
	}
	return actions
}

// quickFixActions returns per-diagnostic quick fixes and per-rule fix-all actions.
func (s *Server) quickFixActions(cl *cachedLint, params *CodeActionParams, uri string) []CodeAction {
	// Find engine diagnostics that match the client-sent diagnostics.
	type match struct {
		engineIdx  int
		clientDiag LDiagnostic
	}

	var matches []match
	if len(params.Context.Diagnostics) > 0 {
		// Match by Code (rule) + Range.
		for _, cd := range params.Context.Diagnostics {
			for i, ld := range cl.lspDiags {
				if ld.Code == cd.Code && ld.Range == cd.Range {
					matches = append(matches, match{engineIdx: i, clientDiag: cd})
					break
				}
			}
		}
	} else {
		// No diagnostics sent — use all diagnostics that overlap the range.
		for i, ld := range cl.lspDiags {
			if rangesOverlap(ld.Range, params.Range) {
				matches = append(matches, match{engineIdx: i, clientDiag: ld})
			}
		}
	}

	var actions []CodeAction

	// Per-diagnostic quick fixes.
	ruleFixCount := make(map[string]int) // rule → number of fixable diags in file
	for i := range cl.engineDiags {
		if cl.engineDiags[i].Fix != nil {
			ruleFixCount[cl.engineDiags[i].Rule]++
		}
	}

	seenRules := make(map[string]bool)
	for _, m := range matches {
		ed := &cl.engineDiags[m.engineIdx]
		if ed.Fix == nil {
			continue
		}

		te := fixToTextEdit(ed.Fix, cl.source, cl.lineStarts)
		actions = append(actions, CodeAction{
			Title:       fmt.Sprintf("Fix: %s", ed.Message),
			Kind:        CodeActionQuickFix,
			Diagnostics: []LDiagnostic{m.clientDiag},
			IsPreferred: true,
			Edit: &WorkspaceEdit{
				Changes: map[string][]TextEdit{uri: {te}},
			},
		})

		// Per-rule fix-all (only if >1 fixable diag for this rule).
		if !seenRules[ed.Rule] && ruleFixCount[ed.Rule] > 1 {
			seenRules[ed.Rule] = true
			if a, ok := s.ruleFixAllAction(cl, ed.Rule, uri); ok {
				actions = append(actions, a)
			}
		}
	}

	return actions
}

// ruleFixAllAction builds a "Fix all '<rule>' problems" action for a single rule.
func (s *Server) ruleFixAllAction(cl *cachedLint, rule, uri string) (CodeAction, bool) {
	var fixes []engine.Fix
	var diags []LDiagnostic
	for i := range cl.engineDiags {
		if cl.engineDiags[i].Rule == rule && cl.engineDiags[i].Fix != nil {
			fixes = append(fixes, *cl.engineDiags[i].Fix)
			diags = append(diags, cl.lspDiags[i])
		}
	}

	if len(fixes) == 0 {
		return CodeAction{}, false
	}

	edits := applyFixesAsEdits(cl.source, cl.lineStarts, fixes)
	if len(edits) == 0 {
		return CodeAction{}, false
	}

	return CodeAction{
		Title:       fmt.Sprintf("Fix all '%s' problems", rule),
		Kind:        CodeActionQuickFix,
		Diagnostics: diags,
		Edit: &WorkspaceEdit{
			Changes: map[string][]TextEdit{uri: edits},
		},
	}, true
}

// fixAllAction builds a source.fixAll action applying all non-conflicting fixes.
func (s *Server) fixAllAction(cl *cachedLint, uri string) (CodeAction, bool) {
	var fixes []engine.Fix
	var diags []LDiagnostic
	for i := range cl.engineDiags {
		if cl.engineDiags[i].Fix != nil {
			fixes = append(fixes, *cl.engineDiags[i].Fix)
			diags = append(diags, cl.lspDiags[i])
		}
	}

	if len(fixes) == 0 {
		return CodeAction{}, false
	}

	edits := applyFixesAsEdits(cl.source, cl.lineStarts, fixes)
	if len(edits) == 0 {
		return CodeAction{}, false
	}

	return CodeAction{
		Title:       "Fix all auto-fixable problems",
		Kind:        CodeActionSourceFixAll,
		Diagnostics: diags,
		Edit: &WorkspaceEdit{
			Changes: map[string][]TextEdit{uri: edits},
		},
	}, true
}

// applyFixesAsEdits runs engine.ApplyFixes to resolve conflicts, then converts
// the non-conflicting fixes to TextEdits.
func applyFixesAsEdits(source []byte, lineStarts []int, fixes []engine.Fix) []TextEdit {
	// ApplyFixes sorts and removes conflicts, returning the new source.
	// We need the non-conflicting subset, not the new source.
	// Re-use ApplyFixes for conflict detection: after calling it, reconstruct
	// which fixes were applied by diffing against the conflict set.
	_, conflicts := engine.ApplyFixes(source, fixes)

	conflictSet := make(map[engine.Fix]bool, len(conflicts))
	for _, c := range conflicts {
		conflictSet[c.Fix] = true
	}

	edits := make([]TextEdit, 0, len(fixes)-len(conflicts))
	for _, f := range fixes {
		if conflictSet[f] {
			continue
		}
		if f.StartByte < 0 || f.EndByte < f.StartByte || f.EndByte > len(source) {
			continue
		}
		edits = append(edits, fixToTextEdit(&f, source, lineStarts))
	}

	if len(conflicts) > 0 {
		slog.Debug("code action: skipped conflicting fixes", "conflicts", len(conflicts))
	}

	return edits
}

// fixToTextEdit converts an engine Fix to an LSP TextEdit.
func fixToTextEdit(fix *engine.Fix, source []byte, lineStarts []int) TextEdit {
	return TextEdit{
		Range: Range{
			Start: byteOffsetToPosition(source, lineStarts, fix.StartByte),
			End:   byteOffsetToPosition(source, lineStarts, fix.EndByte),
		},
		NewText: fix.NewText,
	}
}

// kindAllowed returns true if the given kind is allowed by the Only filter.
// An empty filter allows all kinds. Kinds match hierarchically: "source"
// allows "source.fixAll".
func kindAllowed(kind CodeActionKind, only []CodeActionKind) bool {
	if len(only) == 0 {
		return true
	}
	for _, f := range only {
		if kind == f || strings.HasPrefix(string(kind), string(f)+".") {
			return true
		}
	}
	return false
}

// rangesOverlap returns true if two LSP ranges overlap.
// LSP ranges are end-exclusive, so ranges that only touch do not overlap.
func rangesOverlap(a, b Range) bool {
	return posLT(a.Start, b.End) && posLT(b.Start, a.End)
}

// posLT returns true if a is strictly less-than b.
func posLT(a, b Position) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Character < b.Character
}
