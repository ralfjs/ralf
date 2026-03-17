package engine

import "github.com/Hideart/ralf/internal/parser"

func isOctalLiteral(text string) bool {
	if len(text) < 2 || text[0] != '0' {
		return false
	}
	for _, c := range text[1:] {
		if c < '0' || c > '7' {
			return false
		}
	}
	return true
}

func checkNoOctal(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	if !isOctalLiteral(node.Text(source)) {
		return
	}
	*diags = append(*diags, builtinDiag(node, lineStarts))
}
