package engine

import (
	"fmt"
	"strings"

	"github.com/ralfjs/ralf/internal/parser"
)

func checkNoUnsafeFinally(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	kind := node.Kind()
	// Walk ancestors to see if we're inside a finally_clause.
	// Stop at function boundaries (we don't flag returns in nested functions).
	p := node.Parent()
	for !p.IsNull() {
		pk := p.Kind()
		if pk == "finally_clause" {
			d := builtinDiag(node, lineStarts)
			d.Message = fmt.Sprintf("Unsafe usage of %s in finally block.", strings.TrimSuffix(kind, "_statement"))
			*diags = append(*diags, d)
			break
		}
		if isFunctionNode(pk) {
			break
		}
		p = p.Parent()
	}
}
