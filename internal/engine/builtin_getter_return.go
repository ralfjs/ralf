package engine

import "github.com/Hideart/ralf/internal/parser"

// checkGetterReturn ensures getter methods contain at least one return statement
// with a value. This is a simplified check (exists-any-return, not all-paths-return)
// which is acceptable until CFG infrastructure lands.
func checkGetterReturn(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	if node.ChildCount() < 1 {
		return
	}
	firstChild := node.Child(0)
	if firstChild.IsNull() || firstChild.Text(source) != "get" {
		return
	}

	body := node.ChildByFieldName("body")
	if body.IsNull() {
		return
	}

	if !bodyHasReturnWithValue(body) {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	}
}

// bodyHasReturnWithValue recursively checks if a node tree contains a
// return statement with a value, skipping nested function boundaries.
func bodyHasReturnWithValue(node parser.Node) bool {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		kind := child.Kind()
		if isFunctionNode(kind) {
			continue
		}
		if kind == "return_statement" && child.NamedChildCount() > 0 {
			return true
		}
		if bodyHasReturnWithValue(child) {
			return true
		}
	}
	return false
}
