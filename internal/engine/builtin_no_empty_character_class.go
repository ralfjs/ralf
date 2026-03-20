package engine

import (
	"github.com/Hideart/ralf/internal/parser"
)

// checkNoEmptyCharacterClass flags regex literals containing empty character
// classes []. Handles escapes correctly: [\]] is not empty, [^] is not empty.
func checkNoEmptyCharacterClass(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	text := node.Text(source)
	if len(text) < 2 {
		return
	}
	// Scan the regex pattern (between first / and last /) for empty [].
	// Find the pattern boundaries, skip leading / and trailing /flags.
	start := 1
	end := len(text) - 1
	for end > start && text[end] != '/' {
		end--
	}
	if end <= start {
		return
	}

	for i := start; i < end; i++ {
		if text[i] == '\\' {
			i++ // skip escaped character
			continue
		}
		if text[i] == '[' {
			// Check for [^] which matches any character — not empty.
			j := i + 1
			if j < end && text[j] == '^' {
				j++
			}
			// If the very next character is ], this is an empty class.
			if j < end && text[j] == ']' {
				// [^] is not empty (matches any char in JS).
				if j == i+2 && text[i+1] == '^' {
					i = j
					continue
				}
				d := builtinDiag(node, lineStarts)
				*diags = append(*diags, d)
				return // one diagnostic per regex
			}
			// Skip to closing ] to avoid false positives on nested [.
			for j < end {
				if text[j] == '\\' {
					j++
				} else if text[j] == ']' {
					break
				}
				j++
			}
			i = j
		}
	}
}
