package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/parser"
)

// ErrPatternCompile indicates a pattern rule failed to compile.
var ErrPatternCompile = errors.New("pattern compile failed")

// patternNodeKind classifies a node in the compiled pattern tree.
type patternNodeKind uint8

const (
	patternLiteral  patternNodeKind = iota // exact match on kind + text
	patternWild                            // $NAME — matches any single node
	patternVariadic                        // $$$NAME — matches zero or more nodes
)

// patternNode is a node in the compiled pattern tree.
type patternNode struct {
	kind     patternNodeKind
	name     string // metavar name (without $/$$$)
	nodeKind string // tree-sitter kind for literals
	text     string // exact text for literal leaves
	children []patternNode
	isNamed  bool
}

// compiledPattern is a pre-compiled AST pattern rule ready for matching.
type compiledPattern struct {
	name     string
	root     patternNode
	message  string
	severity config.Severity
}

// compilePatternRules extracts rules with Pattern != "" and Severity != Off,
// compiles each, and returns the compiled set plus any compilation errors.
// It does not fail fast — all bad patterns are collected.
func compilePatternRules(rules map[string]config.RuleConfig) ([]compiledPattern, []error) {
	var compiled []compiledPattern
	var errs []error

	for name := range rules {
		rule := rules[name]
		if rule.Pattern == "" || rule.Severity == config.SeverityOff {
			continue
		}

		cp, err := compilePattern(name, &rule)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		compiled = append(compiled, cp)
	}

	return compiled, errs
}

// compilePattern parses a pattern string as JS via tree-sitter and builds a
// patternNode tree. Patterns are always parsed as JavaScript since JS
// identifiers allow $ and the grammar covers the expression patterns needed.
func compilePattern(name string, rule *config.RuleConfig) (compiledPattern, error) {
	p := parser.NewParser(parser.LangJS)
	defer p.Close()

	source := []byte(rule.Pattern)
	tree, err := p.Parse(context.Background(), source, nil)
	if err != nil {
		return compiledPattern{}, fmt.Errorf("rule %q: %w: %w", name, ErrPatternCompile, err)
	}
	defer tree.Close()

	root := tree.RootNode()
	if root.IsNull() {
		return compiledPattern{}, fmt.Errorf("rule %q: %w: empty parse tree", name, ErrPatternCompile)
	}

	// Reject patterns with syntax errors — they would silently never match
	// since matchPatterns skips ERROR nodes.
	if root.HasError() {
		return compiledPattern{}, fmt.Errorf("rule %q: %w: pattern has syntax errors", name, ErrPatternCompile)
	}

	// Unwrap program > expression_statement wrappers to get the meaningful node.
	inner := unwrapPattern(root)

	pn := buildPatternNode(inner, source)

	return compiledPattern{
		name:     name,
		root:     pn,
		message:  rule.Message,
		severity: rule.Severity,
	}, nil
}

// unwrapPattern skips program > expression_statement wrappers to get the
// meaningful expression/statement node from a parsed pattern.
func unwrapPattern(root parser.Node) parser.Node {
	node := root

	// Skip "program" wrapper.
	if node.Kind() == "program" && node.NamedChildCount() == 1 {
		node = node.NamedChild(0)
	}

	// Skip "expression_statement" wrapper — when a pattern is an expression,
	// tree-sitter wraps it as expression_statement. The sole named child is
	// always the expression itself.
	if node.Kind() == "expression_statement" && node.NamedChildCount() == 1 {
		node = node.NamedChild(0)
	}

	return node
}

// buildPatternNode recursively converts a tree-sitter node into a patternNode.
func buildPatternNode(node parser.Node, source []byte) patternNode {
	text := node.Text(source)

	// Detect metavariables: $$$NAME (variadic) or $NAME (wild).
	// Only on identifier-like leaf nodes.
	if isMetavarCandidate(node) {
		if strings.HasPrefix(text, "$$$") {
			return patternNode{
				kind:    patternVariadic,
				name:    strings.TrimPrefix(text, "$$$"),
				isNamed: node.IsNamed(),
			}
		}
		if strings.HasPrefix(text, "$") {
			return patternNode{
				kind:    patternWild,
				name:    strings.TrimPrefix(text, "$"),
				isNamed: node.IsNamed(),
			}
		}
	}

	children := node.CollectChildren()
	pn := patternNode{
		kind:     patternLiteral,
		nodeKind: node.Kind(),
		isNamed:  node.IsNamed(),
	}

	if len(children) == 0 {
		// Leaf node — store exact text.
		pn.text = text
	} else {
		pn.children = make([]patternNode, len(children))
		for i, child := range children {
			pn.children[i] = buildPatternNode(child, source)
		}
	}

	return pn
}

// isMetavarCandidate returns true if the node could be a metavariable.
// Metavariables are identifiers or expression_statements wrapping an identifier
// whose text starts with $.
func isMetavarCandidate(node parser.Node) bool {
	kind := node.Kind()
	return kind == "identifier" || kind == "property_identifier" ||
		kind == "expression_statement" || kind == "statement_identifier"
}

