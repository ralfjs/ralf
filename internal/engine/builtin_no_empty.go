package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoEmpty(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	// ESLint's no-empty skips blocks that contain any content, including
	// comments. Only truly empty blocks (zero named children) are flagged.
	if node.NamedChildCount() > 0 {
		return
	}
	// Skip empty blocks that are catch clause bodies (ESLint allows those)
	parent := node.Parent()
	if !parent.IsNull() && parent.Kind() == "catch_clause" {
		return
	}
	*diags = append(*diags, builtinDiag(node, lineStarts))
}
