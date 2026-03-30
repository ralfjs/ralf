package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FieldError describes a single validation problem.
type FieldError struct {
	Rule    string
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("rule %q: %s: %s", e.Rule, e.Field, e.Message)
}

// ValidationError is returned when a config fails structural validation.
type ValidationError struct {
	Errors []FieldError
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		msgs[i] = fe.Error()
	}
	return fmt.Sprintf("config validation failed:\n  %s", strings.Join(msgs, "\n  "))
}

// Validate checks structural correctness of cfg. Each rule must have exactly
// one matcher and a valid severity.
func Validate(cfg *Config) error {
	var errs []FieldError

	for name := range cfg.Rules {
		rule := cfg.Rules[name]
		validateRule(name, &rule, &errs)
	}

	// Validate overrides
	for i := range cfg.Overrides {
		o := &cfg.Overrides[i]
		prefix := fmt.Sprintf("overrides[%d]", i)

		// Check file globs
		if len(o.Files) == 0 {
			errs = append(errs, FieldError{Rule: prefix, Field: "files", Message: "must have at least one file glob"})
		}
		for j, glob := range o.Files {
			if strings.TrimSpace(glob) == "" {
				errs = append(errs, FieldError{Rule: prefix, Field: fmt.Sprintf("files[%d]", j), Message: "glob must not be empty"})
			} else if _, err := filepath.Match(glob, ""); err != nil {
				errs = append(errs, FieldError{Rule: prefix, Field: fmt.Sprintf("files[%d]", j), Message: fmt.Sprintf("invalid glob syntax: %v", err)})
			}
		}

		// Validate override rules
		for name := range o.Rules {
			rule := o.Rules[name]
			validateRule(fmt.Sprintf("%s.rules.%s", prefix, name), &rule, &errs)
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

func validateRule(name string, rule *RuleConfig, errs *[]FieldError) {
	switch rule.Severity {
	case SeverityError, SeverityWarn, SeverityOff:
		// valid
	case "":
		*errs = append(*errs, FieldError{Rule: name, Field: "severity", Message: "required"})
	default:
		*errs = append(*errs, FieldError{Rule: name, Field: "severity", Message: fmt.Sprintf("invalid value %q (must be error, warn, or off)", rule.Severity)})
	}

	// Validate scope field.
	if rule.Scope != "" && rule.Scope != "cross-file" {
		*errs = append(*errs, FieldError{Rule: name, Field: "scope", Message: fmt.Sprintf("invalid scope %q (must be empty or \"cross-file\")", rule.Scope)})
	}

	// Cross-file rules must use builtin matcher only.
	if rule.Scope == "cross-file" {
		if !rule.Builtin {
			*errs = append(*errs, FieldError{Rule: name, Field: "scope", Message: "scope \"cross-file\" requires builtin: true"})
		}
		if rule.Regex != "" || rule.Pattern != "" || rule.AST != nil || rule.Imports != nil {
			*errs = append(*errs, FieldError{Rule: name, Field: "scope", Message: "scope \"cross-file\" cannot have regex, pattern, ast, or imports matchers"})
		}
		return // skip normal matcher validation for cross-file rules
	}

	matcherCount := countMatchers(rule)
	if matcherCount == 0 {
		*errs = append(*errs, FieldError{Rule: name, Field: "matcher", Message: "rule must have exactly one matcher (regex, pattern, ast, imports, or builtin)"})
	} else if matcherCount > 1 {
		*errs = append(*errs, FieldError{Rule: name, Field: "matcher", Message: fmt.Sprintf("rule has %d matchers but must have exactly one", matcherCount)})
	}

	// Imports must have at least one group, and each group must be non-empty.
	if rule.Imports != nil {
		if len(rule.Imports.Groups) == 0 {
			*errs = append(*errs, FieldError{Rule: name, Field: "imports.groups", Message: "imports.groups must not be empty"})
		}
		seen := make(map[string]bool, len(rule.Imports.Groups))
		for i, g := range rule.Imports.Groups {
			field := fmt.Sprintf("imports.groups[%d]", i)
			trimmed := strings.TrimSpace(g)
			switch {
			case trimmed == "":
				*errs = append(*errs, FieldError{Rule: name, Field: field, Message: "group name must not be empty"})
			case g != trimmed:
				*errs = append(*errs, FieldError{Rule: name, Field: field, Message: fmt.Sprintf("group name %q has leading/trailing whitespace", g)})
			case seen[g]:
				*errs = append(*errs, FieldError{Rule: name, Field: field, Message: fmt.Sprintf("duplicate group %q", g)})
			}
			seen[trimmed] = true
		}
	}

	// Naming is a modifier on AST, not a standalone matcher.
	if rule.Naming != nil {
		if rule.AST == nil {
			*errs = append(*errs, FieldError{Rule: name, Field: "naming", Message: "naming requires ast matcher"})
		}
		if rule.Naming.Match == "" {
			*errs = append(*errs, FieldError{Rule: name, Field: "naming.match", Message: "naming.match is required"})
		}
		if rule.Regex != "" || rule.Pattern != "" || rule.Imports != nil {
			*errs = append(*errs, FieldError{Rule: name, Field: "naming", Message: "naming can only be combined with ast"})
		}
	}
}

func countMatchers(r *RuleConfig) int {
	count := 0
	if r.Regex != "" {
		count++
	}
	if r.Pattern != "" {
		count++
	}
	if r.AST != nil {
		count++
	}
	if r.Imports != nil {
		count++
	}
	if r.Builtin {
		count++
	}
	return count
}
