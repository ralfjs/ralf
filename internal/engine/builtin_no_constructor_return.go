package engine

import "github.com/ralfjs/ralf/internal/parser"

// checkNoConstructorReturn flags return statements with a value inside class constructors.
// Bare "return;" is allowed (early exit). Nested functions are not checked.
func checkNoConstructorReturn(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	// Only flag return statements that have a value.
	if node.NamedChildCount() == 0 {
		return
	}
	// Walk ancestors to find the enclosing method_definition.
	// Stop at function boundaries (nested functions are fine).
	for p := node.Parent(); !p.IsNull(); p = p.Parent() {
		pk := p.Kind()
		if pk == "method_definition" {
			name := p.ChildByFieldName("name")
			if !name.IsNull() && name.Text(source) == "constructor" {
				*diags = append(*diags, builtinDiag(node, lineStarts))
			}
			return
		}
		if isFunctionNode(pk) {
			return
		}
	}
}
