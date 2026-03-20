package engine

import (
	"fmt"

	"github.com/Hideart/ralf/internal/parser"
)

// memberKindTag distinguishes get/set/method/field for duplicate detection.
type memberKindTag byte

const (
	memberMethod memberKindTag = iota
	memberGetter
	memberSetter
	memberField
)

// memberKey uniquely identifies a class member for duplicate detection.
// Getters and setters with the same name are not duplicates.
// Static and instance members with the same name are not duplicates.
type memberKey struct {
	name     string
	isStatic bool
	kind     memberKindTag
}

func checkNoDupeClassMembers(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	seen := make(map[memberKey]bool)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		ck := child.Kind()
		if ck != "method_definition" && ck != "field_definition" {
			continue
		}

		key := child.ChildByFieldName("name")
		if key.IsNull() || key.Kind() == "computed_property_name" {
			continue
		}

		name := normalizeKeyName(key, source)

		// Determine static-ness: first raw child is "static" keyword.
		isStatic := false
		if fc := child.Child(0); !fc.IsNull() && fc.Text(source) == "static" {
			isStatic = true
		}

		// Determine member kind: get/set/method/field.
		mk := memberMethod
		if ck == "field_definition" {
			mk = memberField
		} else {
			// For method_definition, check for get/set keywords before the name.
			for j := uint(0); j < child.ChildCount(); j++ {
				c := child.Child(j)
				cs, ce := c.StartByte(), c.EndByte()
				l := ce - cs
				if l == 3 && source[cs] == 'g' && source[cs+1] == 'e' && source[cs+2] == 't' {
					mk = memberGetter
					break
				}
				if l == 3 && source[cs] == 's' && source[cs+1] == 'e' && source[cs+2] == 't' {
					mk = memberSetter
					break
				}
				// Stop at the name/params to avoid false matches.
				ck := c.Kind()
				if ck == "property_identifier" || ck == "formal_parameters" {
					break
				}
			}
		}

		mkey := memberKey{name: name, isStatic: isStatic, kind: mk}
		if seen[mkey] {
			d := builtinDiag(key, lineStarts)
			d.Message = fmt.Sprintf("Duplicate class member '%s'.", name)
			*diags = append(*diags, d)
		}
		seen[mkey] = true
	}
}
