package project

import (
	"context"
	"fmt"

	"github.com/ralfjs/ralf/internal/parser"
)

// ExtractFile parses a file and extracts its imports and exports.
// Convenience wrapper around parser + ExtractImportsExports. Callers that
// already have a parsed tree should use ExtractFileWithTree to avoid
// re-parsing.
func ExtractFile(ctx context.Context, filePath string, source []byte) ([]ImportInfo, []ExportInfo, error) {
	return ExtractFileWithTree(ctx, filePath, source, nil)
}

// ExtractFileWithTree is like ExtractFile but accepts a pre-parsed tree.
// When tree is nil, it parses internally. When tree is non-nil the caller
// retains ownership; the tree is not closed by this function. The tree
// must have been parsed from the same source bytes.
//
// Unsupported file types return the same error as ExtractFile regardless
// of whether a tree is provided.
func ExtractFileWithTree(ctx context.Context, filePath string, source []byte, tree *parser.Tree) ([]ImportInfo, []ExportInfo, error) {
	lang, ok := parser.LangFromPath(filePath)
	if !ok {
		return nil, nil, fmt.Errorf("unsupported file type: %s", filePath)
	}

	if tree == nil {
		p := parser.NewParser(lang)
		parsed, err := p.Parse(ctx, source, nil)
		p.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", filePath, err)
		}
		defer parsed.Close()
		tree = parsed
	}

	lineStarts := buildLineIndex(source)
	imports, exports := ExtractImportsExports(tree, source, lineStarts)
	return imports, exports, nil
}
