package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrCircularExtend is returned when a circular extends chain is detected.
var ErrCircularExtend = errors.New("circular extends detected")

// ResolveExtends loads and merges all configs referenced by cfg.Extends.
// Paths are resolved relative to baseDir. Returns a new *Config with all
// extended rules merged (current config wins).
func ResolveExtends(cfg *Config, baseDir string) (*Config, error) {
	if len(cfg.Extends) == 0 {
		return cfg, nil
	}
	ctx := &extendsCtx{
		cache: make(map[string]*Config),
	}
	return ctx.resolve(cfg, baseDir, make(map[string]struct{}))
}

// extendsCtx holds memoized configs across a single ResolveExtends call so
// that diamond dependencies (A→B→D, A→C→D) load each file at most once.
type extendsCtx struct {
	cache map[string]*Config
}

func (c *extendsCtx) resolve(cfg *Config, baseDir string, ancestors map[string]struct{}) (*Config, error) {
	if len(cfg.Extends) == 0 {
		return cfg, nil
	}

	merged := &Config{
		Rules: make(map[string]RuleConfig),
	}

	for _, ext := range cfg.Extends {
		path, err := resolveExtendPath(ext, baseDir)
		if err != nil {
			return nil, err
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("config: resolve extends path %q: %w", ext, err)
		}

		if _, ok := ancestors[abs]; ok {
			return nil, fmt.Errorf("config: %w: %s", ErrCircularExtend, abs)
		}

		// Return cached result for diamonds — avoids re-loading and
		// re-evaluating files (important for JS configs with side effects).
		// Note: in a diamond (A→B→D, A→C→D), D's ignores/overrides still
		// appear via both B and C since each branch inherits independently.
		if cached, ok := c.cache[abs]; ok {
			mergeInto(merged, cached)
			continue
		}

		// Add to ancestor chain before recursing, remove after — this tracks
		// the current path from root to leaf, not all previously seen nodes.
		ancestors[abs] = struct{}{}

		base, err := LoadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: extends %q: %w", ext, err)
		}

		base, err = c.resolve(base, filepath.Dir(abs), ancestors)
		if err != nil {
			return nil, err
		}

		delete(ancestors, abs)

		c.cache[abs] = base
		mergeInto(merged, base)
	}

	// Current config wins — applied last.
	mergeInto(merged, cfg)

	// Clear Extends on the final merged config.
	merged.Extends = nil

	return merged, nil
}

// resolveExtendPath resolves an extends string to a file path.
// Relative paths (starting with ".", containing a separator, or having a
// file extension) are resolved from baseDir. Absolute paths are used directly.
// Bare identifiers like "@org/rules" are not supported in v0.1.
func resolveExtendPath(ext, baseDir string) (string, error) {
	if filepath.IsAbs(ext) {
		return ext, nil
	}
	if isFilePath(ext) {
		return filepath.Join(baseDir, ext), nil
	}
	return "", fmt.Errorf("config: named presets not yet supported, use file paths (got %q)", ext)
}

// isFilePath returns true if ext looks like a file path rather than a named
// preset. Named presets start with "@" (scoped packages) or contain no path
// separator and no file extension.
func isFilePath(ext string) bool {
	if strings.HasPrefix(ext, "@") {
		return false
	}
	if strings.HasPrefix(ext, ".") {
		return true
	}
	if strings.ContainsRune(ext, filepath.Separator) || strings.ContainsRune(ext, '/') {
		return true
	}
	switch filepath.Ext(ext) {
	case ".json", ".yaml", ".yml", ".toml", ".js":
		return true
	}
	return false
}

// mergeInto merges src into dst. Rules from src override existing rules in dst.
// Ignores and Overrides are concatenated.
func mergeInto(dst, src *Config) {
	for name := range src.Rules {
		dst.Rules[name] = src.Rules[name]
	}
	dst.Ignores = append(dst.Ignores, src.Ignores...)
	dst.Overrides = append(dst.Overrides, src.Overrides...)
}
