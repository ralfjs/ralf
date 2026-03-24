package engine

import (
	"bytes"
	"sort"
)

// buildLineIndex scans source for newline positions and returns a slice of
// byte offsets where each line starts. The first entry is always 0 (line 1
// starts at byte 0).
//
// Uses bytes.IndexByte which leverages platform-optimized assembly (SIMD on
// supported architectures) to scan multiple bytes per cycle.
func buildLineIndex(source []byte) []int {
	// Estimate ~60 bytes per line to pre-allocate.
	starts := make([]int, 1, len(source)/60+1)
	starts[0] = 0
	offset := 0
	for {
		i := bytes.IndexByte(source[offset:], '\n')
		if i < 0 {
			break
		}
		starts = append(starts, offset+i+1)
		offset += i + 1
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
