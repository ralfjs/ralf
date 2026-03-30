// Package crossfile implements lint rules that operate across file boundaries
// using the module dependency graph. These rules detect unused exports,
// circular dependencies, and dead modules.
package crossfile

import (
	"log/slog"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/project"
)

// checkFunc is the signature for a cross-file rule check.
type checkFunc func(g *project.Graph, cfg *config.Config) []engine.Diagnostic

// registry maps rule names to their check functions.
var registry = map[string]checkFunc{
	"no-unused-exports": checkUnusedExports,
	"no-circular-deps":  checkCircularDeps,
	"no-dead-modules":   checkDeadModules,
}

// Run evaluates all active cross-file rules against the module graph.
// Only rules with scope "cross-file" and severity != "off" are run.
func Run(g *project.Graph, cfg *config.Config) []engine.Diagnostic {
	if g == nil {
		return nil
	}

	var allDiags []engine.Diagnostic

	for name := range cfg.Rules {
		rule := cfg.Rules[name]
		if rule.Scope != "cross-file" || rule.Severity == config.SeverityOff {
			continue
		}

		check, ok := registry[name]
		if !ok {
			slog.Debug("cross-file rule not registered", "rule", name)
			continue
		}

		diags := check(g, cfg)

		// Stamp rule name and severity on each diagnostic.
		for i := range diags {
			diags[i].Rule = name
			diags[i].Severity = rule.Severity
			if diags[i].Message == "" {
				diags[i].Message = rule.Message
			}
		}

		allDiags = append(allDiags, diags...)
	}

	return allDiags
}
