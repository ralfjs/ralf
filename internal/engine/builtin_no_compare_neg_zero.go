package engine

import (
	"fmt"

	"github.com/Hideart/ralf/internal/parser"
)

func checkNoCompareNegZero(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() {
		return
	}
	opText := op.Text(source)
	switch opText {
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=":
	default:
		return
	}
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if isNegZero(left, source) || isNegZero(right, source) {
		d := builtinDiag(node, lineStarts)
		d.Message = fmt.Sprintf("Do not use the '%s' operator to compare against -0.", opText)
		*diags = append(*diags, d)
	}
}

func isNegZero(node parser.Node, source []byte) bool {
	if node.IsNull() || node.Kind() != "unary_expression" {
		return false
	}
	op := node.ChildByFieldName("operator")
	arg := node.ChildByFieldName("argument")
	if op.IsNull() || arg.IsNull() {
		return false
	}
	return op.Text(source) == "-" && arg.Kind() == "number" && arg.Text(source) == "0"
}
