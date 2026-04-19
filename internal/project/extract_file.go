package project

import (
	"context"
	"fmt"

	"github.com/ralfjs/ralf/internal/parser"
)

// ExtractFile parses a file and extracts its imports and exports.
// Convenience wrapper around parser + ExtractImportsExports.
func ExtractFile(ctx context.Context, filePath string, source []byte) ([]ImportInfo, []ExportInfo, error) {
	lang, ok := parser.LangFromPath(filePath)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported file type: %s", filePath)
	}

	p := parser.NewParser(lang)
	tree, err := p.Parse(ctx, source, nil)
	p.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(source)
	imports, exports := ExtractImportsExports(tree, source, lineStarts)
	return imports, exports, nil
}
