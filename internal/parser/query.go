package parser

import (
	"fmt"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Query wraps a compiled tree-sitter S-expression query pattern.
//
// Every Query must be closed after use to release C resources.
type Query struct {
	inner *tree_sitter.Query
	lang  Lang
}

// Match represents a single pattern match from a query execution.
type Match struct {
	PatternIndex uint
	Captures     []Capture
}

// Capture represents a single captured node within a match.
type Capture struct {
	Name string
	Node Node
}

// NewQuery compiles an S-expression pattern for the given language.
func NewQuery(lang Lang, pattern string) (*Query, error) {
	q, qErr := tree_sitter.NewQuery(lang.Language(), pattern)
	if qErr != nil {
		return nil, fmt.Errorf("parser: query compile error: %w", qErr)
	}
	return &Query{inner: q, lang: lang}, nil
}

// Exec runs the query against a node and returns all matches.
func (q *Query) Exec(node Node, source []byte) []Match {
	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	names := q.inner.CaptureNames()
	matches := cursor.Matches(q.inner, node.inner, source)

	var result []Match
	for {
		m := matches.Next()
		if m == nil {
			break
		}

		captures := make([]Capture, len(m.Captures))
		for i, c := range m.Captures {
			name := ""
			if int(c.Index) < len(names) {
				name = names[c.Index]
			}
			captures[i] = Capture{
				Name: name,
				Node: newNode(&c.Node),
			}
		}

		result = append(result, Match{
			PatternIndex: m.PatternIndex,
			Captures:     captures,
		})
	}

	return result
}

// Close releases the underlying C resources.
func (q *Query) Close() {
	q.inner.Close()
}
