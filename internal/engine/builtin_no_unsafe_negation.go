package engine

import (
	"fmt"

	"github.com/Hideart/ralf/internal/parser"
)

func checkNoUnsafeNegation(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() {
		return
	}
	opText := op.Text(source)
	if opText != "in" && opText != "instanceof" {
		return
	}
	left := node.ChildByFieldName("left")
	if left.IsNull() || left.Kind() != "unary_expression" {
		return
	}
	leftOp := left.ChildByFieldName("operator")
	if leftOp.IsNull() || leftOp.Text(source) != "!" {
		return
	}
	// If the left operand were wrapped in parenthesized_expression, tree-sitter
	// would report that wrapper as the "left" field instead of unary_expression
	// directly, so reaching here means the negation is unparenthesised (unsafe).
	d := builtinDiag(node, lineStarts)
	d.Message = fmt.Sprintf("Unexpected negating the left operand of '%s' operator.", opText)
	*diags = append(*diags, d)
}
