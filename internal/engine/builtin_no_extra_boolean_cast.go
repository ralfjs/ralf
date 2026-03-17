package engine

import "github.com/Hideart/ralf/internal/parser"

func checkNoExtraBooleanCast(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	// Check for !!x pattern: unary "!" whose argument is also unary "!"
	op := node.ChildByFieldName("operator")
	if op.IsNull() || op.Text(source) != "!" {
		return
	}
	arg := node.ChildByFieldName("argument")
	if arg.IsNull() || arg.Kind() != "unary_expression" {
		return
	}
	innerOp := arg.ChildByFieldName("operator")
	if innerOp.IsNull() || innerOp.Text(source) != "!" {
		return
	}
	// Check if in a boolean context. Walk up parents, skipping
	// parenthesized_expression wrappers.
	if isInBooleanContext(node) {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	}
}

// isInBooleanContext checks if node is the condition of an if/while/do/for/ternary.
func isInBooleanContext(node parser.Node) bool {
	child := node
	parent := node.Parent()
	for !parent.IsNull() {
		pk := parent.Kind()
		// Skip parenthesized wrappers
		if pk == "parenthesized_expression" {
			child = parent
			parent = parent.Parent()
			continue
		}
		switch pk {
		case "if_statement", "while_statement", "do_statement", "for_statement", "ternary_expression":
			cond := parent.ChildByFieldName("condition")
			return !cond.IsNull() && cond.StartByte() == child.StartByte() && cond.EndByte() == child.EndByte()
		}
		return false
	}
	return false
}
