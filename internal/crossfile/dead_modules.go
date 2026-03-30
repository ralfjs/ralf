package crossfile

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/project"
)

// defaultEntryPatterns are file patterns excluded from no-dead-modules and
// no-unused-exports checks. These files are expected entry points.
var defaultEntryPatterns = []string{
	"**/index.{js,ts,jsx,tsx}",
	"**/*.test.{js,ts,jsx,tsx}",
	"**/*.spec.{js,ts,jsx,tsx}",
	"**/*.config.{js,ts,jsx,tsx}",
	"**/main.{js,ts,jsx,tsx}",
	"**/app.{js,ts,jsx,tsx}",
}

// checkDeadModules flags files that are not imported by any other module.
// Entry point files are excluded.
func checkDeadModules(g *project.Graph, cfg *config.Config) []engine.Diagnostic {
	entryPatterns := cfg.EntryPoints
	if len(entryPatterns) == 0 {
		entryPatterns = defaultEntryPatterns
	}

	dead := g.DeadModules(entryPatterns)
	if len(dead) == 0 {
		return nil
	}

	diags := make([]engine.Diagnostic, len(dead))
	for i, file := range dead {
		diags[i] = engine.Diagnostic{
			File:    file,
			Line:    1,
			Col:     0,
			EndLine: 1,
			EndCol:  0,
		}
	}
	return diags
}

// matchAnyPattern returns true if path matches any of the glob patterns.
func matchAnyPattern(patterns []string, path string) bool {
	for _, p := range patterns {
		ok, _ := doublestar.Match(p, path)
		if ok {
			return true
		}
	}
	return false
}
