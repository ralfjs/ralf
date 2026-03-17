package engine

import "github.com/Hideart/ralf/internal/parser"

func checkRequireYield(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	body := node.ChildByFieldName("body")
	if body.IsNull() {
		return
	}
	if containsYield(body) {
		return
	}
	*diags = append(*diags, builtinDiag(node, lineStarts))
}

func containsYield(node parser.Node) bool {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		kind := child.Kind()
		if kind == "yield_expression" {
			return true
		}
		// Don't descend into nested function scopes
		if isFunctionNode(kind) {
			continue
		}
		if containsYield(child) {
			return true
		}
	}
	return false
}
