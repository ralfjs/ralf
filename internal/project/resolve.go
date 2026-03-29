package project

import (
	"os"
	"path/filepath"
	"strings"
)

// jsExtensions is the order in which extensions are tried during resolution.
var jsExtensions = []string{"", ".ts", ".tsx", ".js", ".jsx", ".mts", ".mjs", ".cts", ".cjs"}

// ResolveSpecifier resolves a relative import specifier to an absolute file path.
// Returns ("", false) for bare specifiers, node builtins, or unresolvable paths.
func ResolveSpecifier(specifier, fromFile string) (string, bool) {
	// Bare specifiers (no relative prefix) are external dependencies.
	if !strings.HasPrefix(specifier, ".") && !strings.HasPrefix(specifier, "/") {
		return "", false
	}

	dir := filepath.Dir(fromFile)
	base := filepath.Join(dir, specifier)

	// Try the specifier with each extension.
	if resolved, ok := tryResolveFile(base); ok {
		return resolved, true
	}

	// Try as a directory with index file.
	if resolved, ok := tryResolveIndex(base); ok {
		return resolved, true
	}

	return "", false
}

// tryResolveFile tries the path as-is and with each JS/TS extension.
func tryResolveFile(base string) (string, bool) {
	for _, ext := range jsExtensions {
		candidate := base + ext
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate, true
			}
			return abs, true
		}
	}
	return "", false
}

// tryResolveIndex tries the path as a directory containing an index file.
func tryResolveIndex(dir string) (string, bool) {
	for _, ext := range jsExtensions {
		if ext == "" {
			continue // "index" alone is not a valid file
		}
		candidate := filepath.Join(dir, "index"+ext)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate, true
			}
			return abs, true
		}
	}
	return "", false
}
