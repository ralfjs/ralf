package config

import (
	"path/filepath"
)

// Merge returns the effective rule set for a given file path by applying
// matching overrides on top of the base rules. Later overrides win.
func Merge(cfg *Config, filePath string) map[string]RuleConfig {
	if len(cfg.Overrides) == 0 {
		return cfg.Rules
	}

	result := make(map[string]RuleConfig, len(cfg.Rules))
	for name := range cfg.Rules {
		result[name] = cfg.Rules[name]
	}

	for i := range cfg.Overrides {
		if !matchesAnyGlob(cfg.Overrides[i].Files, filePath) {
			continue
		}
		for name := range cfg.Overrides[i].Rules {
			result[name] = cfg.Overrides[i].Rules[name]
		}
	}

	return result
}

func matchesAnyGlob(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if matched, err := filepath.Match(pattern, path); err == nil && matched {
			return true
		}
		// Also try matching against just the filename for simple globs
		base := filepath.Base(path)
		if matched, err := filepath.Match(pattern, base); err == nil && matched {
			return true
		}
	}
	return false
}
