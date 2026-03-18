package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoUselessCatch(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	param := node.ChildByFieldName("parameter")
	body := node.ChildByFieldName("body")
	if param.IsNull() || body.IsNull() {
		return
	}
	// Find the single non-comment statement in the body. If there are
	// zero or more than one real statements, the catch is not useless.
	var stmt parser.Node
	for i := uint(0); i < body.NamedChildCount(); i++ {
		child := body.NamedChild(i)
		if child.Kind() == "comment" {
			continue
		}
		if !stmt.IsNull() {
			return // more than one statement
		}
		stmt = child
	}
	if stmt.IsNull() || stmt.Kind() != "throw_statement" {
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
