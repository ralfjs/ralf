package parser

import (
	"context"
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Parser wraps a tree-sitter parser for a specific language.
//
// Every Parser must be closed after use to release C resources.
type Parser struct {
	inner *tree_sitter.Parser
	lang  Lang
}

// NewParser creates a parser configured for the given language.
func NewParser(lang Lang) *Parser {
	p := tree_sitter.NewParser()
	if err := p.SetLanguage(lang.Language()); err != nil {
		// Language grammars are compiled-in; a mismatch here is a build bug.
		panic(fmt.Sprintf("parser: failed to set language %s: %v", lang, err))
	}
	return &Parser{inner: p, lang: lang}
}

// Parse parses the given source code and returns a syntax tree.
//
// If oldTree is non-nil, the parser performs an incremental reparse. The
// caller must have called oldTree.Edit() to describe the changes before
// calling Parse again.
//
// The returned Tree must be closed after use.
//
// If ctx is cancelled during parsing, the returned tree may be nil.
func (p *Parser) Parse(ctx context.Context, source []byte, oldTree *Tree) (*Tree, error) {
	var old *tree_sitter.Tree
	if oldTree != nil {
		old = oldTree.inner
	}

	var cancelled bool
	opts := &tree_sitter.ParseOptions{
		ProgressCallback: func(_ tree_sitter.ParseState) bool {
			select {
			case <-ctx.Done():
				cancelled = true
				return true
			default:
				return false
			}
		},
	}

	result := p.inner.ParseWithOptions(func(offset int, _ tree_sitter.Point) []byte {
		if offset < len(source) {
			return source[offset:]
		}
		return nil
	}, old, opts)

	if cancelled {
		if result != nil {
			result.Close()
		}
		return nil, ctx.Err()
	}

	if result == nil {
		return nil, fmt.Errorf("parser: parse returned nil for language %s", p.lang)
	}

	return &Tree{inner: result}, nil
}

// Close releases the underlying C resources. Must be called when the parser
// is no longer needed.
func (p *Parser) Close() {
	p.inner.Close()
}
