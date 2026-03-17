package engine

import "github.com/Hideart/ralf/internal/parser"

func checkUseIsNaN(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() {
		return
	}
	switch op.Text(source) {
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=":
	default:
		return
	}
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if isNaNNode(left, source) || isNaNNode(right, source) {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	}
}

func isNaNNode(node parser.Node, source []byte) bool {
	if node.IsNull() {
		return false
	}
	text := node.Text(source)
	return text == "NaN" || text == "Number.NaN"
}
