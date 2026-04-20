package project

import (
	"context"
	"testing"

	"github.com/ralfjs/ralf/internal/parser"
)

func parseAndExtract(t *testing.T, source string, lang parser.Lang) ([]ImportInfo, []ExportInfo) { //nolint:unparam // lang will vary when TS tests are added
	t.Helper()
	src := []byte(source)
	p := parser.NewParser(lang)
	tree, err := p.Parse(context.Background(), src, nil)
	p.Close()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(src)
	return ExtractImportsExports(tree, src, lineStarts)
}

func TestExtract_NamedImport(t *testing.T) {
	imports, _ := parseAndExtract(t, `import { foo, bar } from './utils';`, parser.LangJS)
	if len(imports) != 2 {
		t.Fatalf("expected 2 imports, got %d", len(imports))
	}
	if imports[0].Name != "foo" || imports[1].Name != "bar" {
		t.Errorf("got imports %v", imports)
	}
	if imports[0].Source != "./utils" {
		t.Errorf("expected source ./utils, got %q", imports[0].Source)
	}
}

func TestExtract_DefaultImport(t *testing.T) {
	imports, _ := parseAndExtract(t, `import foo from './bar';`, parser.LangJS)
	if len(imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(imports))
	}
	if imports[0].Name != "default" {
		t.Errorf("expected name 'default', got %q", imports[0].Name)
	}
}

func TestExtract_NamespaceImport(t *testing.T) {
	imports, _ := parseAndExtract(t, `import * as utils from './utils';`, parser.LangJS)
	if len(imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(imports))
	}
	if imports[0].Name != "*" {
		t.Errorf("expected name '*', got %q", imports[0].Name)
	}
}

func TestExtract_SideEffectImport(t *testing.T) {
	imports, _ := parseAndExtract(t, `import './polyfill';`, parser.LangJS)
	if len(imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(imports))
	}
	if imports[0].Name != "*" || imports[0].Source != "./polyfill" {
		t.Errorf("got %v", imports[0])
	}
}

func TestExtract_ExportFunction(t *testing.T) {
	_, exports := parseAndExtract(t, `export function foo() {}`, parser.LangJS)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].Name != "foo" || exports[0].Kind != "function" {
		t.Errorf("got %v", exports[0])
	}
}

func TestExtract_ExportClass(t *testing.T) {
	_, exports := parseAndExtract(t, `export class Foo {}`, parser.LangJS)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].Name != "Foo" || exports[0].Kind != "class" {
		t.Errorf("got %v", exports[0])
	}
}

func TestExtract_ExportConst(t *testing.T) {
	_, exports := parseAndExtract(t, `export const x = 1, y = 2;`, parser.LangJS)
	if len(exports) != 2 {
		t.Fatalf("expected 2 exports, got %d", len(exports))
	}
	if exports[0].Name != "x" || exports[1].Name != "y" {
		t.Errorf("got %v", exports)
	}
}

func TestExtract_ExportDefault(t *testing.T) {
	_, exports := parseAndExtract(t, `export default function foo() {}`, parser.LangJS)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].Name != "default" || exports[0].Kind != "function" {
		t.Errorf("got %v", exports[0])
	}
}

func TestExtract_ExportDefaultExpression(t *testing.T) {
	_, exports := parseAndExtract(t, `export default 42;`, parser.LangJS)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].Name != "default" {
		t.Errorf("got %v", exports[0])
	}
}

func TestExtract_ExportClause(t *testing.T) {
	src := "const foo = 1;\nconst bar = 2;\nexport { foo, bar };"
	_, exports := parseAndExtract(t, src, parser.LangJS)
	if len(exports) != 2 {
		t.Fatalf("expected 2 exports, got %d", len(exports))
	}
}

