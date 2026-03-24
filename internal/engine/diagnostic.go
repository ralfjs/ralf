// Package engine implements the lint pipeline: compile rules, match against
// source files, and produce diagnostics.
package engine

import (
	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
)

// Diagnostic represents a single lint finding.
type Diagnostic struct {
	File     string          // absolute file path
	Line     int             // 1-based
	Col      int             // 0-based byte offset within line
	EndLine  int             // 1-based
	EndCol   int             // 0-based byte offset within line
	Rule     string          // rule name from config
	Message  string          // human-readable description
	Severity config.Severity // error, warn, off
	Fix      *Fix            // nil if rule has no fix
}

// FileError records a file that could not be read or processed.
type FileError struct {
	File string
	Err  error
}

// Result aggregates diagnostics and file-level errors from a lint run.
type Result struct {
	Diagnostics []Diagnostic
	Errors      []FileError
}

// nodeDiag builds a Diagnostic from an AST node's position. Used by both
// pattern and structural matchers to avoid duplicating offset → line/col
// resolution and Diagnostic construction.
func nodeDiag(node parser.Node, lineStarts []int, rule, message string, severity config.Severity) Diagnostic {
	startLine, startCol := offsetToLineCol(lineStarts, int(node.StartByte())) //nolint:gosec // tree-sitter offsets fit in int
	endLine, endCol := offsetToLineCol(lineStarts, int(node.EndByte()))       //nolint:gosec // tree-sitter offsets fit in int
	return Diagnostic{
		Line:     startLine,
		Col:      startCol,
		EndLine:  endLine,
		EndCol:   endCol,
		Rule:     rule,
		Message:  message,
		Severity: severity,
	}
}

// nodeFix builds a Fix from an AST node's byte range. Handles the
// fixDeleteStatement sentinel by expanding to the full statement line.
func nodeFix(node parser.Node, source []byte, fixText string) *Fix {
	sb := int(node.StartByte()) //nolint:gosec // tree-sitter offsets fit in int
	eb := int(node.EndByte())   //nolint:gosec // tree-sitter offsets fit in int
	newText := fixText
	if newText == fixDeleteStatement {
		sb, eb = expandToStatement(source, sb, eb)
		newText = ""
	}
	return &Fix{StartByte: sb, EndByte: eb, NewText: newText}
}

// builtinDiag builds a position-only Diagnostic from a node for builtin checkers.
// The engine loop fills in File, Rule, Severity, and Message (unless Message is
// already set by the caller to override the default).
func builtinDiag(node parser.Node, lineStarts []int) Diagnostic {
	startLine, startCol := offsetToLineCol(lineStarts, int(node.StartByte())) //nolint:gosec // tree-sitter offsets fit in int
	endLine, endCol := offsetToLineCol(lineStarts, int(node.EndByte()))       //nolint:gosec // tree-sitter offsets fit in int
	return Diagnostic{
		Line:    startLine,
		Col:     startCol,
		EndLine: endLine,
		EndCol:  endCol,
	}
}

// setFilePath sets the File field on a slice of diagnostics.
func setFilePath(diags []Diagnostic, filePath string) {
	for i := range diags {
		diags[i].File = filePath
	}
}
