package config

import (
	"fmt"
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

	for name, rule := range cfg.Rules {
		// Check severity
		switch rule.Severity {
		case SeverityError, SeverityWarn, SeverityOff:
			// valid
		case "":
			errs = append(errs, FieldError{Rule: name, Field: "severity", Message: "required"})
		default:
			errs = append(errs, FieldError{Rule: name, Field: "severity", Message: fmt.Sprintf("invalid value %q (must be error, warn, or off)", rule.Severity)})
		}

		// Check exactly one matcher
		matcherCount := countMatchers(rule)
		if matcherCount == 0 {
			errs = append(errs, FieldError{Rule: name, Field: "matcher", Message: "rule must have exactly one matcher (regex, pattern, ast, imports, or naming)"})
		} else if matcherCount > 1 {
			errs = append(errs, FieldError{Rule: name, Field: "matcher", Message: fmt.Sprintf("rule has %d matchers but must have exactly one", matcherCount)})
		}
	}

	// Validate override globs are non-empty
	for i, o := range cfg.Overrides {
		if len(o.Files) == 0 {
			errs = append(errs, FieldError{Rule: fmt.Sprintf("overrides[%d]", i), Field: "files", Message: "must have at least one file glob"})
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

func countMatchers(r RuleConfig) int {
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
	if r.Naming != nil {
		count++
	}
	return count
}
