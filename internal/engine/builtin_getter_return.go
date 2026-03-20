package engine

import "github.com/Hideart/ralf/internal/parser"

// checkGetterReturn ensures getter methods contain at least one return statement
// with a value. This is a simplified check (exists-any-return, not all-paths-return)
// which is acceptable until CFG infrastructure lands.
// Scans raw children for "get" keyword to handle both `get foo()` and `static get foo()`.
func checkGetterReturn(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	body := node.ChildByFieldName("body")
	if body.IsNull() {
		return
	}

	isGetter := false
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.StartByte() >= body.StartByte() {
			break
		}
		if child.IsNull() {
			continue
		}
		s, e := child.StartByte(), child.EndByte()
		if e-s == 3 && source[s] == 'g' && source[s+1] == 'e' && source[s+2] == 't' {
			isGetter = true
			break
		}
	}
	if !isGetter {
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
