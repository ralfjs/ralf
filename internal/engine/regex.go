package engine

import (
	"fmt"
	"regexp"

	"github.com/Hideart/bepro/internal/config"
)

// compiledRegex is a pre-compiled regex rule ready for matching.
type compiledRegex struct {
	name     string
	re       *regexp.Regexp
	message  string
	severity config.Severity
	where    *config.WherePredicate
}

// compileRegexRules extracts rules with Regex != "" and Severity != Off,
// compiles each, and returns the compiled set plus any compilation errors.
// It does not fail fast — all bad patterns are collected.
func compileRegexRules(rules map[string]config.RuleConfig) ([]compiledRegex, []error) {
	var compiled []compiledRegex
	var errs []error

	for name := range rules {
		rule := rules[name]
		if rule.Regex == "" || rule.Severity == config.SeverityOff {
			continue
		}

		re, err := regexp.Compile(rule.Regex)
		if err != nil {
			errs = append(errs, fmt.Errorf("rule %q: invalid regex: %w", name, err))
			continue
		}

		compiled = append(compiled, compiledRegex{
			name:     name,
			re:       re,
			message:  rule.Message,
			severity: rule.Severity,
			where:    rule.Where,
		})
	}

	return compiled, errs
}

// defaultMaxMatches is the upper bound on regex matches per rule per file.
const defaultMaxMatches = 10000

// matchRegex runs a compiled regex against source and returns diagnostics.
// It deduplicates by line (one diagnostic per rule per line) and stops after
// maxMatches. If maxMatches <= 0, defaultMaxMatches is used.
func matchRegex(cr compiledRegex, source []byte, lineStarts []int, maxMatches int) []Diagnostic {
	if maxMatches <= 0 {
		maxMatches = defaultMaxMatches
	}

	matches := cr.re.FindAllIndex(source, maxMatches)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[int]struct{})
	var diags []Diagnostic

	for _, m := range matches {
		startLine, startCol := offsetToLineCol(lineStarts, m[0])

		if _, dup := seen[startLine]; dup {
			continue
		}
		seen[startLine] = struct{}{}

		endLine, endCol := offsetToLineCol(lineStarts, m[1])

		diags = append(diags, Diagnostic{
			Line:     startLine,
			Col:      startCol,
			EndLine:  endLine,
			EndCol:   endCol,
			Rule:     cr.name,
			Message:  cr.message,
			Severity: cr.severity,
		})
	}

	return diags
}
