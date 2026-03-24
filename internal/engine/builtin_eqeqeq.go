package engine

import (
	"github.com/ralfjs/ralf/internal/parser"
)

func checkEqeqeq(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() {
		return
	}
	// Fast reject: == and != are 2 bytes; === and !== are 3.
	s, e := op.StartByte(), op.EndByte()
	if e-s != 2 {
		return
	}
	isEq := source[s] == '=' && source[s+1] == '=' // ==
	isNe := source[s] == '!' && source[s+1] == '=' // !=
	if !isEq && !isNe {
		return
	}

	// Allow x == null and x != null (ESLint "smart" mode default).
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if (!left.IsNull() && left.Text(source) == "null") ||
		(!right.IsNull() && right.Text(source) == "null") {
		return
	}

	d := builtinDiag(op, lineStarts)
	if isEq {
		d.Message = "Expected '===' and instead saw '=='."
	} else {
		d.Message = "Expected '!==' and instead saw '!='."
	}
	*diags = append(*diags, d)
}
