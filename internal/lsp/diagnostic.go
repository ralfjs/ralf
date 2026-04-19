package lsp

import (
	"sort"
	"unicode/utf8"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

// convertDiagnostics maps engine diagnostics to LSP diagnostics.
// Returns an empty non-nil slice when there are no diagnostics.
func convertDiagnostics(engineDiags []engine.Diagnostic, source []byte) []LDiagnostic {
	return convertDiagnosticsWithIndex(engineDiags, source, buildLineIndex(source))
}

// convertDiagnosticsWithIndex maps engine diagnostics to LSP diagnostics
// using a pre-built line index. Returns an empty non-nil slice when there
// are no diagnostics.
func convertDiagnosticsWithIndex(engineDiags []engine.Diagnostic, source []byte, lineStarts []int) []LDiagnostic {
	if len(engineDiags) == 0 {
		return []LDiagnostic{}
	}

	lspDiags := make([]LDiagnostic, 0, len(engineDiags))
	for i := range engineDiags {
		lspDiags = append(lspDiags, convertDiagnostic(&engineDiags[i], source, lineStarts))
	}
	return lspDiags
}

func convertDiagnostic(ed *engine.Diagnostic, source []byte, lineStarts []int) LDiagnostic {
	return LDiagnostic{
		Range: Range{
			Start: Position{
				Line:      ed.Line - 1,
				Character: byteColToUTF16(source, lineStarts, ed.Line, ed.Col),
			},
			End: Position{
				Line:      ed.EndLine - 1,
				Character: byteColToUTF16(source, lineStarts, ed.EndLine, ed.EndCol),
			},
		},
		Severity: mapSeverity(ed.Severity),
		Source:   "ralf",
		Message:  ed.Message,
		Code:     ed.Rule,
	}
}

// mapSeverity converts config.Severity to LSP DiagnosticSeverity.
func mapSeverity(s config.Severity) DiagnosticSeverity {
	switch s {
	case config.SeverityError:
		return SeverityError
	case config.SeverityWarn:
		return SeverityWarning
	default:
		return SeverityWarning
	}
}

// buildLineIndex returns the byte offset of the start of each line.
// Line 1 starts at index 0 in the returned slice.
func buildLineIndex(source []byte) []int {
	starts := make([]int, 1, len(source)/60+1)
	starts[0] = 0
	for i, b := range source {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// byteColToUTF16 converts a 0-based byte column offset to a 0-based UTF-16
// code unit offset. line1Based is the 1-indexed line number.
func byteColToUTF16(source []byte, lineStarts []int, line1Based, byteCol int) int {
	lineIdx := line1Based - 1
	if lineIdx < 0 || lineIdx >= len(lineStarts) {
		return byteCol
	}

	start := lineStarts[lineIdx]
	end := start + byteCol
	if end > len(source) {
		end = len(source)
	}

	// Fast path: if all bytes in range are ASCII, byte offset = UTF-16 offset.
	allASCII := true
	for i := start; i < end; i++ {
		if source[i] >= 0x80 {
			allASCII = false
			break
		}
	}
	if allASCII {
		return byteCol
	}

	// Slow path: decode runes and count UTF-16 code units.
	// Bound the slice to [start:end] so DecodeRune cannot read past the target
	// column (e.g. when byteCol lands mid-rune).
	segment := source[start:end]
	utf16Offset := 0
	i := 0
	for i < len(segment) {
		r, size := utf8.DecodeRune(segment[i:])
		if r == utf8.RuneError && size <= 1 {
			utf16Offset++
			i++
			continue
		}
		if r >= 0x10000 {
			utf16Offset += 2
		} else {
			utf16Offset++
		}
		i += size
	}
	return utf16Offset
}

// byteOffsetToPosition converts a byte offset in source to a 0-based LSP
// Position (line in 0-based, character in UTF-16 code units).
func byteOffsetToPosition(source []byte, lineStarts []int, offset int) Position {
	idx := sort.SearchInts(lineStarts, offset+1) - 1
	if idx < 0 {
		idx = 0
	}
	line1 := idx + 1
	byteCol := offset - lineStarts[idx]
	return Position{
		Line:      line1 - 1,
		Character: byteColToUTF16(source, lineStarts, line1, byteCol),
	}
}

// positionToByteOffset converts a 0-based LSP Position (line + UTF-16 code
// unit offset) to a byte offset in source. Inverse of byteOffsetToPosition.
//
// Clamping:
//   - Line before 0 → 0.
//   - Line past end of source → len(source).
//   - Character past end of the line → the line's terminating '\n' (or end of
//     source for the last line).
//   - Character landing inside a surrogate pair (odd offset within a rune
//     that needs 2 UTF-16 code units) → byte offset at that rune's start.
func positionToByteOffset(source []byte, lineStarts []int, pos Position) int {
	if pos.Line < 0 {
		return 0
	}
	if pos.Line >= len(lineStarts) {
		return len(source)
	}

	lineStart := lineStarts[pos.Line]
	lineEnd := len(source)
	if pos.Line+1 < len(lineStarts) {
		// lineStarts[i+1] points just past the '\n' that terminates line i.
		lineEnd = lineStarts[pos.Line+1] - 1
	}

	if pos.Character <= 0 {
		return lineStart
	}

	// Scan only the prefix up to the requested character (clamped to end of
	// line). If that prefix is all ASCII, UTF-16 offset == byte offset — bytes
	// after the prefix can't affect the mapping.
	scanEnd := lineStart + pos.Character
	if scanEnd > lineEnd {
		scanEnd = lineEnd
	}
	allASCII := true
	for i := lineStart; i < scanEnd; i++ {
		if source[i] >= 0x80 {
			allASCII = false
			break
		}
	}
	if allASCII {
		return scanEnd
	}

	offset := lineStart
	utf16 := 0
	for offset < lineEnd && utf16 < pos.Character {
		r, size := utf8.DecodeRune(source[offset:lineEnd])
		if r == utf8.RuneError && size <= 1 {
			utf16++
			offset++
			continue
		}
		units := 1
		if r >= 0x10000 {
			units = 2
		}
		if utf16+units > pos.Character {
			break
		}
		utf16 += units
		offset += size
	}
	return offset
}
