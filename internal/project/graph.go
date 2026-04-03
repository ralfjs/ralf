package project

import (
	"context"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// ExportInfo describes a single exported symbol from a file.
type ExportInfo struct {
	Name string // symbol name ("default" for default exports)
	Kind string // "function", "class", "variable", "type", "value"
	Line int    // 1-based line number
}

// ImportInfo describes a single import reference in a file.
type ImportInfo struct {
	Source string // raw specifier as written ("./bar", "react", etc.)
	Name   string // imported symbol name ("default", "*", or named)
	Line   int    // 1-based line number
}

// Graph holds the in-memory module dependency graph.
type Graph struct {
	mu              sync.RWMutex
	exports         map[string][]ExportInfo        // file → exports
	imports         map[string][]ImportInfo        // file → imports
	importedBy      map[string]map[string]struct{} // resolved target → set of importing files
	edges           map[string]map[string]struct{} // source file → set of resolved targets
	symbolImporters map[string]map[string]struct{} // "file:symbol" → set of importing files
}

// BuildGraph loads export/import data from the cache and constructs the graph.
func BuildGraph(ctx context.Context, cache *Cache) (*Graph, error) {
	exports, err := cache.LoadAllExports(ctx)
	if err != nil {
		return nil, err
	}
	imports, err := cache.LoadAllImports(ctx)
	if err != nil {
		return nil, err
	}
	return NewGraph(exports, imports), nil
}

// NewGraph constructs a graph from pre-loaded export/import data.
// Import specifiers are resolved to absolute paths where possible.
func NewGraph(exports map[string][]ExportInfo, imports map[string][]ImportInfo) *Graph {
	g := &Graph{
		exports:         exports,
		imports:         imports,
		importedBy:      make(map[string]map[string]struct{}),
		edges:           make(map[string]map[string]struct{}),
		symbolImporters: make(map[string]map[string]struct{}),
	}

	for fromFile, fileImports := range imports {
		g.addImportEdges(fromFile, fileImports)
	}

	return g
}

// NewGraphFromResolved constructs a graph from pre-resolved imports (sources
// are already absolute paths). Used by tests and cross-file rules.
func NewGraphFromResolved(exports map[string][]ExportInfo, imports map[string][]ImportInfo) *Graph {
	g := &Graph{
		exports:         exports,
		imports:         imports,
		importedBy:      make(map[string]map[string]struct{}),
		edges:           make(map[string]map[string]struct{}),
		symbolImporters: make(map[string]map[string]struct{}),
	}

	for fromFile, fileImports := range imports {
		g.addImportEdges(fromFile, fileImports)
	}

	return g
}

// addImportEdges resolves import specifiers and adds edges to the graph.
// Caller must hold the write lock if needed (NewGraph doesn't need it,
// UpdateFile acquires it before calling).
func (g *Graph) addImportEdges(fromFile string, fileImports []ImportInfo) {
	for _, imp := range fileImports {
		resolved, ok := g.resolveImport(imp.Source, fromFile)
		if !ok {
			continue
		}

		if g.edges[fromFile] == nil {
			g.edges[fromFile] = make(map[string]struct{})
		}
		g.edges[fromFile][resolved] = struct{}{}

		if g.importedBy[resolved] == nil {
			g.importedBy[resolved] = make(map[string]struct{})
		}
		g.importedBy[resolved][fromFile] = struct{}{}

		key := resolved + ":" + imp.Name
		if g.symbolImporters[key] == nil {
			g.symbolImporters[key] = make(map[string]struct{})
		}
		g.symbolImporters[key][fromFile] = struct{}{}
	}
}

// UpdateFile replaces exports and imports for a single file in the graph.
// Removes old edges, adds new ones. Used for incremental updates.
func (g *Graph) UpdateFile(file string, newExports []ExportInfo, newImports []ImportInfo) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Remove old edges originating from this file.
	for target := range g.edges[file] {
		if set := g.importedBy[target]; set != nil {
			delete(set, file)
			if len(set) == 0 {
				delete(g.importedBy, target)
			}
		}
	}
	delete(g.edges, file)

	// Remove old symbol importer entries for this file.
	// Only iterate keys associated with the file's old imports (not the entire map).
	for _, imp := range g.imports[file] {
		resolved, ok := g.resolveImport(imp.Source, file)
		if !ok {
			continue
		}
		key := resolved + ":" + imp.Name
		if set := g.symbolImporters[key]; set != nil {
			delete(set, file)
			if len(set) == 0 {
				delete(g.symbolImporters, key)
			}
		}
	}

	// Update exports.
	g.exports[file] = newExports

	// Update imports and rebuild edges.
	g.imports[file] = newImports
	g.addImportEdges(file, newImports)
}

