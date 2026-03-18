package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoEmptyPattern(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	if node.NamedChildCount() > 0 {
		return
	}
	d := builtinDiag(node, lineStarts)
	kind := node.Kind()
	if kind == "array_pattern" {
		d.Message = "Unexpected empty array pattern."
	} else {
		d.Message = "Unexpected empty object pattern."
	}
	*diags = append(*diags, d)
}
