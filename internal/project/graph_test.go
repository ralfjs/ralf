package project

import (
	"testing"
)

func testGraph() *Graph {
	exports := map[string][]ExportInfo{
		"/src/utils.ts":  {{Name: "formatDate", Kind: "function", Line: 1}, {Name: "parseDate", Kind: "function", Line: 5}},
		"/src/config.ts": {{Name: "default", Kind: "value", Line: 1}},
		"/src/app.ts":    {{Name: "App", Kind: "class", Line: 1}},
		"/src/dead.ts":   {{Name: "unused", Kind: "function", Line: 1}},
	}
	// Note: imports use absolute resolved paths (resolution happens before graph construction in tests).
	imports := map[string][]ImportInfo{
		"/src/app.ts":   {{Source: "/src/utils.ts", Name: "formatDate", Line: 1}, {Source: "/src/config.ts", Name: "default", Line: 2}},
		"/src/index.ts": {{Source: "/src/app.ts", Name: "App", Line: 1}},
	}
	return NewGraphFromResolved(exports, imports)
}

func TestGraph_ImportedBy(t *testing.T) {
	g := testGraph()

	got := g.ImportedBy("/src/utils.ts")
	if len(got) != 1 || got[0] != "/src/app.ts" {
		t.Errorf("ImportedBy(utils.ts) = %v, want [/src/app.ts]", got)
	}

	got = g.ImportedBy("/src/dead.ts")
	if len(got) != 0 {
		t.Errorf("ImportedBy(dead.ts) = %v, want []", got)
	}
}

func TestGraph_ImportedBySymbol(t *testing.T) {
	g := testGraph()

	got := g.ImportedBySymbol("/src/utils.ts", "formatDate")
	if len(got) != 1 || got[0] != "/src/app.ts" {
		t.Errorf("ImportedBySymbol(utils.ts, formatDate) = %v, want [/src/app.ts]", got)
	}

	got = g.ImportedBySymbol("/src/utils.ts", "parseDate")
	if len(got) != 0 {
		t.Errorf("ImportedBySymbol(utils.ts, parseDate) = %v, want []", got)
	}
}

func TestGraph_ExportedBy(t *testing.T) {
	g := testGraph()

	got := g.ExportedBy("/src/utils.ts")
	if len(got) != 2 {
		t.Fatalf("ExportedBy(utils.ts) has %d exports, want 2", len(got))
	}
}

func TestGraph_ExportMap(t *testing.T) {
	g := testGraph()

	m := g.ExportMap("/src/utils.ts")
	if m == nil {
		t.Fatal("ExportMap returned nil")
	}
	if _, ok := m["formatDate"]; !ok {
		t.Error("expected formatDate in export map")
	}
	if _, ok := m["parseDate"]; !ok {
		t.Error("expected parseDate in export map")
	}
}

func TestGraph_HasCycle_NoCycle(t *testing.T) {
	g := testGraph()
	cycle := g.HasCycle()
	if cycle != nil {
		t.Errorf("expected no cycle, got %v", cycle)
	}
}

func TestGraph_HasCycle_SimpleCycle(t *testing.T) {
	exports := map[string][]ExportInfo{
		"/a.ts": {{Name: "a", Kind: "value", Line: 1}},
		"/b.ts": {{Name: "b", Kind: "value", Line: 1}},
	}
	imports := map[string][]ImportInfo{
		"/a.ts": {{Source: "/b.ts", Name: "b", Line: 1}},
		"/b.ts": {{Source: "/a.ts", Name: "a", Line: 1}},
	}
	g := NewGraphFromResolved(exports, imports)

	cycle := g.HasCycle()
	if cycle == nil {
		t.Fatal("expected a cycle")
	}
	if len(cycle) < 2 {
		t.Errorf("cycle too short: %v", cycle)
	}
}

func TestGraph_HasCycle_TransitiveCycle(t *testing.T) {
	exports := map[string][]ExportInfo{
		"/a.ts": {}, "/b.ts": {}, "/c.ts": {},
	}
	imports := map[string][]ImportInfo{
		"/a.ts": {{Source: "/b.ts", Name: "*", Line: 1}},
		"/b.ts": {{Source: "/c.ts", Name: "*", Line: 1}},
		"/c.ts": {{Source: "/a.ts", Name: "*", Line: 1}},
	}
	g := NewGraphFromResolved(exports, imports)

	cycle := g.HasCycle()
	if cycle == nil {
		t.Fatal("expected a cycle")
	}
	if len(cycle) < 3 {
		t.Errorf("expected 3+ node cycle, got %v", cycle)
	}
}

