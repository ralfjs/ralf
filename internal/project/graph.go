package project

import (
	"context"
	"sort"
	"sync"
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
		for _, imp := range fileImports {
			resolved, ok := ResolveSpecifier(imp.Source, fromFile)
			if !ok {
				continue // bare specifier or unresolvable
			}

			// Add edge: fromFile → resolved
			if g.edges[fromFile] == nil {
				g.edges[fromFile] = make(map[string]struct{})
			}
			g.edges[fromFile][resolved] = struct{}{}

			// Add reverse edge: resolved → fromFile
			if g.importedBy[resolved] == nil {
				g.importedBy[resolved] = make(map[string]struct{})
			}
			g.importedBy[resolved][fromFile] = struct{}{}

			// Add symbol-level index
			key := resolved + ":" + imp.Name
			if g.symbolImporters[key] == nil {
				g.symbolImporters[key] = make(map[string]struct{})
			}
			g.symbolImporters[key][fromFile] = struct{}{}
		}
	}

	return g
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

// ExportedBy returns the exports of a given file.
func (g *Graph) ExportedBy(file string) []ExportInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.exports[file]
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

// matchGlob performs a simple suffix/contains match for entry point detection.
// For full glob support, this would use doublestar, but for MVP a suffix match
// on common patterns like "**/index.*" or "**/*.test.*" is sufficient.
func matchGlob(pattern, path string) bool {
	// Simple: check if path ends with the non-** part of the pattern.
	if len(pattern) > 3 && pattern[:3] == "**/" {
		suffix := pattern[3:]
		return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
	}
	return path == pattern
}
