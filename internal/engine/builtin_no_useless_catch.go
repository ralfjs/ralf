package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoUselessCatch(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	param := node.ChildByFieldName("parameter")
	body := node.ChildByFieldName("body")
	if param.IsNull() || body.IsNull() {
		return
	}
	// Body is a statement_block; check it has exactly one named child.
	if body.NamedChildCount() != 1 {
		return
	}
	stmt := body.NamedChild(0)
	if stmt.Kind() != "throw_statement" {
		return
	}
	// The throw's argument should be the same identifier as the catch param.
	throwArg := stmt.NamedChild(0)
	if throwArg.IsNull() {
		return
	}
	if throwArg.Kind() == "identifier" && throwArg.Text(source) == param.Text(source) {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	}
}
