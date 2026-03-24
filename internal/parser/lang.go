// Package parser provides tree-sitter based parsing for JavaScript and
// TypeScript source files.
package parser

import (
	"path/filepath"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Lang identifies a supported language variant.
type Lang uint8

// Supported language variants.
const (
	LangJS  Lang = iota // JavaScript (.js, .mjs, .cjs)
	LangJSX             // JSX (.jsx)
	LangTS              // TypeScript (.ts, .mts, .cts)
	LangTSX             // TSX (.tsx)
)

// LangFromPath returns the language for the given file path based on its
// extension. The second return value is false if the extension is not
// recognised.
func LangFromPath(path string) (Lang, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".js", ".mjs", ".cjs":
		return LangJS, true
	case ".jsx":
		return LangJSX, true
	case ".ts", ".mts", ".cts":
		return LangTS, true
	case ".tsx":
		return LangTSX, true
	default:
		return 0, false
	}
}

// Language returns the tree-sitter grammar for this language variant.
//
// JS and JSX share the JavaScript grammar (JSX is part of the grammar).
// TS and TSX use separate grammars from the tree-sitter-typescript package.
func (l Lang) Language() *tree_sitter.Language {
	switch l {
	case LangJS, LangJSX:
		return tree_sitter.NewLanguage(tree_sitter_javascript.Language())
	case LangTS:
		return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	case LangTSX:
		return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
	default:
		panic("parser: unknown Lang value")
	}
}

// String returns the human-readable name of the language.
func (l Lang) String() string {
	switch l {
	case LangJS:
		return "JavaScript"
	case LangJSX:
		return "JSX"
	case LangTS:
		return "TypeScript"
	case LangTSX:
		return "TSX"
	default:
		return "Unknown"
	}
}
