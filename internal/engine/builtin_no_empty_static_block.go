package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoEmptyStaticBlock(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	// Check if block body (statement_block child) has any named children.
	body := node.ChildByFieldName("body")
	if body.IsNull() {
		// No body field — check node directly.
		if node.NamedChildCount() > 0 {
			return
		}
	} else if body.NamedChildCount() > 0 {
		return
	}
	*diags = append(*diags, builtinDiag(node, lineStarts))
}
