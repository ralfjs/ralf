package crossfile

import (
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/project"
)

// helper to build a test graph from pre-resolved data.
func buildTestGraph(exports map[string][]project.ExportInfo, imports map[string][]project.ImportInfo) *project.Graph {
	return project.NewGraphFromResolved(exports, imports)
}

func TestCheckUnusedExports_Basic(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{
			"/src/utils.ts": {
				{Name: "formatDate", Kind: "function", Line: 1},
				{Name: "parseDate", Kind: "function", Line: 5},
			},
		},
		map[string][]project.ImportInfo{
			"/src/app.ts": {{Source: "/src/utils.ts", Name: "formatDate", Line: 1}},
		},
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-unused-exports": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].File != "/src/utils.ts" || diags[0].Line != 5 {
		t.Errorf("expected unused parseDate at line 5, got %+v", diags[0])
	}
}

func TestCheckUnusedExports_WildcardImport(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{
			"/src/utils.ts": {{Name: "formatDate", Kind: "function", Line: 1}},
		},
		map[string][]project.ImportInfo{
			"/src/app.ts": {{Source: "/src/utils.ts", Name: "*", Line: 1}},
		},
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-unused-exports": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics with wildcard import, got %d", len(diags))
	}
}

func TestCheckUnusedExports_EntryPointSkipped(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{
			"/src/index.ts": {{Name: "default", Kind: "value", Line: 1}},
			"/src/dead.ts":  {{Name: "unused", Kind: "function", Line: 1}},
		},
		nil,
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-unused-exports": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	// index.ts is an entry point — its exports should not be flagged.
	for _, d := range diags {
		if d.File == "/src/index.ts" {
			t.Errorf("entry point index.ts should not be flagged: %+v", d)
		}
	}
	// dead.ts should be flagged.
	found := false
	for _, d := range diags {
		if d.File == "/src/dead.ts" {
			found = true
		}
	}
	if !found {
		t.Error("expected dead.ts unused export to be flagged")
	}
}

func TestCheckCircularDeps_NoCycle(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{"/a.ts": {}, "/b.ts": {}},
		map[string][]project.ImportInfo{"/a.ts": {{Source: "/b.ts", Name: "*", Line: 1}}},
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-circular-deps": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics, got %d", len(diags))
	}
}

func TestCheckCircularDeps_SimpleCycle(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{"/a.ts": {}, "/b.ts": {}},
		map[string][]project.ImportInfo{
			"/a.ts": {{Source: "/b.ts", Name: "*", Line: 1}},
			"/b.ts": {{Source: "/a.ts", Name: "*", Line: 1}},
		},
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-circular-deps": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics (one per file in cycle), got %d", len(diags))
	}
}

func TestCheckDeadModules_Basic(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{
			"/src/utils.ts": {{Name: "foo", Kind: "function", Line: 1}},
			"/src/dead.ts":  {{Name: "bar", Kind: "function", Line: 1}},
		},
		map[string][]project.ImportInfo{
			"/src/app.ts": {{Source: "/src/utils.ts", Name: "foo", Line: 1}},
		},
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-dead-modules": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	found := false
	for _, d := range diags {
		if d.File == "/src/dead.ts" {
			found = true
		}
	}
	if !found {
		t.Error("expected dead.ts to be flagged as dead module")
	}
}

func TestCheckDeadModules_EntryPointExcluded(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{
			"/src/index.ts": {{Name: "default", Kind: "value", Line: 1}},
		},
		nil,
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-dead-modules": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	for _, d := range diags {
		if d.File == "/src/index.ts" {
			t.Error("entry point index.ts should not be flagged as dead module")
		}
	}
}

func TestRun_DisabledRulesSkipped(t *testing.T) {
	g := buildTestGraph(
		map[string][]project.ExportInfo{"/src/dead.ts": {{Name: "foo", Kind: "function", Line: 1}}},
		nil,
	)

	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-dead-modules": {Severity: config.SeverityOff, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(g, cfg)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics for disabled rule, got %d", len(diags))
	}
}

func TestRun_NilGraph(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-dead-modules": {Severity: config.SeverityWarn, Scope: "cross-file", Builtin: true},
		},
	}

	diags := Run(nil, cfg)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics for nil graph, got %d", len(diags))
	}
}
