package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoEmpty(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	if hasNonCommentChild(node) {
		return
	}
	// Skip empty blocks that are catch clause bodies (ESLint allows those)
	parent := node.Parent()
	if !parent.IsNull() && parent.Kind() == "catch_clause" {
		return
	}
	*diags = append(*diags, builtinDiag(node, lineStarts))
}

// hasNonCommentChild returns true if node has any named child that is not a comment.
func hasNonCommentChild(node parser.Node) bool {
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if node.NamedChild(i).Kind() != "comment" {
			return true
		}
	}
	return false
}