// RemoveFile completely removes a file from the graph, including its exports,
// imports, and all edges. Unlike UpdateFile with nil slices (which leaves empty
// keys), this ensures the file no longer appears in AllFiles or DeadModules.
func (g *Graph) RemoveFile(file string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Remove edges originating from this file.
	for target := range g.edges[file] {
		if set := g.importedBy[target]; set != nil {
			delete(set, file)
			if len(set) == 0 {
				delete(g.importedBy, target)
			}
		}
	}
	delete(g.edges, file)

	// Remove symbol importer entries for this file.
	for _, imp := range g.imports[file] {
		resolved, ok := g.resolveImport(imp.Source, file)
		if !ok {
			continue
		}
		key := resolved + ":" + imp.Name
		if set := g.symbolImporters[key]; set != nil {
			delete(set, file)
			if len(set) == 0 {
				delete(g.symbolImporters, key)
			}
		}
	}

	// Remove incoming edges (other files importing this file).
	for source := range g.importedBy[file] {
		if set := g.edges[source]; set != nil {
			delete(set, file)
			if len(set) == 0 {
				delete(g.edges, source)
			}
		}
	}
	delete(g.importedBy, file)

	delete(g.exports, file)
	delete(g.imports, file)
}

// AllFiles returns all files known to the graph (files with exports or imports).
func (g *Graph) AllFiles() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	seen := make(map[string]struct{})
	for f := range g.exports {
		seen[f] = struct{}{}
	}
	for f := range g.imports {
		seen[f] = struct{}{}
	}
	return setToSorted(seen)
}

// ImportedBy returns files that import from the given file.
func (g *Graph) ImportedBy(file string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return setToSorted(g.importedBy[file])
}

// ImportedBySymbol returns files that import a specific symbol from a file.
func (g *Graph) ImportedBySymbol(file, symbol string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return setToSorted(g.symbolImporters[file+":"+symbol])
}

// ExportedBy returns a copy of the exports of a given file.
func (g *Graph) ExportedBy(file string) []ExportInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	exps := g.exports[file]
	if len(exps) == 0 {
		return nil
	}
	out := make([]ExportInfo, len(exps))
	copy(out, exps)
	return out
}

// ExportMap returns a map of symbol name → ExportInfo for a file.
func (g *Graph) ExportMap(file string) map[string]ExportInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	exps := g.exports[file]
	if len(exps) == 0 {
		return nil
	}
	m := make(map[string]ExportInfo, len(exps))
	for _, e := range exps {
		m[e.Name] = e
	}
	return m
}

// Dependents returns all files that directly depend on the given file.
func (g *Graph) Dependents(file string) []string {
	return g.ImportedBy(file)
}

