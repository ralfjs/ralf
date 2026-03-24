package engine

import (
	"github.com/bmatcuk/doublestar/v4"
	"github.com/ralfjs/ralf/internal/config"
)

// matchesWhere evaluates a Where predicate against a file path.
// A nil predicate matches all files.
//
// Field precedence: File is evaluated first, then Not inverts the inner
// predicate. If both File and Not are set on the same level, File takes
// precedence and Not is ignored.
func matchesWhere(where *config.WherePredicate, filePath string) bool {
	if where == nil {
		return true
	}

	if where.File != "" {
		matched, err := doublestar.Match(where.File, filePath)
		if err != nil {
			return false
		}
		return matched
	}

	if where.Not != nil {
		return !matchesWhere(where.Not, filePath)
	}

	// ImportCrosses and other predicates are not yet implemented.
	// Unknown predicates match all files (don't block linting).
	return true
}