func TestGraph_HasCycle_Diamond(t *testing.T) {
	// A → B, A → C, B → D, C → D — no cycle.
	exports := map[string][]ExportInfo{
		"/a.ts": {}, "/b.ts": {}, "/c.ts": {}, "/d.ts": {},
	}
	imports := map[string][]ImportInfo{
		"/a.ts": {{Source: "/b.ts", Name: "*", Line: 1}, {Source: "/c.ts", Name: "*", Line: 2}},
		"/b.ts": {{Source: "/d.ts", Name: "*", Line: 1}},
		"/c.ts": {{Source: "/d.ts", Name: "*", Line: 1}},
	}
	g := NewGraphFromResolved(exports, imports)

	cycle := g.HasCycle()
	if cycle != nil {
		t.Errorf("expected no cycle in diamond, got %v", cycle)
	}
}

func TestGraph_CyclicFiles_SelfImport(t *testing.T) {
	exports := map[string][]ExportInfo{"/a.ts": {{Name: "a", Kind: "value", Line: 1}}}
	imports := map[string][]ImportInfo{"/a.ts": {{Source: "/a.ts", Name: "a", Line: 1}}}
	g := NewGraphFromResolved(exports, imports)

	sccs := g.CyclicFiles()
	if len(sccs) != 1 {
		t.Fatalf("expected 1 SCC for self-import, got %d", len(sccs))
	}
	if sccs[0][0] != "/a.ts" {
		t.Errorf("expected /a.ts in self-cycle, got %v", sccs[0])
	}
}

func TestGraph_DeadModules(t *testing.T) {
	g := testGraph()

	dead := g.DeadModules([]string{"**/index.ts"})
	// dead.ts and config.ts are imported, utils.ts is imported, app.ts is imported.
	// Only dead.ts has no importers — but we need to check.
	found := false
	for _, d := range dead {
		if d == "/src/dead.ts" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected /src/dead.ts in dead modules, got %v", dead)
	}
}

func TestGraph_UpdateFile(t *testing.T) {
	g := testGraph()

	// Verify initial state: app.ts imports from utils.ts.
	importers := g.ImportedBy("/src/utils.ts")
	if len(importers) != 1 || importers[0] != "/src/app.ts" {
		t.Fatalf("initial ImportedBy(utils.ts) = %v", importers)
	}

	// Update app.ts to no longer import from utils.ts.
	g.UpdateFile("/src/app.ts",
		[]ExportInfo{{Name: "App", Kind: "class", Line: 1}},
		[]ImportInfo{{Source: "/src/config.ts", Name: "default", Line: 1}}, // only config, no utils
	)

	// utils.ts should now have zero importers.
	importers = g.ImportedBy("/src/utils.ts")
	if len(importers) != 0 {
		t.Errorf("after update, ImportedBy(utils.ts) = %v, want []", importers)
	}

	// config.ts should still have app.ts as importer.
	importers = g.ImportedBy("/src/config.ts")
	if len(importers) != 1 || importers[0] != "/src/app.ts" {
		t.Errorf("after update, ImportedBy(config.ts) = %v", importers)
	}
}

func TestGraph_UpdateFile_AddNewImport(t *testing.T) {
	g := testGraph()

	// Update app.ts to also import from dead.ts.
	g.UpdateFile("/src/app.ts",
		[]ExportInfo{{Name: "App", Kind: "class", Line: 1}},
		[]ImportInfo{
			{Source: "/src/utils.ts", Name: "formatDate", Line: 1},
			{Source: "/src/config.ts", Name: "default", Line: 2},
			{Source: "/src/dead.ts", Name: "unused", Line: 3},
		},
	)

	// dead.ts should now have an importer.
	importers := g.ImportedBy("/src/dead.ts")
	if len(importers) != 1 || importers[0] != "/src/app.ts" {
		t.Errorf("after update, ImportedBy(dead.ts) = %v", importers)
	}
}

func TestGraph_RemoveFile(t *testing.T) {
	g := testGraph()

	// Verify app.ts exists in graph before removal.
	if exports := g.ExportedBy("/src/app.ts"); len(exports) == 0 {
		t.Fatal("expected exports for app.ts before removal")
	}

	g.RemoveFile("/src/app.ts")

	// Should no longer appear in exports or AllFiles.
	if exports := g.ExportedBy("/src/app.ts"); len(exports) != 0 {
		t.Errorf("expected no exports after removal, got %v", exports)
	}

	allFiles := g.AllFiles()
	for _, f := range allFiles {
		if f == "/src/app.ts" {
			t.Error("removed file should not appear in AllFiles")
		}
	}

	// utils.ts should no longer have app.ts as an importer.
	importers := g.ImportedBy("/src/utils.ts")
	for _, imp := range importers {
		if imp == "/src/app.ts" {
			t.Error("removed file should not appear in ImportedBy")
		}
	}
}
