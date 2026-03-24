package engine

import "github.com/ralfjs/ralf/internal/parser"

func checkNoDeleteVar(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() || op.Text(source) != "delete" {
		return
	}
	arg := node.ChildByFieldName("argument")
	if arg.IsNull() {
		return
	}
	// Unwrap parenthesized expressions: delete (x) → delete x
	for arg.Kind() == "parenthesized_expression" && arg.NamedChildCount() > 0 {
		arg = arg.NamedChild(0)
	}
	if arg.Kind() != "identifier" {
		return
	}
	*diags = append(*diags, builtinDiag(node, lineStarts))
}
