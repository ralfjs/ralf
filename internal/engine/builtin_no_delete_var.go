package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoDeleteVar(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() || op.Text(source) != "delete" {
		return
	}
	arg := node.ChildByFieldName("argument")
	if arg.IsNull() || arg.Kind() != "identifier" {
		return
	}
	*diags = append(*diags, builtinDiag(node, lineStarts))
}
