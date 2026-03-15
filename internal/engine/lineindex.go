package engine

import "sort"

// buildLineIndex scans source for newline positions and returns a slice of
// byte offsets where each line starts. The first entry is always 0 (line 1
// starts at byte 0).
func buildLineIndex(source []byte) []int {
	// Estimate ~60 bytes per line to pre-allocate.
	starts := make([]int, 1, len(source)/60+1)
	starts[0] = 0
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offsetToLineCol converts a byte offset into 1-based line and 0-based column.
// If offset is beyond the source length, it returns the last line.
func offsetToLineCol(lineStarts []int, offset int) (line, col int) {
	// Binary search: find the last lineStart <= offset.
	idx := sort.SearchInts(lineStarts, offset+1) - 1
	if idx < 0 {
		idx = 0
	}
	return idx + 1, offset - lineStarts[idx]
}
