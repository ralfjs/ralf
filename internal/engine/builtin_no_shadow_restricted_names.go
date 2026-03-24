package engine

import (
	"fmt"

	"github.com/ralfjs/ralf/internal/parser"
)

func checkNoShadowRestrictedNames(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	nameNode := node.ChildByFieldName("name")
	if nameNode.IsNull() {
		return
	}
	name := nameNode.Text(source)
	if !restrictedNames[name] {
		return
	}
	d := builtinDiag(nameNode, lineStarts)
	d.Message = fmt.Sprintf("Shadowing of global property '%s'.", name)
	*diags = append(*diags, d)
}

var restrictedNames = map[string]bool{
	"undefined": true,
	"NaN":       true,
	"Infinity":  true,
	"arguments": true,
	"eval":      true,
}
