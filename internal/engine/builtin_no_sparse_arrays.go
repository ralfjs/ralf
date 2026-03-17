package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoSparseArrays(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	// In tree-sitter, sparse array elements appear as consecutive ","
	// tokens with no element node between them.
	childCount := node.ChildCount()
	for i := uint(0); i < childCount; i++ {
		child := node.Child(i)
		if child.Kind() != "," {
			continue
		}
		// If the previous child is also "," or "[", there is a hole.
		if i > 0 {
			prev := node.Child(i - 1)
			if prev.Kind() == "," || prev.Kind() == "[" {
				*diags = append(*diags, builtinDiag(child, lineStarts))
			}
		}
	}
}
