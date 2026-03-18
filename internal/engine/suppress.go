package engine

import (
	"bytes"
	"regexp"
	"strings"
)

// suppressions holds all inline suppression directives parsed from source comments.
type suppressions struct {
	file   map[string]bool         // rules disabled for entire file ("" = all)
	lines  map[int]map[string]bool // line → rules suppressed ("" = all)
	blocks []blockRange            // disable/enable ranges
}

// blockRange represents a lint-disable/lint-enable region.
type blockRange struct {
	startLine int    // 1-based, inclusive
	endLine   int    // 1-based, inclusive (0 = EOF, resolved after scan)
	rule      string // "" = all rules
}

// suppressRe matches all four suppression directive keywords in both // and /* */ comments.
// Alternation order matters: longer prefixes first so "disable-next-line" and
// "disable-file" match before bare "disable".
var suppressRe = regexp.MustCompile(
	`(?://|/\*)\s*lint-(disable-next-line|disable-file|disable|enable)\s*([\w\s,\-]*?)(?:\s*\*/|\s*$)`,
)

// parseRuleList splits a comma-separated rule list from a suppression comment.
// An empty or whitespace-only string returns [""] meaning "all rules".
func parseRuleList(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []string{""}
	}
	parts := strings.Split(trimmed, ",")
	rules := make([]string, 0, len(parts))
	for _, p := range parts {
		r := strings.TrimSpace(p)
		if r != "" {
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 {
		return []string{""}
	}
	return rules
}

// addLineSuppressions records rules as suppressed on the given line number.
func addLineSuppressions(lines map[int]map[string]bool, lineNum int, rules []string) {
	if lines[lineNum] == nil {
		lines[lineNum] = make(map[string]bool, len(rules))
	}
	for _, r := range rules {
		lines[lineNum][r] = true
	}
}

// emptySup is the zero-value returned when no suppression directives are present.
var emptySup = suppressions{
	file:  map[string]bool{},
	lines: map[int]map[string]bool{},
}

// parseSuppressComments scans source line-by-line and extracts all suppression directives.
func parseSuppressComments(source []byte) suppressions {
	// Fast path: skip the full parse when source contains no directives.
	if !bytes.Contains(source, []byte("lint-")) {
		return emptySup
	}

	sup := suppressions{
		file:  make(map[string]bool),
		lines: make(map[int]map[string]bool),
	}

	lines := strings.Split(string(source), "\n")
	totalLines := len(lines)

	// openBlocks tracks unmatched lint-disable directives per rule for block suppression.
	// Key is rule name ("" = all), value is stack of start lines.
	openBlocks := make(map[string][]int)

	for i, line := range lines {
		lineNum := i + 1 // 1-based

		loc := suppressRe.FindStringSubmatchIndex(line)
		if loc == nil {
			continue
		}

		directive := line[loc[2]:loc[3]]
		ruleStr := ""
		if loc[4] >= 0 && loc[5] >= 0 {
			ruleStr = line[loc[4]:loc[5]]
		}
		rules := parseRuleList(ruleStr)

		switch directive {
		case "disable-next-line":
			addLineSuppressions(sup.lines, lineNum+1, rules)

		case "disable-file":
			for _, r := range rules {
				sup.file[r] = true
			}

		case "disable":
			// Same-line if there's non-whitespace content before the comment.
			beforeComment := strings.TrimSpace(line[:loc[0]])
			if beforeComment != "" {
				addLineSuppressions(sup.lines, lineNum, rules)
			} else {
				// Block start.
				for _, r := range rules {
					openBlocks[r] = append(openBlocks[r], lineNum)
				}
			}

		case "enable":
			// Bare "// lint-enable" (no rules) closes ALL open blocks.
			if len(rules) == 1 && rules[0] == "" {
				for r, stack := range openBlocks {
					for _, startLine := range stack {
						sup.blocks = append(sup.blocks, blockRange{
							startLine: startLine,
							endLine:   lineNum,
							rule:      r,
						})
					}
					delete(openBlocks, r)
				}
			} else {
				for _, r := range rules {
					stack := openBlocks[r]
					if len(stack) == 0 {
						continue // no matching disable — silently ignore
					}
					startLine := stack[len(stack)-1]
					openBlocks[r] = stack[:len(stack)-1]
					sup.blocks = append(sup.blocks, blockRange{
						startLine: startLine,
						endLine:   lineNum,
						rule:      r,
					})
				}
			}
		}
	}

	// Close any unclosed blocks at EOF.
	for r, stack := range openBlocks {
		for _, startLine := range stack {
			sup.blocks = append(sup.blocks, blockRange{
				startLine: startLine,
				endLine:   totalLines,
				rule:      r,
			})
		}
	}

	return sup
}

// isSuppressed checks whether a diagnostic at the given line for the given rule
// should be suppressed.
func isSuppressed(sup *suppressions, line int, rule string) bool {
	// File-level: all rules or specific rule.
	if sup.file[""] || sup.file[rule] {
		return true
	}

	// Line-level: all rules or specific rule.
	if m := sup.lines[line]; m != nil {
		if m[""] || m[rule] {
			return true
		}
	}

	// Block ranges.
	for i := range sup.blocks {
		b := &sup.blocks[i]
		if line >= b.startLine && line <= b.endLine {
			if b.rule == "" || b.rule == rule {
				return true
			}
		}
	}

	return false
}

// filterSuppressed removes diagnostics that are suppressed by inline comments.
// It filters in-place, reusing the backing array.
func filterSuppressed(diags []Diagnostic, sup suppressions) []Diagnostic {
	n := 0
	for i := range diags {
		if !isSuppressed(&sup, diags[i].Line, diags[i].Rule) {
			diags[n] = diags[i]
			n++
		}
	}
	return diags[:n]
}
