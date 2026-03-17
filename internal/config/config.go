// Package config provides types and loading for linter configuration files.
package config

// Config is the top-level configuration structure.
type Config struct {
	Rules     map[string]RuleConfig `json:"rules" yaml:"rules"`
	Ignores   []string              `json:"ignores,omitempty" yaml:"ignores,omitempty"`
	Extends   []string              `json:"extends,omitempty" yaml:"extends,omitempty"`
	Overrides []Override            `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

// RuleConfig defines a single lint rule. Exactly one matcher field (Regex,
// Pattern, AST, Imports, or Builtin) must be set.
type RuleConfig struct {
	Severity Severity        `json:"severity" yaml:"severity"`
	Message  string          `json:"message,omitempty" yaml:"message,omitempty"`
	Regex    string          `json:"regex,omitempty" yaml:"regex,omitempty"`
	Pattern  string          `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	AST      *ASTMatcher     `json:"ast,omitempty" yaml:"ast,omitempty"`
	Imports  *ImportsMatcher `json:"imports,omitempty" yaml:"imports,omitempty"`
	Builtin  bool            `json:"builtin,omitempty" yaml:"builtin,omitempty"` // custom Go checker
	Naming   *NamingMatcher  `json:"naming,omitempty" yaml:"naming,omitempty"`
	Where    *WherePredicate `json:"where,omitempty" yaml:"where,omitempty"`
	Scope    string          `json:"scope,omitempty" yaml:"scope,omitempty"`
	Fix      string          `json:"fix,omitempty" yaml:"fix,omitempty"`
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
	Files []string              `json:"files" yaml:"files"`
	Rules map[string]RuleConfig `json:"rules" yaml:"rules"`
}

// ASTMatcher describes a structural AST query for matching nodes.
type ASTMatcher struct {
	Kind     string      `json:"kind,omitempty" yaml:"kind,omitempty"`
	Name     interface{} `json:"name,omitempty" yaml:"name,omitempty"`
	Parent   *ASTMatcher `json:"parent,omitempty" yaml:"parent,omitempty"`
	Children interface{} `json:"children,omitempty" yaml:"children,omitempty"`
	Returns  string      `json:"returns,omitempty" yaml:"returns,omitempty"`
	Capture  interface{} `json:"capture,omitempty" yaml:"capture,omitempty"`
	Params   interface{} `json:"params,omitempty" yaml:"params,omitempty"`
	Not      *ASTMatcher `json:"not,omitempty" yaml:"not,omitempty"`
}

// WherePredicate restricts which files or contexts a rule applies to.
type WherePredicate struct {
	File          string          `json:"file,omitempty" yaml:"file,omitempty"`
	Not           *WherePredicate `json:"not,omitempty" yaml:"not,omitempty"`
	ImportCrosses string          `json:"importCrosses,omitempty" yaml:"importCrosses,omitempty"`
}

// ImportsMatcher controls import ordering rules.
type ImportsMatcher struct {
	Groups         []string `json:"groups,omitempty" yaml:"groups,omitempty"`
	Alphabetize    bool     `json:"alphabetize,omitempty" yaml:"alphabetize,omitempty"`
	NewlineBetween bool     `json:"newlineBetween,omitempty" yaml:"newlineBetween,omitempty"`
}

// NamingMatcher enforces naming conventions via regex.
type NamingMatcher struct {
	Match   string `json:"match,omitempty" yaml:"match,omitempty"`
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}
