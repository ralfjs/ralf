package project

import (
	"github.com/ralfjs/ralf/internal/parser"
)

// ExtractImportsExports walks the top-level AST nodes and extracts import
// and export information. The caller owns the tree and source bytes.
func ExtractImportsExports(tree *parser.Tree, source []byte, lineStarts []int) ([]ImportInfo, []ExportInfo) {
	root := tree.RootNode()
	count := root.NamedChildCount()

	var imports []ImportInfo
	var exports []ExportInfo

	sourceFieldID := root.FieldID("source")
	nameFieldID := root.FieldID("name")
	declarationFieldID := root.FieldID("declaration")

	for i := range count {
		child := root.NamedChild(i)
		kind := child.Kind()

		switch kind {
		case "import_statement":
			imp := extractImport(child, source, lineStarts, sourceFieldID)
			imports = append(imports, imp...)

		case "export_statement":
			// Re-exports have a source field — they create both import and export edges.
			var srcNode parser.Node
			if sourceFieldID != 0 {
				srcNode = child.ChildByFieldID(sourceFieldID)
			} else {
				srcNode = child.ChildByFieldName("source")
			}

			if !srcNode.IsNull() {
				// Re-export: export { foo } from './bar'
				imp, exp := extractReExport(child, source, lineStarts, srcNode)
				imports = append(imports, imp...)
				exports = append(exports, exp...)
			} else {
				exp := extractExport(child, source, lineStarts, declarationFieldID, nameFieldID)
				exports = append(exports, exp...)
			}

		case "expression_statement":
			// CJS: module.exports = ... or exports.foo = ...
			exp := extractCJSExport(child, source, lineStarts)
			exports = append(exports, exp...)

			// CJS: require('./foo')
			imp := extractCJSRequire(child, source, lineStarts)
			imports = append(imports, imp...)

		case "lexical_declaration", "variable_declaration":
			// const foo = require('./bar')
			imp := extractRequireFromDecl(child, source, lineStarts)
			imports = append(imports, imp...)
		}
	}

	return imports, exports
}

func extractImport(node parser.Node, source []byte, lineStarts []int, sourceFieldID uint16) []ImportInfo {
	var srcNode parser.Node
	if sourceFieldID != 0 {
		srcNode = node.ChildByFieldID(sourceFieldID)
	} else {
		srcNode = node.ChildByFieldName("source")
	}
	if srcNode.IsNull() {
		return nil
	}

	modPath := stripQuotes(srcNode.Text(source))
	line := offsetToLine(lineStarts, int(node.StartByte())) //nolint:gosec // tree-sitter offset fits int

	// Determine imported names from import clause.
	names := extractImportNames(node, source)
	if len(names) == 0 {
		// Side-effect import: import './module'
		return []ImportInfo{{Source: modPath, Name: "*", Line: line}}
	}

	result := make([]ImportInfo, len(names))
	for i, name := range names {
		result[i] = ImportInfo{Source: modPath, Name: name, Line: line}
	}
	return result
}

func extractImportNames(node parser.Node, source []byte) []string {
	var names []string
	for i := range node.NamedChildCount() {
		child := node.NamedChild(i)
		if child.Kind() == "import_clause" {
			names = append(names, extractImportClauseNames(child, source)...)
		}
	}
	return names
}

func extractImportClauseNames(clause parser.Node, source []byte) []string {
	var names []string
	for i := range clause.NamedChildCount() {
		child := clause.NamedChild(i)
		switch child.Kind() {
		case "identifier":
			// Default import: import foo from './bar'
			names = append(names, "default")
		case "named_imports":
			for j := range child.NamedChildCount() {
				spec := child.NamedChild(j)
				if spec.Kind() == "import_specifier" {
					nameNode := spec.ChildByFieldName("name")
					if !nameNode.IsNull() {
						names = append(names, nameNode.Text(source))
					}
				}
			}
		case "namespace_import":
			// import * as foo from './bar'
			names = append(names, "*")
		}
	}
	return names
}

