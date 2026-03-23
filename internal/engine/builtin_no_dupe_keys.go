package engine

import (
	"fmt"

	"github.com/ralfjs/ralf/internal/parser"
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
		name := normalizeKeyName(key, source)
		if seen[name] {
			d := builtinDiag(key, lineStarts)
			d.Message = fmt.Sprintf("Duplicate key '%s'.", name)
			*diags = append(*diags, d)
		}
		seen[name] = true
	}
}

// normalizeKeyName returns a comparable key name. String literal keys
// have their quotes stripped so { a: 1, "a": 2 } is detected as a duplicate.
func normalizeKeyName(key parser.Node, source []byte) string {
	text := key.Text(source)
	if key.Kind() == "string" && len(text) >= 2 {
		return text[1 : len(text)-1]
	}
	return text
}
