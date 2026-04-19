package crossfile

import (
	"fmt"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/project"
)

// checkUnusedExports flags exported symbols that are not imported by any other
// file in the project. Entry point files' exports are excluded since they
// serve as the public API.
func checkUnusedExports(g *project.Graph, cfg *config.Config) []engine.Diagnostic {
	entryPatterns := resolveEntryPatterns(cfg)
	var diags []engine.Diagnostic

	for _, file := range g.AllFiles() {
		exports := g.ExportedBy(file)
		if len(exports) == 0 {
			continue
		}

		// Skip entry point files — their exports are the public API.
		if matchAnyPattern(entryPatterns, file) {
			continue
		}

		// Compute wildcard importers once per file to avoid repeated work per export.
		hasWildcardImporters := len(g.ImportedBySymbol(file, "*")) > 0

		for _, exp := range exports {
			importers := g.ImportedBySymbol(file, exp.Name)
			if len(importers) == 0 {
				if hasWildcardImporters {
					continue
				}
				diags = append(diags, engine.Diagnostic{
					File:    file,
					Line:    exp.Line,
					Col:     0,
					EndLine: exp.Line,
					EndCol:  0,
					Message: fmt.Sprintf("Export '%s' is not imported by any file.", exp.Name),
				})
			}
		}
	}

	return diags
}
