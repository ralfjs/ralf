// Package config provides types and loading for linter configuration files.
package config

// Config is the top-level configuration structure.
// NOTE: when adding fields, update mergeInto in extends.go.
type Config struct {
	Rules     map[string]RuleConfig `json:"rules" yaml:"rules" toml:"rules"`
	Ignores   []string              `json:"ignores,omitempty" yaml:"ignores,omitempty" toml:"ignores,omitempty"`
	Extends   []string              `json:"extends,omitempty" yaml:"extends,omitempty" toml:"extends,omitempty"`
	Overrides []Override            `json:"overrides,omitempty" yaml:"overrides,omitempty" toml:"overrides,omitempty"`
}

// RuleConfig defines a single lint rule. Exactly one matcher field (Regex,
// Pattern, AST, Imports, or Builtin) must be set.
type RuleConfig struct {
	Severity Severity        `json:"severity" yaml:"severity" toml:"severity"`
	Message  string          `json:"message,omitempty" yaml:"message,omitempty" toml:"message,omitempty"`
	Regex    string          `json:"regex,omitempty" yaml:"regex,omitempty" toml:"regex,omitempty"`
	Pattern  string          `json:"pattern,omitempty" yaml:"pattern,omitempty" toml:"pattern,omitempty"`
	AST      *ASTMatcher     `json:"ast,omitempty" yaml:"ast,omitempty" toml:"ast,omitempty"`
	Imports  *ImportsMatcher `json:"imports,omitempty" yaml:"imports,omitempty" toml:"imports,omitempty"`
	Builtin  bool            `json:"builtin,omitempty" yaml:"builtin,omitempty" toml:"builtin,omitempty"` // custom Go checker
	Naming   *NamingMatcher  `json:"naming,omitempty" yaml:"naming,omitempty" toml:"naming,omitempty"`
	Where    *WherePredicate `json:"where,omitempty" yaml:"where,omitempty" toml:"where,omitempty"`
	Fix      string          `json:"fix,omitempty" yaml:"fix,omitempty" toml:"fix,omitempty"`
}

// Severity controls diagnostic output level.
type Severity string

// Valid severity levels.
const (
	SeverityError Severity = "error"
	SeverityWarn  Severity = "warn"
	SeverityOff   Severity = "off"
)

// Override applies rule overrides to files matching the given globs.
type Override struct {
	Files []string              `json:"files" yaml:"files" toml:"files"`
	Rules map[string]RuleConfig `json:"rules" yaml:"rules" toml:"rules"`
}

// ASTMatcher describes a structural AST query for matching nodes.
// Supported fields: kind, name, parent, not. Additional query primitives
// (children, ancestor, hasChild, etc.) are planned for future phases.
type ASTMatcher struct {
	Kind   string      `json:"kind,omitempty" yaml:"kind,omitempty" toml:"kind,omitempty"`
	Name   interface{} `json:"name,omitempty" yaml:"name,omitempty" toml:"name,omitempty"`
	Parent *ASTMatcher `json:"parent,omitempty" yaml:"parent,omitempty" toml:"parent,omitempty"`
	Not    *ASTMatcher `json:"not,omitempty" yaml:"not,omitempty" toml:"not,omitempty"`
}

// WherePredicate restricts which files or contexts a rule applies to.
type WherePredicate struct {
	File string          `json:"file,omitempty" yaml:"file,omitempty" toml:"file,omitempty"`
	Not  *WherePredicate `json:"not,omitempty" yaml:"not,omitempty" toml:"not,omitempty"`
}

// ImportsMatcher controls import ordering rules.
type ImportsMatcher struct {
	Groups         []string `json:"groups,omitempty" yaml:"groups,omitempty" toml:"groups,omitempty"`
	Alphabetize    bool     `json:"alphabetize,omitempty" yaml:"alphabetize,omitempty" toml:"alphabetize,omitempty"`
	NewlineBetween bool     `json:"newlineBetween,omitempty" yaml:"newlineBetween,omitempty" toml:"newlineBetween,omitempty"`
}

// NamingMatcher enforces naming conventions via regex.
type NamingMatcher struct {
	Match   string `json:"match,omitempty" yaml:"match,omitempty" toml:"match,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty" toml:"message,omitempty"`
}
