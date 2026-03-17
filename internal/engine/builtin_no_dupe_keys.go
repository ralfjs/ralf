package engine

import (
	"fmt"

	"github.com/Hideart/ralf/internal/parser"
)

func checkNoDupeKeys(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	seen := make(map[string]bool)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		prop := node.NamedChild(i)
		if prop.Kind() != "pair" && prop.Kind() != "method_definition" {
			continue
		}
		key := prop.ChildByFieldName("key")
		if key.IsNull() || key.Kind() == "computed_property_name" {
			continue
		}
		name := key.Text(source)
		if seen[name] {
			d := builtinDiag(key, lineStarts)
			d.Message = fmt.Sprintf("Duplicate key '%s'.", name)
			*diags = append(*diags, d)
		}
		seen[name] = true
	}
}
