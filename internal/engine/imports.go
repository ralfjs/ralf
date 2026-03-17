package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/parser"
)

// ErrImportCompile indicates an import rule failed to compile.
var ErrImportCompile = errors.New("import compile failed")

// importGroup classifies an import statement by its source path.
type importGroup int

const (
	groupBuiltin  importGroup = iota
	groupExternal             // no relative prefix, not builtin
	groupInternal             // @/ or ~/ prefix
	groupParent               // ../ prefix
	groupSibling              // ./ prefix (not index)
	groupIndex                // "." or "./index" or "./index.*"
	groupType                 // import type (TS only)
)

// compiledImport is a pre-compiled import ordering rule ready for matching.
type compiledImport struct {
	name     string
	groups   []importGroup
	alpha    bool
	newline  bool
	message  string
	severity config.Severity
}

// importInfo describes a single import statement extracted from the AST.
type importInfo struct {
	source  string      // module path (quotes stripped)
	group   importGroup // classified group
	line    int         // 1-based start line
	col     int         // 0-based byte offset within line
	endLine int         // 1-based end line
	endCol  int         // 0-based byte offset within end line
}

// nodeBuiltins is the set of Node.js built-in modules.
var nodeBuiltins = map[string]struct{}{
	"assert":              {},
	"assert/strict":       {},
	"async_hooks":         {},
	"buffer":              {},
	"child_process":       {},
	"cluster":             {},
	"console":             {},
	"constants":           {},
	"crypto":              {},
	"dgram":               {},
	"diagnostics_channel": {},
	"dns":                 {},
	"dns/promises":        {},
	"domain":              {},
	"events":              {},
	"fs":                  {},
	"fs/promises":         {},
	"http":                {},
	"http2":               {},
	"https":               {},
	"inspector":           {},
	"module":              {},
	"net":                 {},
	"os":                  {},
	"path":                {},
	"path/posix":          {},
	"path/win32":          {},
	"perf_hooks":          {},
	"process":             {},
	"punycode":            {},
	"querystring":         {},
	"readline":            {},
	"readline/promises":   {},
	"repl":                {},
	"stream":              {},
	"stream/consumers":    {},
	"stream/promises":     {},
	"stream/web":          {},
	"string_decoder":      {},
	"sys":                 {},
	"test":                {},
	"timers":              {},
	"timers/promises":     {},
	"tls":                 {},
	"trace_events":        {},
	"tty":                 {},
	"url":                 {},
	"util":                {},
	"util/types":          {},
	"v8":                  {},
	"vm":                  {},
	"wasi":                {},
	"worker_threads":      {},
	"zlib":                {},
}

// groupNames maps config strings to importGroup values.
var groupNames = map[string]importGroup{
	"builtin":  groupBuiltin,
	"external": groupExternal,
	"internal": groupInternal,
	"parent":   groupParent,
	"sibling":  groupSibling,
	"index":    groupIndex,
	"type":     groupType,
}

// isNodeBuiltin reports whether path is a Node.js built-in module.
func isNodeBuiltin(path string) bool {
	if strings.HasPrefix(path, "node:") {
		return true
	}
	_, ok := nodeBuiltins[path]
	return ok
}

// classifyImport determines the importGroup for a given module path.
func classifyImport(source string, isType bool) importGroup {
	if isType {
		return groupType
	}
	if isNodeBuiltin(source) {
		return groupBuiltin
	}
	if strings.HasPrefix(source, "@/") || strings.HasPrefix(source, "~/") {
		return groupInternal
	}
	if strings.HasPrefix(source, "../") {
		return groupParent
	}
	if source == "." || source == "./index" || (strings.HasPrefix(source, "./index.") && !strings.Contains(source[len("./index."):], "/")) {
		return groupIndex
	}
	if strings.HasPrefix(source, "./") {
		return groupSibling
	}
	// No relative prefix and not builtin → external.
	return groupExternal
}

// stripQuotes removes matching surrounding quotes from a string literal.
// Only strips when both ends are the same quote character.
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

