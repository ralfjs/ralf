package engine

import (
	"fmt"

	"github.com/ralfjs/ralf/internal/parser"
)

func checkNoSelfAssign(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left.IsNull() || right.IsNull() {
		return
	}
	// assignment_expression in tree-sitter JS is always "=".
	// augmented_assignment_expression is a different node kind.
	leftText := left.Text(source)
	if leftText == right.Text(source) && left.Kind() == "identifier" {
		d := builtinDiag(node, lineStarts)
		d.Message = fmt.Sprintf("'%s' is assigned to itself.", leftText)
		*diags = append(*diags, d)
	}
}
