package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/BurntSushi/rure-go"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/parser"
)

// ErrStructuralCompile indicates a structural AST rule failed to compile.
var ErrStructuralCompile = errors.New("structural compile failed")

// maxASTMatcherDepth bounds recursive compilation of nested ASTMatcher
// (parent/not chains) to prevent stack overflow from pathological configs.
const maxASTMatcherDepth = 10

// compiledStructural is a pre-compiled structural AST rule ready for matching.
type compiledStructural struct {
	name     string
	matcher  compiledASTMatcher
	naming   *compiledNaming
	message  string
	severity config.Severity
	fix      string
}

// compiledASTMatcher is the compiled form of config.ASTMatcher.
type compiledASTMatcher struct {
	kind   string              // "" = match any kind (used at compile time)
	kindID uint16              // resolved symbol ID (0 = match any, set by resolveSymbolIDs)
	name   *compiledNameMatch  // nil = no name constraint
	parent *compiledASTMatcher // nil = no parent constraint
	not    *compiledASTMatcher // nil = no negation
}

// compiledNameMatch holds either an exact string or a compiled regex for name matching.
type compiledNameMatch struct {
	exact string      // non-empty = exact string match
	re    *rure.Regex // non-nil = regex match
}

// compileStructuralRules extracts rules with AST != nil and Severity != Off,
// compiles each, and returns the compiled set plus any compilation errors.
func compileStructuralRules(rules map[string]config.RuleConfig) ([]compiledStructural, []error) {
	var compiled []compiledStructural
	var errs []error

	for name := range rules {
		rule := rules[name]
		if rule.AST == nil || rule.Severity == config.SeverityOff {
			continue
		}

		m, err := compileASTMatcher(name, rule.AST, 0)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		nm, err := compileNaming(name, rule.Naming)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		compiled = append(compiled, compiledStructural{
			name:     name,
			matcher:  m,
			naming:   nm,
			message:  rule.Message,
			severity: rule.Severity,
			fix:      rule.Fix,
		})
	}

	return compiled, errs
}

// compileASTMatcher recursively compiles a config.ASTMatcher into a
// compiledASTMatcher. depth tracks recursion to enforce maxASTMatcherDepth.
func compileASTMatcher(name string, ast *config.ASTMatcher, depth int) (compiledASTMatcher, error) {
	if depth >= maxASTMatcherDepth {
		return compiledASTMatcher{}, fmt.Errorf("rule %q: %w: matcher nesting exceeds max depth %d", name, ErrStructuralCompile, maxASTMatcherDepth)
	}

	var m compiledASTMatcher
	m.kind = ast.Kind

	// Compile name constraint.
	if ast.Name != nil {
		nm, err := compileNameMatch(name, ast.Name)
		if err != nil {
			return compiledASTMatcher{}, err
		}
		m.name = &nm
	}

	// Compile parent constraint.
	if ast.Parent != nil {
		p, err := compileASTMatcher(name, ast.Parent, depth+1)
		if err != nil {
			return compiledASTMatcher{}, err
		}
		m.parent = &p
	}

	// Compile not constraint.
	if ast.Not != nil {
		n, err := compileASTMatcher(name, ast.Not, depth+1)
		if err != nil {
			return compiledASTMatcher{}, err
		}
		m.not = &n
	}

	return m, nil
}

// resolveSymbolIDs resolves kind strings to numeric symbol IDs using the
// tree-sitter grammar. This must be called before matching and requires
// a parsed tree (to access the language grammar).
func resolveSymbolIDs(root parser.Node, rules []compiledStructural) {
	for i := range rules {
		resolveMatcherSymbolIDs(root, &rules[i].matcher)
	}
}

func resolveMatcherSymbolIDs(root parser.Node, m *compiledASTMatcher) {
	if m.kind != "" {
		m.kindID = root.SymbolForKind(m.kind, true)
	}
	if m.parent != nil {
		resolveMatcherSymbolIDs(root, m.parent)
	}
	if m.not != nil {
		resolveMatcherSymbolIDs(root, m.not)
	}
}