// HasCycle reports whether the graph contains any import cycles.
// Returns the first cycle found as a path of file names, or nil.
func (g *Graph) HasCycle() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully explored
	)

	color := make(map[string]int, len(g.edges))
	parent := make(map[string]string)

	var cycle []string

	var dfs func(node string) bool
	dfs = func(node string) bool {
		color[node] = gray
		for neighbor := range g.edges[node] {
			switch color[neighbor] {
			case gray:
				// Found cycle — reconstruct path.
				cycle = []string{neighbor, node}
				for p := node; p != neighbor; {
					p = parent[p]
					cycle = append(cycle, p)
				}
				// Reverse to get the cycle in traversal order.
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				return true
			case white:
				parent[neighbor] = node
				if dfs(neighbor) {
					return true
				}
			}
		}
		color[node] = black
		return false
	}

	for node := range g.edges {
		if color[node] == white {
			if dfs(node) {
				return cycle
			}
		}
	}
	return nil
}

// CyclicFiles returns all files that participate in at least one import cycle,
// grouped into strongly connected components using Tarjan's algorithm.
// Each inner slice is one SCC. Components normally have size >= 2, but
// single-node components are included when the file has a self-import.
func (g *Graph) CyclicFiles() [][]string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	type nodeState struct {
		index   int
		lowlink int
		onStack bool
	}

	state := make(map[string]*nodeState, len(g.edges))
	var stack []string
	index := 0
	var sccs [][]string

	var strongconnect func(v string)
	strongconnect = func(v string) {
		s := &nodeState{index: index, lowlink: index, onStack: true}
		state[v] = s
		index++
		stack = append(stack, v)

		for w := range g.edges[v] {
			ws, visited := state[w]
			if !visited {
				strongconnect(w)
				if state[w].lowlink < s.lowlink {
					s.lowlink = state[w].lowlink
				}
			} else if ws.onStack {
				if ws.index < s.lowlink {
					s.lowlink = ws.index
				}
			}
		}

		if s.lowlink == s.index {
			var scc []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				state[w].onStack = false
				scc = append(scc, w)
				if w == v {
					break
				}
			}
			// Include multi-node SCCs and single-node SCCs with self-edges.
			include := len(scc) >= 2
			if !include && len(scc) == 1 {
				if neighbors, ok := g.edges[scc[0]]; ok {
					_, include = neighbors[scc[0]]
				}
			}
			if include {
				sort.Strings(scc)
				sccs = append(sccs, scc)
			}
		}
	}

	for v := range g.edges {
		if _, visited := state[v]; !visited {
			strongconnect(v)
		}
	}

	return sccs
}

// DeadModules returns files that are not imported by any other file.
// Files matching entryPatterns (glob) are excluded since they are expected
// entry points. Pass nil for no exclusions.
func (g *Graph) DeadModules(entryPatterns []string) []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Collect all known files.
	allFiles := make(map[string]struct{})
	for f := range g.exports {
		allFiles[f] = struct{}{}
	}
	for f := range g.imports {
		allFiles[f] = struct{}{}
	}

	var dead []string
	for f := range allFiles {
		if len(g.importedBy[f]) > 0 {
			continue
		}
		// Check if it's an entry point.
		isEntry := false
		for _, pattern := range entryPatterns {
			if matchGlob(pattern, f) {
				isEntry = true
				break
			}
		}
		if !isEntry {
			dead = append(dead, f)
		}
	}
	sort.Strings(dead)
	return dead
}

// resolveImport resolves an import source to a canonical target path.
// Absolute paths are accepted as-is if they're known files in the graph;
// otherwise falls through to ResolveSpecifier for validation.
// Must be called with at least RLock held (or during construction).
func (g *Graph) resolveImport(source, fromFile string) (string, bool) {
	if filepath.IsAbs(source) {
		if _, known := g.exports[source]; known {
			return source, true
		}
		if _, known := g.imports[source]; known {
			return source, true
		}
		return ResolveSpecifier(source, fromFile)
	}
	return ResolveSpecifier(source, fromFile)
}

func setToSorted(s map[string]struct{}) []string {
	if len(s) == 0 {
		return nil
	}
	result := make([]string, 0, len(s))
	for k := range s {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

// matchGlob matches a path against a doublestar glob pattern.
func matchGlob(pattern, path string) bool {
	ok, _ := doublestar.Match(pattern, path)
	return ok
}
