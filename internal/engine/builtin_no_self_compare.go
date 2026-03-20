package engine

import (
	"fmt"

	"github.com/Hideart/ralf/internal/parser"
)

func checkNoSelfCompare(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() {
		return
	}
	opText := op.Text(source)
	switch opText {
	case "==", "===", "!=", "!==", "<", ">", "<=", ">=":
	default:
		return
	}

	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left.IsNull() || right.IsNull() {
		return
	}
	leftText := left.Text(source)
	if leftText == right.Text(source) {
		d := builtinDiag(node, lineStarts)
		d.Message = fmt.Sprintf("Comparing '%s' to itself is always the same.", leftText)
		*diags = append(*diags, d)
	}
}
