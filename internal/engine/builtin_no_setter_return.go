package engine

import "github.com/ralfjs/ralf/internal/parser"

func checkNoSetterReturn(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	// Check for setter methods: method_definition with "set" keyword
	if node.ChildCount() < 1 {
		return
	}
	firstChild := node.Child(0)
	if firstChild.IsNull() || firstChild.Text(source) != "set" {
		return
	}
	// Find return statements with values inside this setter's body
	body := node.ChildByFieldName("body")
	if body.IsNull() {
		return
	}
	findReturnWithValue(body, lineStarts, diags)
}

func findReturnWithValue(node parser.Node, lineStarts []int, diags *[]Diagnostic) {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		kind := child.Kind()
		if isFunctionNode(kind) {
			continue
		}
		if kind == "return_statement" && child.NamedChildCount() > 0 {
			*diags = append(*diags, builtinDiag(child, lineStarts))
			continue
		}
		findReturnWithValue(child, lineStarts, diags)
	}
}
