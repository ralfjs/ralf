package engine

import (
	"fmt"

	"github.com/Hideart/ralf/internal/parser"
)

func checkNoDupeArgs(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	seen := make(map[string]bool)
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		collectParamNames(child, source, lineStarts, seen, diags)
	}
}

// collectParamNames extracts all bound identifier names from a parameter node,
// recursing into destructuring patterns, and flags duplicates.
func collectParamNames(param parser.Node, source []byte, lineStarts []int, seen map[string]bool, diags *[]Diagnostic) {
	switch param.Kind() {
	case "identifier":
		name := param.Text(source)
		if seen[name] {
			d := builtinDiag(param, lineStarts)
			d.Message = fmt.Sprintf("Duplicate parameter '%s'.", name)
			*diags = append(*diags, d)
		}
		seen[name] = true
	case "assignment_pattern":
		// Default parameter: left side holds the binding.
		left := param.ChildByFieldName("left")
		if !left.IsNull() {
			collectParamNames(left, source, lineStarts, seen, diags)
		}
	case "rest_pattern":
		// Rest parameter: ...name or ...{destructured}
		if param.NamedChildCount() > 0 {
			collectParamNames(param.NamedChild(0), source, lineStarts, seen, diags)
		}
	case "object_pattern":
		// Destructured object: { a, b: c, ...rest }
		for i := uint(0); i < param.NamedChildCount(); i++ {
			child := param.NamedChild(i)
			switch child.Kind() {
			case "shorthand_property_identifier_pattern":
				// { a } — the identifier IS the binding.
				collectParamNames(child, source, lineStarts, seen, diags)
			case "pair_pattern":
				// { b: c } — the value side is the binding.
				val := child.ChildByFieldName("value")
				if !val.IsNull() {
					collectParamNames(val, source, lineStarts, seen, diags)
				}
			case "rest_pattern":
				collectParamNames(child, source, lineStarts, seen, diags)
			}
		}
	case "array_pattern":
		// Destructured array: [a, b, ...rest]
		for i := uint(0); i < param.NamedChildCount(); i++ {
			collectParamNames(param.NamedChild(i), source, lineStarts, seen, diags)
		}
	case "shorthand_property_identifier_pattern":
		// Treat like identifier for duplicate detection.
		name := param.Text(source)
		if seen[name] {
			d := builtinDiag(param, lineStarts)
			d.Message = fmt.Sprintf("Duplicate parameter '%s'.", name)
			*diags = append(*diags, d)
		}
		seen[name] = true
	}
}
