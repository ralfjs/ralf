package engine

import (
	"fmt"

	"github.com/BurntSushi/rure-go"

	"github.com/Hideart/ralf/internal/config"
)

// compiledRegex is a pre-compiled regex rule ready for matching.
type compiledRegex struct {
	name     string
	re       *rure.Regex
	message  string
	severity config.Severity
	fix      string // raw fix template from config (empty = no fix)
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

		re, err := rure.Compile(rule.Regex)
		if err != nil {
			errs = append(errs, fmt.Errorf("rule %q: invalid regex: %w", name, err))
			continue
		}

		compiled = append(compiled, compiledRegex{
			name:     name,
			re:       re,
			message:  rule.Message,
			severity: rule.Severity,
			fix:      rule.Fix,
		})
	}

	return compiled, errs
}

// defaultMaxMatches is the upper bound on regex matches per rule per file.
const defaultMaxMatches = 10000

// matchRegex runs a compiled regex against source and returns diagnostics.
// It deduplicates by line (one diagnostic per rule per line) and stops after
// maxMatches. If maxMatches <= 0, defaultMaxMatches is used.
// Caller must hold the CGo semaphore (see LintFile).
func matchRegex(cr compiledRegex, source []byte, lineStarts []int, maxMatches int) []Diagnostic {
	if maxMatches <= 0 {
		maxMatches = defaultMaxMatches
	}

	it := cr.re.IterBytes(source)
	seen := make(map[int]struct{}, len(lineStarts)/4)
	diags := make([]Diagnostic, 0, len(lineStarts)/4)
	count := 0

	for count < maxMatches && it.Next(nil) {
		start, end := it.Match()
		startLine, startCol := offsetToLineCol(lineStarts, start)

		if _, dup := seen[startLine]; dup {
			continue
		}
		seen[startLine] = struct{}{}

		endLine, endCol := offsetToLineCol(lineStarts, end)

		d := Diagnostic{
			Line:     startLine,
			Col:      startCol,
			EndLine:  endLine,
			EndCol:   endCol,
			Rule:     cr.name,
			Message:  cr.message,
			Severity: cr.severity,
		}

		if cr.fix != "" {
			fs, fe := start, end
			newText := cr.fix
			if newText == fixDeleteStatement {
				fs, fe = expandToStatement(source, start, end)
				newText = ""
			}
			d.Fix = &Fix{StartByte: fs, EndByte: fe, NewText: newText}
		}

		diags = append(diags, d)
		count++
	}

	return diags
}
