package engine

import "github.com/Hideart/ralf/internal/parser"

// checkNoInnerDeclarations flags function declarations inside nested blocks
// (if/for/while bodies, etc.). Functions at program top level or directly
// inside a function body are allowed.
func checkNoInnerDeclarations(node parser.Node, _ []byte, lineStarts []int, diags *[]Diagnostic) {
	p := node.Parent()
	if p.IsNull() {
		return
	}
	pk := p.Kind()

	// Direct child of program — always OK.
	if pk == "program" {
		return
	}
	// Inside an export_statement at program level — OK.
	if pk == "export_statement" {
		gp := p.Parent()
		if !gp.IsNull() && gp.Kind() == "program" {
			return
		}
	}
	// Direct child of a function body's statement_block — OK.
	if pk == "statement_block" {
		gp := p.Parent()
		if !gp.IsNull() && isFunctionNode(gp.Kind()) {
			return
		}
	}

	// Otherwise it's inside a nested block — flag it.
	*diags = append(*diags, builtinDiag(node, lineStarts))
}