// detectTypeImport checks if an import_statement node is a type-only import.
// In the TS grammar, child at index 1 has kind "type" (unnamed).
// In the JS grammar, child at index 1 is an ERROR node.
func detectTypeImport(node parser.Node) bool {
	if node.ChildCount() < 2 {
		return false
	}
	child := node.Child(1)
	if child.IsNull() {
		return false
	}
	return child.Kind() == "type" && !child.IsNamed()
}

// compileImportRules extracts rules with Imports != nil and Severity != Off,
// compiles each, and returns the compiled set plus any compilation errors.
func compileImportRules(rules map[string]config.RuleConfig) ([]compiledImport, []error) {
	var compiled []compiledImport
	var errs []error

	for name := range rules {
		rule := rules[name]
		if rule.Imports == nil || rule.Severity == config.SeverityOff {
			continue
		}

		groups := make([]importGroup, 0, len(rule.Imports.Groups))
		for _, gs := range rule.Imports.Groups {
			g, ok := groupNames[gs]
			if !ok {
				errs = append(errs, fmt.Errorf("rule %q: %w: unknown group %q", name, ErrImportCompile, gs))
				continue
			}
			groups = append(groups, g)
		}
		if len(groups) != len(rule.Imports.Groups) {
			continue // had errors, skip this rule
		}

		compiled = append(compiled, compiledImport{
			name:     name,
			groups:   groups,
			alpha:    rule.Imports.Alphabetize,
			newline:  rule.Imports.NewlineBetween,
			message:  rule.Message,
			severity: rule.Severity,
		})
	}

	return compiled, errs
}

// resolveImportRules filters import rules that are active for the given file.
func resolveImportRules(rules []compiledImport, effective map[string]config.RuleConfig, filePath string) []compiledImport {
	active := make([]compiledImport, 0, len(rules))
	for i := range rules {
		ci := &rules[i]
		sev, msg, ok := resolveRule(effective, ci.name, ci.message, filePath)
		if !ok {
			continue
		}
		resolved := *ci
		resolved.severity = sev
		resolved.message = msg
		active = append(active, resolved)
	}
	return active
}

// collectImports iterates top-level children and extracts import statements.
func collectImports(root parser.Node, source []byte, lineStarts []int, importSymID uint16) []importInfo {
	count := root.NamedChildCount()
	var imports []importInfo

	for i := range count {
		child := root.NamedChild(i)
		if child.KindID() != importSymID {
			continue
		}

		srcNode := child.ChildByFieldName("source")
		if srcNode.IsNull() {
			continue
		}

		rawSource := srcNode.Text(source)
		modPath := stripQuotes(rawSource)
		isType := detectTypeImport(child)

		startLine, startCol := offsetToLineCol(lineStarts, int(child.StartByte())) //nolint:gosec // tree-sitter offsets fit in int
		endLine, endCol := offsetToLineCol(lineStarts, int(child.EndByte()))       //nolint:gosec // tree-sitter offsets fit in int

		imports = append(imports, importInfo{
			source:  modPath,
			group:   classifyImport(modPath, isType),
			line:    startLine,
			col:     startCol,
			endLine: endLine,
			endCol:  endCol,
		})
	}

	return imports
}

// groupIdx returns the position of g in the rule's groups slice, or -1 if
// the group is not listed.
func groupIdx(groups []importGroup, g importGroup) int {
	for i, gg := range groups {
		if gg == g {
			return i
		}
	}
	return -1
}

// matchImports runs all three import checks (group order, alphabetize,
// newline between) and returns diagnostics.
func matchImports(ctx context.Context, rules []compiledImport, tree *parser.Tree, source []byte, lineStarts []int) []Diagnostic {
	if len(rules) == 0 {
		return nil
	}

	root := tree.RootNode()
	if root.IsNull() {
		return nil
	}

	importSymID := root.SymbolForKind("import_statement", true)
	if importSymID == 0 {
		return nil
	}

	imports := collectImports(root, source, lineStarts, importSymID)
	if len(imports) == 0 {
		return nil
	}

	var diags []Diagnostic

	for i := range rules {
		if ctx.Err() != nil {
			break
		}
		found := checkImportRule(&rules[i], imports, source, lineStarts)
		diags = append(diags, found...)
	}

	return diags
}

