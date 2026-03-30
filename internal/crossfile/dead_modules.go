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
	"**/index.js", "**/index.ts", "**/index.jsx", "**/index.tsx",
	"**/*.test.js", "**/*.test.ts", "**/*.test.jsx", "**/*.test.tsx",
	"**/*.spec.js", "**/*.spec.ts", "**/*.spec.jsx", "**/*.spec.tsx",
	"**/*.config.js", "**/*.config.ts", "**/*.config.jsx", "**/*.config.tsx",
	"**/main.js", "**/main.ts", "**/main.jsx", "**/main.tsx",
	"**/app.js", "**/app.ts", "**/app.jsx", "**/app.tsx",
}

// checkDeadModules flags files that are not imported by any other module.
// Entry point files are excluded.
func checkDeadModules(g *project.Graph, cfg *config.Config) []engine.Diagnostic {
	dead := g.DeadModules(resolveEntryPatterns(cfg))
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

// resolveEntryPatterns returns configured entry patterns or defaults.
func resolveEntryPatterns(cfg *config.Config) []string {
	if len(cfg.EntryPoints) > 0 {
		return cfg.EntryPoints
	}
	return defaultEntryPatterns
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
