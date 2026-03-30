package crossfile

import (
	"fmt"
	"strings"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/project"
)

// checkCircularDeps flags files that participate in import cycles.
// Uses Tarjan's SCC via Graph.CyclicFiles().
func checkCircularDeps(g *project.Graph, _ *config.Config) []engine.Diagnostic {
	sccs := g.CyclicFiles()
	if len(sccs) == 0 {
		return nil
	}

	var diags []engine.Diagnostic
	for _, scc := range sccs {
		cycle := strings.Join(scc, " → ")
		for _, file := range scc {
			diags = append(diags, engine.Diagnostic{
				File:    file,
				Line:    1,
				Col:     0,
				EndLine: 1,
				EndCol:  0,
				Message: fmt.Sprintf("Circular dependency: %s", cycle),
			})
		}
	}

	return diags
}