// checkImportRule runs group ordering, alphabetize, and newline-between checks
// in a single pass over the imports slice.
func checkImportRule(rule *compiledImport, imports []importInfo, source []byte, lineStarts []int) []Diagnostic {
	var diags []Diagnostic

	maxGroupIdx := -1  // highest group index seen so far (for ordering check)
	prevGroupIdx := -2 // previous import's group index (-2 = sentinel: no previous)
	prevSource := ""   // previous import's source (for alphabetize check)
	prevEndLine := 0   // previous import's end line (for newline check)

	for _, imp := range imports {
		idx := groupIdx(rule.groups, imp.group)
		if idx < 0 {
			// Group not in the rule's list — reset run tracking, skip checks.
			prevGroupIdx = -2
			continue
		}

		// Check 1: Group ordering — current group should not precede the
		// highest group index seen so far.
		if idx < maxGroupIdx {
			diags = append(diags, importDiag(imp, rule.name, rule.message,
				fmt.Sprintf("import %q should appear before imports from a later group", imp.source),
				rule.severity))
		}
		if idx > maxGroupIdx {
			maxGroupIdx = idx
		}

		if prevGroupIdx >= 0 {
			// Check 2: Alphabetize within same group.
			if rule.alpha && idx == prevGroupIdx {
				if strings.ToLower(imp.source) < strings.ToLower(prevSource) {
					diags = append(diags, importDiag(imp, rule.name, rule.message,
						fmt.Sprintf("import %q should appear before %q (alphabetical order)", imp.source, prevSource),
						rule.severity))
				}
			}

			// Check 3: Newline between groups — count actual empty lines
			// in source between previous import's end and current import's start.
			if rule.newline {
				emptyCount := countEmptyLinesBetween(source, lineStarts, prevEndLine, imp.line)
				if idx != prevGroupIdx && emptyCount < 1 {
					diags = append(diags, importDiag(imp, rule.name, rule.message,
						fmt.Sprintf("missing blank line before import %q (different group)", imp.source),
						rule.severity))
				} else if idx == prevGroupIdx && emptyCount > 0 {
					diags = append(diags, importDiag(imp, rule.name, rule.message,
						fmt.Sprintf("unexpected blank line before import %q (same group)", imp.source),
						rule.severity))
				}
			}
		}

		prevSource = imp.source
		prevGroupIdx = idx
		prevEndLine = imp.endLine
	}

	return diags
}

// countEmptyLinesBetween counts whitespace-only lines between two 1-based line
// numbers (exclusive on both ends). Lines containing only spaces/tabs count as
// empty; lines with comments or other content do not.
func countEmptyLinesBetween(source []byte, lineStarts []int, fromLine, toLine int) int {
	count := 0
	for line := fromLine + 1; line < toLine; line++ {
		if isEmptyLine(source, lineStarts, line) {
			count++
		}
	}
	return count
}

// isEmptyLine reports whether the given 1-based line contains only whitespace.
func isEmptyLine(source []byte, lineStarts []int, line int) bool {
	idx := line - 1 // 0-based index into lineStarts
	if idx < 0 || idx >= len(lineStarts) {
		return false
	}
	start := lineStarts[idx]
	end := len(source)
	if idx+1 < len(lineStarts) {
		end = lineStarts[idx+1]
	}
	for i := start; i < end; i++ {
		b := source[i]
		if b != ' ' && b != '\t' && b != '\r' && b != '\n' {
			return false
		}
	}
	return true
}

// importDiag builds a Diagnostic from an importInfo. Uses ruleMsg if non-empty,
// otherwise falls back to fallback.
func importDiag(imp importInfo, rule, ruleMsg, fallback string, severity config.Severity) Diagnostic {
	msg := ruleMsg
	if msg == "" {
		msg = fallback
	}
	return Diagnostic{
		Line:     imp.line,
		Col:      imp.col,
		EndLine:  imp.endLine,
		EndCol:   imp.endCol,
		Rule:     rule,
		Message:  msg,
		Severity: severity,
	}
}
