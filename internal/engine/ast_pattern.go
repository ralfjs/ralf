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
	patternVariadic                        // $$$NAME or $$$ — matches zero or more nodes
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
	fix      string // raw fix template from config (empty = no fix)
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
		fix:      rule.Fix,
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

			// Collect capture bindings when the fix template uses $VAR.
			var captures map[string]string
			if p.fix != "" && strings.Contains(p.fix, "$") {
				captures = make(map[string]string)
			}

			if !matchNode(&p.root, node, source, captures) {
				continue
			}

			d := nodeDiag(node, lineStarts, p.name, p.message, p.severity)
			key := seenKey{line: d.Line, rule: p.name}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}

			if p.fix != "" {
				f := nodeFix(node, source, p.fix)
				if p.fix != fixDeleteStatement && captures != nil {
					f.NewText = substituteCaptures(f.NewText, captures)
				}
				d.Fix = f
			}

			diags = append(diags, d)
			counts[p.name]++
		}

		return true
	})

	return diags
}

// matchNode checks if a patternNode structurally matches a target tree-sitter
// node. When captures is non-nil, metavariable bindings are recorded into it.
func matchNode(pn *patternNode, target parser.Node, source []byte, captures map[string]string) bool {
	switch pn.kind {
	case patternWild, patternVariadic:
		// Wild matches any single node. Variadic also matches any single node
		// at top level — multi-node semantics are handled in matchChildren.
		if captures != nil && pn.name != "" {
			captures[pn.name] = target.Text(source)
		}
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
		return matchChildren(pn.children, targetChildren, source, captures)

	default:
		return false
	}
}

// matchChildren attempts to match pattern children against target children,
// handling variadic metavariables via backtracking. Trailing anonymous nodes
// in the target (like ";") that have no counterpart in the pattern are ignored,
// since pattern authors don't write trailing semicolons.
// When captures is non-nil, metavariable bindings are recorded.
func matchChildren(patterns []patternNode, targets []parser.Node, source []byte, captures map[string]string) bool {
	// Strip trailing anonymous punctuation from target that pattern omits
	// (e.g., ";" in the target but not in the pattern source).
	trimmed := targets
	if !endsWithAnonymousLiteral(patterns) {
		for len(trimmed) > 0 && !trimmed[len(trimmed)-1].IsNamed() {
			trimmed = trimmed[:len(trimmed)-1]
		}
	}
	steps := maxBacktrackSteps
	return matchChildrenRec(patterns, trimmed, 0, 0, source, &steps, captures)
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
// When captures is non-nil, metavariable bindings are recorded and
// saved/restored on backtrack.
func matchChildrenRec(patterns []patternNode, targets []parser.Node, pi, ti int, source []byte, steps *int, captures map[string]string) bool {
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
			// Save and restore captures on backtrack.
			var saved map[string]string
			if captures != nil {
				saved = make(map[string]string, len(captures))
				for k, v := range captures {
					saved[k] = v
				}
				// Record variadic capture as concatenated text of consumed nodes.
				// Zero consumed nodes → empty string (so $$$ARGS doesn't survive
				// into the fix template as a literal token).
				if p.name != "" {
					if consume > 0 {
						start := int(targets[ti].StartByte())       //nolint:gosec // tree-sitter offsets fit in int
						end := int(targets[ti+consume-1].EndByte()) //nolint:gosec // tree-sitter offsets fit in int
						captures[p.name] = string(source[start:end])
					} else {
						captures[p.name] = ""
					}
				}
			}

			if matchChildrenRec(patterns, targets, pi+1, ti+consume, source, steps, captures) {
				return true
			}

			if saved != nil {
				for k := range captures {
					delete(captures, k)
				}
				for k, v := range saved {
					captures[k] = v
				}
			}
		}
		return false
	}

	// Non-variadic: must match exactly one target child.
	if ti >= len(targets) {
		return false
	}

	if !matchNode(p, targets[ti], source, captures) {
		return false
	}

	return matchChildrenRec(patterns, targets, pi+1, ti+1, source, steps, captures)
}

// substituteCaptures replaces $$$NAME and $NAME tokens in a fix template
// with their captured values. Uses a single forward scan to avoid
// re-processing captured text that may itself contain $ characters.
func substituteCaptures(tmpl string, captures map[string]string) string {
	var buf strings.Builder
	buf.Grow(len(tmpl))
	i := 0
	for i < len(tmpl) {
		if tmpl[i] != '$' {
			buf.WriteByte(tmpl[i])
			i++
			continue
		}

		// Try $$$ prefix first (longer match wins).
		if matched, val, advance := matchCapture(tmpl, i, "$$$", captures); matched {
			buf.WriteString(val)
			i += advance
			continue
		}
		if matched, val, advance := matchCapture(tmpl, i, "$", captures); matched {
			buf.WriteString(val)
			i += advance
			continue
		}

		// No capture match — emit the literal $.
		buf.WriteByte('$')
		i++
	}
	return buf.String()
}

// matchCapture checks if tmpl[pos:] starts with prefix followed by a capture
// name. Returns whether it matched, the captured value, and how many bytes to
// advance past.
func matchCapture(tmpl string, pos int, prefix string, captures map[string]string) (matched bool, val string, advance int) {
	if !strings.HasPrefix(tmpl[pos:], prefix) {
		return false, "", 0
	}
	rest := tmpl[pos+len(prefix):]
	// Find the longest matching capture name at this position.
	bestLen := 0
	bestVal := ""
	for name, val := range captures {
		if len(name) > bestLen && strings.HasPrefix(rest, name) {
			bestLen = len(name)
			bestVal = val
		}
	}
	if bestLen == 0 {
		return false, "", 0
	}
	return true, bestVal, len(prefix) + bestLen
}
