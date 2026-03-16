package engine

import "slices"

// Fix represents a concrete source code edit to resolve a diagnostic.
type Fix struct {
	StartByte int    // inclusive byte offset in source
	EndByte   int    // exclusive byte offset in source
	NewText   string // replacement text (empty = deletion)
}

// Conflict records a fix that was skipped due to overlap with a prior fix.
type Conflict struct {
	Fix    Fix
	Reason string
}

// fixDeleteStatement is the sentinel value that signals "delete the entire
// statement containing the match" rather than a literal replacement.
const fixDeleteStatement = "delete-statement"

// ApplyFixes applies non-conflicting fixes to source, returning the new source.
// Fixes are sorted by StartByte, overlaps are detected and skipped (returned as
// conflicts), and the result is built in a single forward pass with no
// intermediate allocations beyond the output buffer.
func ApplyFixes(source []byte, fixes []Fix) ([]byte, []Conflict) {
	if len(fixes) == 0 {
		return source, nil
	}

	// Sort by StartByte ascending to detect overlaps.
	// Fast path: skip the copy+sort when fixes are already ordered (common
	// case — LintFile returns diagnostics sorted by line/col).
	cmpFix := func(a, b Fix) int {
		if a.StartByte != b.StartByte {
			return a.StartByte - b.StartByte
		}
		return a.EndByte - b.EndByte
	}
	sorted := fixes
	if !slices.IsSortedFunc(fixes, cmpFix) {
		sorted = make([]Fix, len(fixes))
		copy(sorted, fixes)
		slices.SortFunc(sorted, cmpFix)
	}

	// Single pass: detect overlaps, compute output size, and build result.
	// First pass: count accepted fixes and compute result length so we can
	// allocate the output buffer exactly once.
	resultLen := len(source)
	prevEnd := -1
	nConflicts := 0
	for i := range sorted {
		f := &sorted[i]
		if f.StartByte < prevEnd {
			nConflicts++
			continue
		}
		resultLen += len(f.NewText) - (f.EndByte - f.StartByte)
		prevEnd = f.EndByte
	}

	// Second pass: build result and collect conflicts.
	result := make([]byte, 0, resultLen)
	var conflicts []Conflict
	if nConflicts > 0 {
		conflicts = make([]Conflict, 0, nConflicts)
	}
	pos := 0
	prevEnd = -1
	for i := range sorted {
		f := &sorted[i]
		if f.StartByte < prevEnd {
			conflicts = append(conflicts, Conflict{
				Fix:    *f,
				Reason: "overlaps with earlier fix",
			})
			continue
		}
		result = append(result, source[pos:f.StartByte]...)
		result = append(result, f.NewText...)
		pos = f.EndByte
		prevEnd = f.EndByte
	}
	result = append(result, source[pos:]...)

	return result, conflicts
}

// expandToStatement expands a byte range to cover the full line(s), including
// the trailing newline. Used for "delete-statement" fix type.
func expandToStatement(source []byte, start, end int) (stmtStart, stmtEnd int) {
	// Find start of line.
	s := start
	for s > 0 && source[s-1] != '\n' {
		s--
	}

	// Find end of line (include trailing newline).
	e := end
	for e < len(source) && source[e] != '\n' {
		e++
	}
	if e < len(source) {
		e++ // include the newline
	}

	return s, e
}
