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

		// Determine static-ness and member kind by scanning anonymous
		// keyword tokens (not named identifiers) before the name field.
		isStatic := false
		mk := memberMethod
		if ck == "field_definition" {
			mk = memberField
		}
		for j := uint(0); j < child.ChildCount(); j++ {
			c := child.Child(j)
			// Stop at the name/params — everything after is not a modifier.
			if c.IsNamed() {
				ck := c.Kind()
				if ck == "property_identifier" || ck == "formal_parameters" || ck == "string" || ck == "number" || ck == "computed_property_name" {
					break
				}
				continue
			}
			// Anonymous token — check for keyword modifiers.
			cs, ce := c.StartByte(), c.EndByte()
			l := ce - cs
			switch {
			case l == 6 && string(source[cs:ce]) == "static":
				isStatic = true
			case l == 3 && source[cs] == 'g' && source[cs+1] == 'e' && source[cs+2] == 't':
				mk = memberGetter
			case l == 3 && source[cs] == 's' && source[cs+1] == 'e' && source[cs+2] == 't':
				mk = memberSetter
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