func extractExport(node parser.Node, source []byte, lineStarts []int, declarationFieldID, nameFieldID uint16) []ExportInfo {
	// Check for export clause: export { foo, bar }
	for i := range node.NamedChildCount() {
		child := node.NamedChild(i)
		if child.Kind() == "export_clause" {
			return extractExportClause(child, source, lineStarts)
		}
	}

	// Check for declaration: export function foo() {}, export class Foo {}, export const foo = ...
	var decl parser.Node
	if declarationFieldID != 0 {
		decl = node.ChildByFieldID(declarationFieldID)
	} else {
		decl = node.ChildByFieldName("declaration")
	}
	if decl.IsNull() {
		// Check for default export: export default expression
		if hasDefaultKeyword(node, source) {
			line := offsetToLine(lineStarts, int(node.StartByte())) //nolint:gosec // tree-sitter offset fits int
			return []ExportInfo{{Name: "default", Kind: "value", Line: line}}
		}
		return nil
	}

	line := offsetToLine(lineStarts, int(decl.StartByte())) //nolint:gosec // tree-sitter offset fits int

	// Check if this is a default export with a declaration.
	isDefault := hasDefaultKeyword(node, source)

	switch decl.Kind() {
	case "function_declaration", "generator_function_declaration":
		name := "default"
		if !isDefault {
			nameNode := decl.ChildByFieldName("name")
			if !nameNode.IsNull() {
				name = nameNode.Text(source)
			}
		}
		return []ExportInfo{{Name: name, Kind: "function", Line: line}}

	case "class_declaration":
		name := "default"
		if !isDefault {
			nameNode := decl.ChildByFieldName("name")
			if !nameNode.IsNull() {
				name = nameNode.Text(source)
			}
		}
		return []ExportInfo{{Name: name, Kind: "class", Line: line}}

	case "lexical_declaration":
		return extractVarExports(decl, source, lineStarts)

	case "type_alias_declaration", "interface_declaration":
		var nameNode parser.Node
		if nameFieldID != 0 {
			nameNode = decl.ChildByFieldID(nameFieldID)
		} else {
			nameNode = decl.ChildByFieldName("name")
		}
		if !nameNode.IsNull() {
			return []ExportInfo{{Name: nameNode.Text(source), Kind: "type", Line: line}}
		}
	}

	if isDefault {
		return []ExportInfo{{Name: "default", Kind: "value", Line: line}}
	}
	return nil
}

func extractExportClause(clause parser.Node, source []byte, lineStarts []int) []ExportInfo {
	var exports []ExportInfo
	for i := range clause.NamedChildCount() {
		spec := clause.NamedChild(i)
		if spec.Kind() != "export_specifier" {
			continue
		}
		// Use alias if present, otherwise use name.
		alias := spec.ChildByFieldName("alias")
		nameNode := spec.ChildByFieldName("name")
		var name string
		switch {
		case !alias.IsNull():
			name = alias.Text(source)
		case !nameNode.IsNull():
			name = nameNode.Text(source)
		default:
			continue
		}
		line := offsetToLine(lineStarts, int(spec.StartByte())) //nolint:gosec // tree-sitter offset fits int
		exports = append(exports, ExportInfo{Name: name, Kind: "value", Line: line})
	}
	return exports
}

func extractReExport(node parser.Node, source []byte, lineStarts []int, srcNode parser.Node) ([]ImportInfo, []ExportInfo) {
	modPath := stripQuotes(srcNode.Text(source))
	line := offsetToLine(lineStarts, int(node.StartByte())) //nolint:gosec // tree-sitter offset fits int

	var imports []ImportInfo
	var exports []ExportInfo

	// Find export clause for re-export names.
	for i := range node.NamedChildCount() {
		child := node.NamedChild(i)
		if child.Kind() == "export_clause" {
			for j := range child.NamedChildCount() {
				spec := child.NamedChild(j)
				if spec.Kind() != "export_specifier" {
					continue
				}
				nameNode := spec.ChildByFieldName("name")
				alias := spec.ChildByFieldName("alias")
				importName := ""
				exportName := ""
				if !nameNode.IsNull() {
					importName = nameNode.Text(source)
				}
				if !alias.IsNull() {
					exportName = alias.Text(source)
				} else {
					exportName = importName
				}
				imports = append(imports, ImportInfo{Source: modPath, Name: importName, Line: line})
				exports = append(exports, ExportInfo{Name: exportName, Kind: "value", Line: line})
			}
			return imports, exports
		}
	}

	// export * from './bar'
	imports = append(imports, ImportInfo{Source: modPath, Name: "*", Line: line})
	return imports, exports
}

