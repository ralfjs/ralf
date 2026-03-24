package engine

import "github.com/ralfjs/ralf/internal/parser"

func checkForDirection(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	condition := node.ChildByFieldName("condition")
	increment := node.ChildByFieldName("increment")
	if condition.IsNull() || increment.IsNull() {
		return
	}
	if condition.Kind() != "binary_expression" {
		return
	}
	condOp := condition.ChildByFieldName("operator")
	if condOp.IsNull() {
		return
	}
	condOpText := condOp.Text(source)
	condLeft := condition.ChildByFieldName("left")
	condRight := condition.ChildByFieldName("right")
	if condLeft.IsNull() || condRight.IsNull() {
		return
	}

	// Determine counter name and effective operator direction.
	// Supports counter on either side: "i < 10" and "10 > i".
	var counterName string
	effectiveOp := condOpText
	switch {
	case condLeft.Kind() == "identifier":
		counterName = condLeft.Text(source)
	case condRight.Kind() == "identifier":
		counterName = condRight.Text(source)
		// Flip operator when counter is on the right: "10 > i" ≡ "i < 10"
		switch condOpText {
		case "<":
			effectiveOp = ">"
		case "<=":
			effectiveOp = ">="
		case ">":
			effectiveOp = "<"
		case ">=":
			effectiveOp = "<="
		}
	default:
		return
	}

	dir := updateDirection(increment, counterName, source)
	wrongDirection := false
	switch effectiveOp {
	case "<", "<=":
		wrongDirection = dir < 0
	case ">", ">=":
		wrongDirection = dir > 0
	}

	if wrongDirection {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	}
}

// updateDirection returns +1 if the increment moves the counter up,
// -1 if down, or 0 if indeterminate. Examines the AST of the update
// expression rather than doing fragile string matching.
func updateDirection(inc parser.Node, counter string, source []byte) int {
	kind := inc.Kind()
	// i++ / i--
	if kind == "update_expression" {
		arg := inc.ChildByFieldName("argument")
		if arg.IsNull() || arg.Text(source) != counter {
			return 0
		}
		op := inc.ChildByFieldName("operator")
		if op.IsNull() {
			return 0
		}
		if op.Text(source) == "++" {
			return 1
		}
		return -1
	}
	// i += N / i -= N
	if kind == "augmented_assignment_expression" {
		left := inc.ChildByFieldName("left")
		if left.IsNull() || left.Text(source) != counter {
			return 0
		}
		op := inc.ChildByFieldName("operator")
		if op.IsNull() {
			return 0
		}
		if op.Text(source) == "+=" {
			return 1
		}
		if op.Text(source) == "-=" {
			return -1
		}
		return 0
	}
	// i = i + N / i = i - N
	if kind == "assignment_expression" {
		left := inc.ChildByFieldName("left")
		if left.IsNull() || left.Text(source) != counter {
			return 0
		}
		right := inc.ChildByFieldName("right")
		if right.IsNull() || right.Kind() != "binary_expression" {
			return 0
		}
		rLeft := right.ChildByFieldName("left")
		rOp := right.ChildByFieldName("operator")
		if rLeft.IsNull() || rOp.IsNull() || rLeft.Text(source) != counter {
			return 0
		}
		if rOp.Text(source) == "+" {
			return 1
		}
		if rOp.Text(source) == "-" {
			return -1
		}
	}
	// Also handle sequence_expression (e.g., i++, j++)
	if kind == "sequence_expression" {
		for i := uint(0); i < inc.NamedChildCount(); i++ {
			if d := updateDirection(inc.NamedChild(i), counter, source); d != 0 {
				return d
			}
		}
	}
	return 0
}