// compileNameMatch compiles the Name field from config into a compiledNameMatch.
// Strings wrapped in "/" are compiled as rure regex; plain strings are exact match.
func compileNameMatch(ruleName string, nameVal interface{}) (compiledNameMatch, error) {
	s, ok := nameVal.(string)
	if !ok {
		return compiledNameMatch{}, fmt.Errorf("rule %q: %w: name must be a string, got %T", ruleName, ErrStructuralCompile, nameVal)
	}

	if strings.HasPrefix(s, "/") && strings.HasSuffix(s, "/") && len(s) > 1 {
		pattern := s[1 : len(s)-1]
		re, err := rure.Compile(pattern)
		if err != nil {
			return compiledNameMatch{}, fmt.Errorf("rule %q: %w: invalid name regex %q: %w", ruleName, ErrStructuralCompile, pattern, err)
		}
		return compiledNameMatch{re: re}, nil
	}

	return compiledNameMatch{exact: s}, nil
}

// rulesNeedName reports whether any rule (or its nested not matcher) has a
// name constraint, which requires the more expensive ChildByFieldID lookup.
func rulesNeedName(rules []compiledStructural) bool {
	for i := range rules {
		if rules[i].naming != nil {
			return true
		}
		if matcherNeedsName(&rules[i].matcher) {
			return true
		}
	}
	return false
}

func matcherNeedsName(m *compiledASTMatcher) bool {
	if m.name != nil {
		return true
	}
	if m.not != nil && matcherNeedsName(m.not) {
		return true
	}
	if m.parent != nil && matcherNeedsName(m.parent) {
		return true
	}
	return false
}

// structuralIndex groups rules by their top-level kindID for fast lookup.
// Rules with kindID == 0 (match any) are stored in the wildcard slice.
type structuralIndex struct {
	byKindID map[uint16][]*compiledStructural
	wildcard []*compiledStructural
}

func buildStructuralIndex(rules []compiledStructural) structuralIndex {
	idx := structuralIndex{
		byKindID: make(map[uint16][]*compiledStructural, len(rules)),
	}
	for i := range rules {
		r := &rules[i]
		if r.matcher.kindID == 0 && r.matcher.kind == "" {
			idx.wildcard = append(idx.wildcard, r)
		} else {
			idx.byKindID[r.matcher.kindID] = append(idx.byKindID[r.matcher.kindID], r)
		}
	}
	return idx
}

// candidates returns rules that could match a node with the given kindID.
func (idx *structuralIndex) candidates(kindID uint16) []*compiledStructural {
	specific := idx.byKindID[kindID]
	if len(idx.wildcard) == 0 {
		return specific
	}
	if len(specific) == 0 {
		return idx.wildcard
	}
	merged := make([]*compiledStructural, 0, len(specific)+len(idx.wildcard))
	merged = append(merged, specific...)
	merged = append(merged, idx.wildcard...)
	return merged
}

