package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoDuplicateCase(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	body := node.ChildByFieldName("body")
	if body.IsNull() {
		return
	}
	seen := make(map[string]bool)
	for i := uint(0); i < body.NamedChildCount(); i++ {
		caseNode := body.NamedChild(i)
		if caseNode.Kind() != "switch_case" {
			continue
		}
		val := caseNode.ChildByFieldName("value")
		if val.IsNull() {
			continue
		}
		text := val.Text(source)
		if seen[text] {
			*diags = append(*diags, builtinDiag(val, lineStarts))
		}
		seen[text] = true
	}
}
