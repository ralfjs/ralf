package lsp

import (
	"unicode/utf8"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

// convertDiagnostics maps engine diagnostics to LSP diagnostics.
// Returns an empty non-nil slice when there are no diagnostics.
func convertDiagnostics(engineDiags []engine.Diagnostic, source []byte) []LDiagnostic {
	if len(engineDiags) == 0 {
		return []LDiagnostic{}
	}

	lineStarts := buildLineIndex(source)
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
	utf16Offset := 0
	i := start
	for i < end {
		r, size := utf8.DecodeRune(source[i:])
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
