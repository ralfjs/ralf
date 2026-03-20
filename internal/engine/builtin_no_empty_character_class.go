package engine

import (
	"github.com/Hideart/ralf/internal/parser"
)

// checkNoEmptyCharacterClass flags regex literals containing truly empty
// character classes []. Correctly handles:
//   - [^] → matches any char in JS, NOT empty
//   - []] → ] is first literal char, NOT empty
//   - [\]] → escaped ], NOT empty
//   - [] → truly empty, flagged
func checkNoEmptyCharacterClass(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	text := node.Text(source)
	if len(text) < 2 {
		return
	}
	// Find pattern boundaries: between first / and last /.
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
		if text[i] != '[' {
			continue
		}

		// Found opening [. Check for optional ^ negation.
		j := i + 1
		if j < end && text[j] == '^' {
			j++
		}

		// In JS regex: [] is truly empty (always fails to match).
		// [^] matches any character (not empty).
		// []] means class containing literal ] (not empty).
		// So only flag when there's no ^ AND the next char is ].
		if j < end && text[j] == ']' && j == i+1 {
			// [] — truly empty.
			d := builtinDiag(node, lineStarts)
			*diags = append(*diags, d)
			return
		}

		// Skip to the actual closing ] of this class.
		// If first char after [/[^ is ], it's a literal — skip past it.
		k := j
		if k < end && text[k] == ']' {
			k++
		}
		for k < end {
			if text[k] == '\\' {
				k++
			} else if text[k] == ']' {
				break
			}
			k++
		}
		i = k
	}
}