func extractVarExports(decl parser.Node, source []byte, lineStarts []int) []ExportInfo {
	var exports []ExportInfo
	for i := range decl.NamedChildCount() {
		child := decl.NamedChild(i)
		if child.Kind() == "variable_declarator" {
			nameNode := child.ChildByFieldName("name")
			if !nameNode.IsNull() && nameNode.Kind() == "identifier" {
				line := offsetToLine(lineStarts, int(child.StartByte())) //nolint:gosec // tree-sitter offset fits int
				exports = append(exports, ExportInfo{Name: nameNode.Text(source), Kind: "variable", Line: line})
			}
		}
	}
	return exports
}

func extractCJSExport(node parser.Node, source []byte, lineStarts []int) []ExportInfo {
	// Look for assignment_expression as first named child.
	if node.NamedChildCount() == 0 {
		return nil
	}
	assign := node.NamedChild(0)
	if assign.Kind() != "assignment_expression" {
		return nil
	}
	left := assign.ChildByFieldName("left")
	if left.IsNull() || left.Kind() != "member_expression" {
		return nil
	}

	obj := left.ChildByFieldName("object")
	prop := left.ChildByFieldName("property")
	if obj.IsNull() || prop.IsNull() {
		return nil
	}

	objText := obj.Text(source)
	propText := prop.Text(source)

	line := offsetToLine(lineStarts, int(node.StartByte())) //nolint:gosec // tree-sitter offset fits int

	if objText == "module" && propText == "exports" {
		return []ExportInfo{{Name: "default", Kind: "value", Line: line}}
	}
	if objText == "exports" {
		return []ExportInfo{{Name: propText, Kind: "value", Line: line}}
	}
	return nil
}

func extractCJSRequire(node parser.Node, source []byte, lineStarts []int) []ImportInfo {
	// expression_statement containing call_expression with require.
	if node.NamedChildCount() == 0 {
		return nil
	}
	call := node.NamedChild(0)
	return extractRequireCall(call, source, lineStarts)
}

func extractRequireFromDecl(node parser.Node, source []byte, lineStarts []int) []ImportInfo {
	var imports []ImportInfo
	for i := range node.NamedChildCount() {
		child := node.NamedChild(i)
		if child.Kind() == "variable_declarator" {
			value := child.ChildByFieldName("value")
			if !value.IsNull() {
				imports = append(imports, extractRequireCall(value, source, lineStarts)...)
			}
		}
	}
	return imports
}

func extractRequireCall(call parser.Node, source []byte, lineStarts []int) []ImportInfo {
	if call.Kind() != "call_expression" {
		return nil
	}
	fn := call.ChildByFieldName("function")
	if fn.IsNull() || fn.Kind() != "identifier" || fn.Text(source) != "require" {
		return nil
	}
	args := call.ChildByFieldName("arguments")
	if args.IsNull() || args.NamedChildCount() == 0 {
		return nil
	}
	firstArg := args.NamedChild(0)
	if firstArg.Kind() != "string" {
		return nil
	}
	modPath := stripQuotes(firstArg.Text(source))
	line := offsetToLine(lineStarts, int(call.StartByte())) //nolint:gosec // tree-sitter offset fits int
	return []ImportInfo{{Source: modPath, Name: "*", Line: line}}
}

func hasDefaultKeyword(node parser.Node, source []byte) bool {
	for i := range node.ChildCount() {
		child := node.Child(i)
		if !child.IsNamed() {
			s, e := child.StartByte(), child.EndByte()
			if e-s == 7 && string(source[s:e]) == "default" {
				return true
			}
		}
	}
	return false
}

// stripQuotes removes surrounding quotes from a string literal.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		first := s[0]
		last := s[len(s)-1]
		if (first == '"' || first == '\'' || first == '`') && first == last {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// offsetToLine converts a byte offset to a 1-based line and 0-based column.
// Duplicated from engine/lineindex.go to avoid engine dependency.
func offsetToLine(lineStarts []int, offset int) int {
	lo, hi := 0, len(lineStarts)
	for lo < hi {
		mid := (lo + hi) / 2
		if lineStarts[mid] <= offset {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo // 1-based
}

// buildLineIndex builds line start offsets for binary-search line resolution.
// Duplicated from engine/lineindex.go to avoid engine dependency.
func buildLineIndex(source []byte) []int {
	lineStarts := make([]int, 1, len(source)/40+1)
	lineStarts[0] = 0
	for i, b := range source {
		if b == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}
	return lineStarts
}