func TestExtract_ReExport(t *testing.T) {
	imports, exports := parseAndExtract(t, `export { foo, bar } from './utils';`, parser.LangJS)
	if len(imports) != 2 {
		t.Fatalf("expected 2 imports from re-export, got %d", len(imports))
	}
	if len(exports) != 2 {
		t.Fatalf("expected 2 exports from re-export, got %d", len(exports))
	}
	if imports[0].Source != "./utils" {
		t.Errorf("expected source ./utils, got %q", imports[0].Source)
	}
}

func TestExtract_ReExportStar(t *testing.T) {
	imports, _ := parseAndExtract(t, `export * from './utils';`, parser.LangJS)
	if len(imports) != 1 {
		t.Fatalf("expected 1 import from star re-export, got %d", len(imports))
	}
	if imports[0].Name != "*" {
		t.Errorf("expected name *, got %q", imports[0].Name)
	}
}

func TestExtract_CJSRequire(t *testing.T) {
	imports, _ := parseAndExtract(t, `const foo = require('./bar');`, parser.LangJS)
	if len(imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(imports))
	}
	if imports[0].Source != "./bar" {
		t.Errorf("expected source ./bar, got %q", imports[0].Source)
	}
}

func TestExtract_CJSModuleExports(t *testing.T) {
	_, exports := parseAndExtract(t, `module.exports = { foo: 1 };`, parser.LangJS)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].Name != "default" {
		t.Errorf("expected default export, got %q", exports[0].Name)
	}
}

func TestExtract_CJSExportsNamed(t *testing.T) {
	_, exports := parseAndExtract(t, `exports.foo = 42;`, parser.LangJS)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].Name != "foo" {
		t.Errorf("expected export name foo, got %q", exports[0].Name)
	}
}

func TestExtract_ExportAlias(t *testing.T) {
	src := "const foo = 1;\nexport { foo as bar };"
	_, exports := parseAndExtract(t, src, parser.LangJS)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if exports[0].Name != "bar" {
		t.Errorf("expected exported name 'bar', got %q", exports[0].Name)
	}
}

func TestExtractFile(t *testing.T) {
	source := []byte(`import { foo } from './utils'; export function bar() {}`)
	imports, exports, err := ExtractFile(context.Background(), "test.js", source)
	if err != nil {
		t.Fatal(err)
	}
	if len(imports) != 1 {
		t.Errorf("expected 1 import, got %d", len(imports))
	}
	if len(exports) != 1 {
		t.Errorf("expected 1 export, got %d", len(exports))
	}
}

func TestExtractFile_UnsupportedType(t *testing.T) {
	_, _, err := ExtractFile(context.Background(), "test.txt", []byte("hello"))
	if err == nil {
		t.Error("expected error for unsupported file type")
	}
}

func TestExtractFileWithTree_ParityWithExtractFile(t *testing.T) {
	t.Parallel()

	source := []byte(`import { foo, bar } from './utils';
import Baz from './baz';
export function alpha() {}
export const beta = 1;
export { gamma } from './other';`)

	// Baseline: internal parse.
	baseImports, baseExports, err := ExtractFile(context.Background(), "test.js", source)
	if err != nil {
		t.Fatalf("ExtractFile: %v", err)
	}

	// WithTree: pre-parsed.
	p := parser.NewParser(parser.LangJS)
	t.Cleanup(p.Close)
	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Cleanup(tree.Close)

	withImports, withExports, err := ExtractFileWithTree(context.Background(), "test.js", source, tree)
	if err != nil {
		t.Fatalf("ExtractFileWithTree: %v", err)
	}

	if len(baseImports) != len(withImports) || len(baseExports) != len(withExports) {
		t.Fatalf("parity: baseline %d imports %d exports, withTree %d imports %d exports",
			len(baseImports), len(baseExports), len(withImports), len(withExports))
	}
	for i := range baseImports {
		if baseImports[i] != withImports[i] {
			t.Errorf("import[%d]: base=%+v with=%+v", i, baseImports[i], withImports[i])
		}
	}
	for i := range baseExports {
		if baseExports[i] != withExports[i] {
			t.Errorf("export[%d]: base=%+v with=%+v", i, baseExports[i], withExports[i])
		}
	}
}
