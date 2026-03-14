package cli

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/Hideart/ralf/internal/parser"
	"github.com/bmatcuk/doublestar/v4"
)

// hardcodedSkips are directories always excluded from traversal.
var hardcodedSkips = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"dist":         {},
	"build":        {},
	".next":        {},
	"coverage":     {},
}

// discoverFiles resolves paths to lintable files. Explicit files are accepted
// if they have a supported extension. Directories are walked recursively.
// Ignore patterns use doublestar matching (supports **).
func discoverFiles(paths, ignorePatterns []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string

	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}

		info, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			if _, ok := parser.LangFromPath(abs); ok {
				if !isIgnored(abs, ignorePatterns) {
					if _, dup := seen[abs]; !dup {
						seen[abs] = struct{}{}
						files = append(files, abs)
					}
				}
			}
			continue
		}

		err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.IsDir() {
				if _, skip := hardcodedSkips[d.Name()]; skip {
					return filepath.SkipDir
				}
				return nil
			}

			if _, ok := parser.LangFromPath(path); !ok {
				return nil
			}

			if isIgnored(path, ignorePatterns) {
				return nil
			}

			if _, dup := seen[path]; !dup {
				seen[path] = struct{}{}
				files = append(files, path)
			}

			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	sort.Strings(files)
	return files, nil
}

// isIgnored checks if a path matches any ignore pattern.
func isIgnored(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, err := doublestar.Match(pattern, path); err == nil && matched {
			return true
		}
		// Also try matching against the relative-looking path from the base.
		base := filepath.Base(path)
		if matched, err := doublestar.Match(pattern, base); err == nil && matched {
			return true
		}
	}
	return false
}
