package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoCondAssign(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	cond := node.ChildByFieldName("condition")
	if cond.IsNull() {
		return
	}
	if containsAssignment(cond) {
		*diags = append(*diags, builtinDiag(cond, lineStarts))
	}
}

func containsAssignment(node parser.Node) bool {
	kind := node.Kind()
	if kind == "assignment_expression" {
		return true
	}
	// Don't descend into nested function bodies — assignments
	// inside arrow/function literals are not part of the condition.
	if isFunctionNode(kind) {
		return false
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if containsAssignment(node.NamedChild(i)) {
			return true
		}
	}
	return false
}
