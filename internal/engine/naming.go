package engine

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/rure-go"

	"github.com/Hideart/ralf/internal/config"
)

// ErrNamingCompile indicates a naming convention regex failed to compile.
var ErrNamingCompile = errors.New("naming compile failed")

// compiledNaming holds a compiled naming convention regex and its message.
type compiledNaming struct {
	re      *rure.Regex
	message string
}

// compileNaming compiles a NamingMatcher into a compiledNaming. Returns nil
// for nil input. Returns an error if the regex fails to compile.
func compileNaming(ruleName string, nm *config.NamingMatcher) (*compiledNaming, error) {
	if nm == nil {
		return nil, nil
	}

	if nm.Match == "" {
		return nil, fmt.Errorf("rule %q: %w: naming.match is required", ruleName, ErrNamingCompile)
	}

	re, err := rure.Compile(nm.Match)
	if err != nil {
		return nil, fmt.Errorf("rule %q: %w: invalid naming regex %q: %w", ruleName, ErrNamingCompile, nm.Match, err)
	}

	return &compiledNaming{
		re:      re,
		message: nm.Message,
	}, nil
}

// matches reports whether name conforms to the naming convention (regex matches).
// A nil receiver always returns true (no naming constraint).
func (cn *compiledNaming) matches(name string) bool {
	if cn == nil {
		return true
	}
	return cn.re.IsMatch(name)
}