// matchPatterns walks the target tree once and tries all pattern rules at each
// node. It deduplicates by (line, rule) and respects maxMatches per rule.
func matchPatterns(ctx context.Context, patterns []compiledPattern, tree *parser.Tree, source []byte, lineStarts []int) []Diagnostic {
	maxMatches := defaultMaxMatches
	if len(patterns) == 0 {
		return nil
	}

	type seenKey struct {
		line int
		rule string
	}
	seen := make(map[seenKey]struct{})
	counts := make(map[string]int, len(patterns))
	var diags []Diagnostic

	// Walk returns false to skip children but does NOT abort traversal.
	// Use a stopped flag to short-circuit the callback on cancellation.
	stopped := false
	parser.Walk(tree, func(node parser.Node, _ int) bool {
		if stopped {
			return false
		}
		if ctx.Err() != nil {
			stopped = true
			return false
		}

		// Skip ERROR nodes.
		if node.Kind() == "ERROR" {
			return false
		}

		for i := range patterns {
			p := &patterns[i]

			if counts[p.name] >= maxMatches {
				continue
			}

			if !matchNode(&p.root, node, source) {
				continue
			}

			startLine, startCol := offsetToLineCol(lineStarts, int(node.StartByte())) //nolint:gosec // tree-sitter offsets fit in int
			key := seenKey{line: startLine, rule: p.name}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}

			endLine, endCol := offsetToLineCol(lineStarts, int(node.EndByte())) //nolint:gosec // tree-sitter offsets fit in int
			diags = append(diags, Diagnostic{
				Line:     startLine,
				Col:      startCol,
				EndLine:  endLine,
				EndCol:   endCol,
				Rule:     p.name,
				Message:  p.message,
				Severity: p.severity,
			})
			counts[p.name]++
		}

		return true
	})

	return diags
}

// matchNode checks if a patternNode structurally matches a target tree-sitter node.
func matchNode(pn *patternNode, target parser.Node, source []byte) bool {
	switch pn.kind {
	case patternWild, patternVariadic:
		// Wild matches any single node. Variadic also matches any single node
		// at top level — multi-node semantics are handled in matchChildren.
		return true

	case patternLiteral:
		// Check kind match.
		if pn.nodeKind != target.Kind() {
			return false
		}

		// Leaf node: compare exact text.
		if len(pn.children) == 0 {
			return pn.text == target.Text(source)
		}

		// Non-leaf: match children structurally.
		targetChildren := target.CollectChildren()
		return matchChildren(pn.children, targetChildren, source)

	default:
		return false
	}
}

// matchChildren attempts to match pattern children against target children,
// handling variadic metavariables via backtracking. Trailing anonymous nodes
// in the target (like ";") that have no counterpart in the pattern are ignored,
// since pattern authors don't write trailing semicolons.
func matchChildren(patterns []patternNode, targets []parser.Node, source []byte) bool {
	// Strip trailing anonymous punctuation from target that pattern omits
	// (e.g., ";" in the target but not in the pattern source).
	trimmed := targets
	if !endsWithAnonymousLiteral(patterns) {
		for len(trimmed) > 0 && !trimmed[len(trimmed)-1].IsNamed() {
			trimmed = trimmed[:len(trimmed)-1]
		}
	}
	steps := maxBacktrackSteps
	return matchChildrenRec(patterns, trimmed, 0, 0, source, &steps)
}

// endsWithAnonymousLiteral checks if the pattern ends with an anonymous literal node.
func endsWithAnonymousLiteral(patterns []patternNode) bool {
	if len(patterns) == 0 {
		return false
	}
	last := &patterns[len(patterns)-1]
	return last.kind == patternLiteral && !last.isNamed
}

// maxBacktrackSteps bounds recursive backtracking to prevent combinatorial
// explosion with multiple variadics or deeply nested child lists.
const maxBacktrackSteps = 10000

// matchChildrenRec is the recursive backtracking implementation for matching
// pattern children against target children with variadic support.
// steps is decremented on each call; matching fails when exhausted.
func matchChildrenRec(patterns []patternNode, targets []parser.Node, pi, ti int, source []byte, steps *int) bool {
	*steps--
	if *steps <= 0 {
		return false
	}

	// Base case: both exhausted.
	if pi == len(patterns) {
		return ti == len(targets)
	}

	p := &patterns[pi]

	if p.kind == patternVariadic {
		// Try consuming max..0 target children (greedy backtracking).
		remaining := len(targets) - ti
		for consume := remaining; consume >= 0; consume-- {
			if matchChildrenRec(patterns, targets, pi+1, ti+consume, source, steps) {
				return true
			}
		}
		return false
	}

	// Non-variadic: must match exactly one target child.
	if ti >= len(targets) {
		return false
	}

	if !matchNode(p, targets[ti], source) {
		return false
	}

	return matchChildrenRec(patterns, targets, pi+1, ti+1, source, steps)
}
