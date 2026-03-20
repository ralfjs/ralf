package engine

import (
	"github.com/Hideart/ralf/internal/parser"
)

func checkEqeqeq(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	op := node.ChildByFieldName("operator")
	if op.IsNull() {
		return
	}
	// Fast reject: check byte length (== is 2, != is 2; === is 3, !== is 3).
	s, e := op.StartByte(), op.EndByte()
	if e-s != 2 {
		return
	}
	opText := string(source[s:e])
	if opText != "==" && opText != "!=" {
		return
	}

	// Allow x == null and x != null (ESLint "smart" mode default).
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if (!left.IsNull() && left.Text(source) == "null") ||
		(!right.IsNull() && right.Text(source) == "null") {
		return
	}

	expected := "==="
	if opText == "!=" {
		expected = "!=="
	}

	d := builtinDiag(op, lineStarts)
	d.Message = "Expected '" + expected + "' and instead saw '" + opText + "'."
	*diags = append(*diags, d)
}
