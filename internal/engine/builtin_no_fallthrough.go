package engine

import "github.com/Hideart/ralf/internal/parser"

// checkNoFallthrough flags switch cases that fall through to the next case
// without a break, return, throw, or continue statement.
// This is an AST-based simplification; full CFG analysis is planned for v1.0.
func checkNoFallthrough(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	parent := node.Parent()
	if parent.IsNull() {
		return
	}

	// Check if this is the last case — last case can't fall through.
	isLast := true
	foundSelf := false
	for i := uint(0); i < parent.NamedChildCount(); i++ {
		child := parent.NamedChild(i)
		if foundSelf {
			isLast = false
			break
		}
		if child.StartByte() == node.StartByte() && child.EndByte() == node.EndByte() {
			foundSelf = true
		}
	}
	if isLast {
		return
	}

	// Count consequent statements: named children after the first (the case value).
	// For default cases, the first named child might be a statement already.
	stmtCount := node.NamedChildCount()
	if stmtCount <= 1 {
		// Empty case body — intentional grouping (case 1: case 2: break;).
		return
	}

	lastStmt := node.NamedChild(stmtCount - 1)
	if isTerminatingStmt(lastStmt) {
		return
	}

	*diags = append(*diags, builtinDiag(node, lineStarts))
}

// isTerminatingStmt reports whether a statement guarantees no fallthrough.
func isTerminatingStmt(node parser.Node) bool {
	switch node.Kind() {
	case "break_statement", "return_statement", "throw_statement", "continue_statement":
		return true
	case "statement_block":
		// Check last statement in block.
		if n := node.NamedChildCount(); n > 0 {
			return isTerminatingStmt(node.NamedChild(n - 1))
		}
	case "if_statement":
		// Both branches must terminate.
		consequence := node.ChildByFieldName("consequence")
		alternative := node.ChildByFieldName("alternative")
		if alternative.IsNull() {
			return false
		}
		return isTerminatingStmt(consequence) && isTerminatingStmt(alternative)
	}
	return false
}
