// Package engine implements the lint pipeline: compile rules, match against
// source files, and produce diagnostics.
package engine

import "github.com/Hideart/ralf/internal/config"

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
