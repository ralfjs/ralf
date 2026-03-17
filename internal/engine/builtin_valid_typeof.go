package engine

import "github.com/Hideart/ralf/internal/parser"

var validTypeofValues = map[string]bool{
	`"undefined"`: true, `'undefined'`: true,
	`"object"`: true, `'object'`: true,
	`"boolean"`: true, `'boolean'`: true,
	`"number"`: true, `'number'`: true,
	`"string"`: true, `'string'`: true,
	`"function"`: true, `'function'`: true,
	`"symbol"`: true, `'symbol'`: true,
	`"bigint"`: true, `'bigint'`: true,
}

func checkValidTypeof(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() {
		return
	}
	opText := op.Text(source)
	switch opText {
	case "==", "===", "!=", "!==":
	default:
		return
	}
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if isTypeofExpr(left, source) && isStringLiteral(right, source) && !validTypeofValues[right.Text(source)] {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	} else if isTypeofExpr(right, source) && isStringLiteral(left, source) && !validTypeofValues[left.Text(source)] {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	}
}

func isTypeofExpr(node parser.Node, source []byte) bool {
	if node.IsNull() || node.Kind() != "unary_expression" {
		return false
	}
	op := node.ChildByFieldName("operator")
	return !op.IsNull() && op.Text(source) == "typeof"
}

func isStringLiteral(node parser.Node, source []byte) bool {
	if node.IsNull() {
		return false
	}
	t := node.Text(source)
	return node.Kind() == "string" && len(t) >= 2 && (t[0] == '"' || t[0] == '\'')
}
