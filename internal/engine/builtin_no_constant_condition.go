package engine

import "github.com/ralfjs/ralf/internal/parser"

// checkNoConstantCondition flags constant (always truthy/falsy) conditions
// in if/while/do/for/ternary statements.
func checkNoConstantCondition(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	cond := node.ChildByFieldName("condition")
	if cond.IsNull() {
		return
	}
	if isConstantExpr(cond, source) {
		*diags = append(*diags, builtinDiag(cond, lineStarts))
	}
}

// isConstantExpr reports whether a node is a compile-time constant expression.
func isConstantExpr(node parser.Node, source []byte) bool {
	switch node.Kind() {
	case "true", "false", "null", "number", "string":
		return true
	case "template_string":
		// Constant only if no interpolations (template_substitution children).
		return node.NamedChildCount() == 0
	case "identifier":
		text := node.Text(source)
		return text == "undefined" || text == "NaN" || text == "Infinity"
	case "array", "object":
		return true
	case "unary_expression":
		op := node.ChildByFieldName("operator")
		arg := node.ChildByFieldName("argument")
		if op.IsNull() || arg.IsNull() {
			return false
		}
		switch op.Text(source) {
		case "!", "-", "+", "~", "typeof", "void":
			return isConstantExpr(arg, source)
		}
	case "binary_expression":
		left := node.ChildByFieldName("left")
		right := node.ChildByFieldName("right")
		if left.IsNull() || right.IsNull() {
			return false
		}
		return isConstantExpr(left, source) && isConstantExpr(right, source)
	case "parenthesized_expression":
		if node.NamedChildCount() > 0 {
			return isConstantExpr(node.NamedChild(0), source)
		}
	}
	return false
}