// matchStructural walks the AST once and tries all structural rules at each
// named node. It deduplicates by (line, rule) and respects maxMatches.
//
// Uses numeric symbol IDs (KindID) instead of string comparisons in the hot
// loop to avoid C.GoString allocations from node.Kind().
func matchStructural(ctx context.Context, rules []compiledStructural, tree *parser.Tree, source []byte, lineStarts []int) []Diagnostic {
	maxMatches := defaultMaxMatches
	if len(rules) == 0 {
		return nil
	}

	root := tree.RootNode()
	if root.IsNull() {
		return nil
	}

	// Resolve kind strings → symbol IDs and the ERROR symbol ID once.
	resolveSymbolIDs(root, rules)
	errorSymID := root.SymbolForKind("ERROR", true)

	// Index rules by kindID for O(1) lookup per node.
	idx := buildStructuralIndex(rules)

	// Resolve the "name" field ID once if any rule needs name matching.
	var nameFieldID uint16
	if rulesNeedName(rules) {
		nameFieldID = root.FieldID("name")
	}

	type seenKey struct {
		line int
		rule string
	}
	seen := make(map[seenKey]struct{})
	counts := make(map[string]int, len(rules))
	var diags []Diagnostic

	stopped := false
	parser.WalkNamed(tree, func(node parser.Node, _ int) bool {
		if stopped {
			return false
		}
		if ctx.Err() != nil {
			stopped = true
			return false
		}

		// KindID is a uint16 from C — no allocation, no string copy.
		kindID := node.KindID()
		if kindID == errorSymID {
			return false
		}

		cands := idx.candidates(kindID)
		if len(cands) == 0 {
			return true
		}

		for _, r := range cands {
			if counts[r.name] >= maxMatches {
				continue
			}

			if !matchASTNode(&r.matcher, node, kindID, source, nameFieldID) {
				continue
			}

			// Naming convention check: extract name, test regex.
			// If name conforms → no violation, skip.
			msg := r.message
			if r.naming != nil {
				name := extractNodeNameByID(node, source, nameFieldID)
				if r.naming.matches(name) {
					continue
				}
				if r.naming.message != "" {
					msg = r.naming.message
				}
			}

			d := nodeDiag(node, lineStarts, r.name, msg, r.severity)
			key := seenKey{line: d.Line, rule: r.name}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}

			if r.fix != "" {
				d.Fix = nodeFix(node, source, r.fix)
			}

			diags = append(diags, d)
			counts[r.name]++
		}

		return true
	})

	return diags
}

// matchASTNode checks if a compiledASTMatcher matches the given node.
// nodeKindID is the pre-fetched node.KindID() — no allocation.
// nameFieldID is the cached field ID for "name" (0 if unused).
func matchASTNode(m *compiledASTMatcher, node parser.Node, nodeKindID uint16, source []byte, nameFieldID uint16) bool {
	if node.IsNull() {
		return false
	}

	// Kind constraint — compare numeric IDs, no string allocation.
	if m.kindID != 0 && m.kindID != nodeKindID {
		return false
	}

	// Name constraint.
	if m.name != nil {
		nameText := extractNodeNameByID(node, source, nameFieldID)
		if !matchName(m.name, nameText) {
			return false
		}
	}

	// Parent constraint.
	if m.parent != nil {
		parent := node.Parent()
		var parentKindID uint16
		if !parent.IsNull() {
			parentKindID = parent.KindID()
		}
		if !matchASTNode(m.parent, parent, parentKindID, source, nameFieldID) {
			return false
		}
	}

	// Not constraint — reuse nodeKindID since we're matching the same node.
	if m.not != nil {
		if matchASTNode(m.not, node, nodeKindID, source, nameFieldID) {
			return false
		}
	}

	return true
}

// extractNodeNameByID extracts the "name" text from a node using a cached
// field ID. Falls back to ChildByFieldName when the ID is not available,
// and to the full node text if no "name" field exists.
func extractNodeNameByID(node parser.Node, source []byte, nameFieldID uint16) string {
	if nameFieldID != 0 {
		nameChild := node.ChildByFieldID(nameFieldID)
		if !nameChild.IsNull() {
			return nameChild.Text(source)
		}
	} else {
		nameChild := node.ChildByFieldName("name")
		if !nameChild.IsNull() {
			return nameChild.Text(source)
		}
	}
	return node.Text(source)
}

// extractNodeName extracts the "name" text from a node. Uses the tree-sitter
// "name" field if present, otherwise falls back to the full node text.
func extractNodeName(node parser.Node, source []byte) string {
	nameChild := node.ChildByFieldName("name")
	if !nameChild.IsNull() {
		return nameChild.Text(source)
	}
	return node.Text(source)
}

// matchName checks if text matches a compiled name constraint.
func matchName(nm *compiledNameMatch, text string) bool {
	if nm.re != nil {
		return nm.re.IsMatch(text)
	}
	return nm.exact == text
}
