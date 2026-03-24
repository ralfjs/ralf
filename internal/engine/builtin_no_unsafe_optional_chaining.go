package engine

import "github.com/ralfjs/ralf/internal/parser"

// checkNoUnsafeOptionalChaining flags optional chaining (?.) in contexts
// where undefined would cause a runtime error: arithmetic, spread, new,
// left side of assignment, tagged templates.
func checkNoUnsafeOptionalChaining(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	if !hasOptionalChain(node, source) {
		return
	}

	// Walk up through transparent wrappers (parenthesized_expression)
	// to find the effective parent context.
	p := node.Parent()
	for !p.IsNull() && p.Kind() == "parenthesized_expression" {
		p = p.Parent()
	}
	if p.IsNull() {
		return
	}
	pk := p.Kind()

	switch pk {
	case "binary_expression":
		op := p.ChildByFieldName("operator")
		if !op.IsNull() && isArithmeticOp(op.Text(source)) {
			*diags = append(*diags, builtinDiag(node, lineStarts))
		}
	case "unary_expression":
		op := p.ChildByFieldName("operator")
		if !op.IsNull() {
			switch op.Text(source) {
			case "+", "-", "~":
				*diags = append(*diags, builtinDiag(node, lineStarts))
			}
		}
	case "new_expression":
		*diags = append(*diags, builtinDiag(node, lineStarts))
	case "spread_element":
		*diags = append(*diags, builtinDiag(node, lineStarts))
	case "assignment_expression", "augmented_assignment_expression":
		left := p.ChildByFieldName("left")
		if !left.IsNull() && left.StartByte() == node.StartByte() {
			*diags = append(*diags, builtinDiag(node, lineStarts))
		}
	case "for_in_statement":
		left := p.ChildByFieldName("left")
		if !left.IsNull() && left.StartByte() == node.StartByte() {
			*diags = append(*diags, builtinDiag(node, lineStarts))
		}
	case "tagged_template_expression":
		if p.ChildCount() > 0 && p.Child(0).StartByte() == node.StartByte() {
			*diags = append(*diags, builtinDiag(node, lineStarts))
		}
	}
}

// hasOptionalChain checks whether a member_expression or call_expression
// uses optional chaining (contains "?." token). Compares source bytes
// directly to avoid string allocation on the hot path.
func hasOptionalChain(node parser.Node, source []byte) bool {
	for i := uint(0); i < node.ChildCount(); i++ {
		c := node.Child(i)
		s, e := c.StartByte(), c.EndByte()
		if e-s == 2 && source[s] == '?' && source[s+1] == '.' {
			return true
		}
	}
	return false
}

func isArithmeticOp(op string) bool {
	switch op {
	case "+", "-", "*", "/", "%", "**", "|", "&", "^", "<<", ">>", ">>>":
		return true
	}
	return false
}
