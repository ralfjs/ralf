package engine

import (
	"fmt"

	"github.com/Hideart/ralf/internal/parser"
)

func checkNoDupeArgs(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	seen := make(map[string]bool)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		name := extractParamName(child, source)
		if name == "" {
			continue
		}
		if seen[name] {
			d := builtinDiag(child, lineStarts)
			d.Message = fmt.Sprintf("Duplicate parameter '%s'.", name)
			*diags = append(*diags, d)
		}
		seen[name] = true
	}
}

// extractParamName returns the identifier name from a parameter node.
// Handles plain identifiers, default values (assignment_pattern), and
// rest parameters (rest_pattern). Returns "" for destructured patterns.
func extractParamName(param parser.Node, source []byte) string {
	switch param.Kind() {
	case "identifier":
		return param.Text(source)
	case "assignment_pattern":
		// Default parameter: left side is the identifier.
		left := param.ChildByFieldName("left")
		if !left.IsNull() && left.Kind() == "identifier" {
			return left.Text(source)
		}
	case "rest_pattern":
		// Rest parameter: ...name
		if param.NamedChildCount() > 0 {
			child := param.NamedChild(0)
			if child.Kind() == "identifier" {
				return child.Text(source)
			}
		}
	}
	return ""
}
