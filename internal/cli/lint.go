package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/project"
	"github.com/spf13/cobra"
)

func lintCmd() *cobra.Command {
	var (
		format      string
		threads     int
		maxWarnings int
		fix         bool
		fixDryRun   bool
		noCache     bool
	)

	cmd := &cobra.Command{
		Use:   "lint [paths...]",
		Short: "Lint files for rule violations",
		Long:  "Lint JavaScript and TypeScript files using rules defined in .ralfrc config.\nWith no paths, lints the current directory recursively.",
		Example: `  ralf lint
  ralf lint src/ tests/
  ralf lint --format json src/
  ralf lint --format sarif src/ > results.sarif
  ralf lint --fix src/
  ralf lint --max-warnings 0 src/`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd, args, format, threads, maxWarnings, fix, fixDryRun, noCache)
		},
	}

	cmd.Flags().StringVar(&format, "format", "stylish", "output format (stylish|json|compact|github|sarif)")
	cmd.Flags().IntVar(&threads, "threads", 0, "number of parallel workers (0 = num CPUs)")
	cmd.Flags().IntVar(&maxWarnings, "max-warnings", -1, "max warnings before non-zero exit (-1 = unlimited)")
	cmd.Flags().BoolVar(&fix, "fix", false, "apply auto-fixes and write files back")
	cmd.Flags().BoolVar(&fixDryRun, "fix-dry-run", false, "show unified diffs without writing")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "disable result caching")

	return cmd
}

func runLint(cmd *cobra.Command, args []string, format string, threads, maxWarnings int, fix, fixDryRun, noCache bool) error {
	w := cmd.ErrOrStderr()

	if fix && fixDryRun {
		_, _ = fmt.Fprintln(w, "Error: --fix and --fix-dry-run are mutually exclusive")
		exitCode = ExitUsageError
		return nil
	}

	// Validate format early so the user gets fast feedback on typos
	// before waiting for config loading and linting.
	formatter, err := newFormatter(format)
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error:", err)
		exitCode = ExitUsageError
		return nil
	}

	cfg, err := loadConfig()
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error:", err)
		exitCode = ExitUsageError
		return nil
	}

	if err := config.Validate(cfg); err != nil {
		_, _ = fmt.Fprintln(w, "Config validation error:", err)
		exitCode = ExitUsageError
		return nil
	}

	eng, errs := engine.New(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			_, _ = fmt.Fprintln(w, "Rule compile error:", e)
		}
		exitCode = ExitUsageError
		return nil
	}

	paths := args
	if len(paths) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			_, _ = fmt.Fprintln(w, "Error:", err)
			exitCode = ExitInternal
			return nil
		}
		paths = []string{cwd}
	}

	files, err := discoverFiles(paths, cfg.Ignores)
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error discovering files:", err)
		exitCode = ExitUsageError
		return nil
	}

	if len(files) == 0 {
		return nil
	}

	result := lintWithCache(cmd, eng, cfg, files, threads, noCache, fix || fixDryRun)

	for _, fe := range result.Errors {
		_, _ = fmt.Fprintf(w, "Error reading %s: %v\n", fe.File, fe.Err)
	}
	if len(result.Errors) > 0 {
		exitCode = ExitInternal
	}

	// Apply fixes if requested.
	if fix || fixDryRun {
		fixCount, conflictCount, applied, fixErrors := applyFixResults(cmd, result.Diagnostics, fixDryRun)
		if fixErrors {
			exitCode = ExitInternal
		}

		// In --fix mode, remove diagnostics whose fixes were actually applied.
		// In --fix-dry-run mode, keep all diagnostics (nothing was written).
		if fix && len(applied) > 0 {
			var remaining []engine.Diagnostic
			for _, d := range result.Diagnostics {
				if d.Fix == nil || !applied[fixKey(d.File, *d.Fix)] {
					remaining = append(remaining, d)
				}
			}
			result.Diagnostics = remaining
		}

		if fixCount > 0 || conflictCount > 0 {
			verb := "Fixed"
			if fixDryRun {
				verb = "Would fix"
			}
			_, _ = fmt.Fprintf(w, "%s %d %s", verb, fixCount, pluralize("problem", fixCount))
			if conflictCount > 0 {
				_, _ = fmt.Fprintf(w, " (%d conflicting %s skipped)",
					conflictCount, pluralize("fix", conflictCount))
			}
			_, _ = fmt.Fprintln(w)
		}
	}

	if err := formatter.Format(cmd.OutOrStdout(), result.Diagnostics); err != nil {
		_, _ = fmt.Fprintln(w, "Error formatting output:", err)
		exitCode = ExitInternal
		return nil
	}

	errCount, warnCount := countBySeverity(result.Diagnostics)

	if errCount > 0 {
		exitCode = ExitLintErrors
		return nil
	}

	if maxWarnings >= 0 && warnCount > maxWarnings {
		_, _ = fmt.Fprintf(w, "Too many warnings (%d), max allowed is %d\n",
			warnCount, maxWarnings)
		exitCode = ExitLintErrors
		return nil
	}

	return nil
}

// fixID uniquely identifies a fix by file, byte range, and replacement text.
type fixID struct {
	file       string
	start, end int
	text       string
}

func fixKey(file string, f engine.Fix) fixID {
	return fixID{file: file, start: f.StartByte, end: f.EndByte, text: f.NewText}
}

// applyFixResults groups fixable diagnostics by file, applies fixes, and
// either writes the files back (--fix) or prints unified diffs (--fix-dry-run).
// Returns the number of fixes applied, conflicts encountered, a set of
// applied fix keys (empty in dry-run mode), and whether any I/O errors occurred.
func applyFixResults(cmd *cobra.Command, diags []engine.Diagnostic, dryRun bool) (fixCount, conflictCount int, applied map[fixID]bool, hadErrors bool) {
	w := cmd.ErrOrStderr()
	out := cmd.OutOrStdout()
	applied = make(map[fixID]bool)

	// Group fixable diagnostics by file.
	byFile := make(map[string][]engine.Fix)
	for _, d := range diags {
		if d.Fix != nil {
			byFile[d.File] = append(byFile[d.File], *d.Fix)
		}
	}

	// Iterate files in sorted order for deterministic output.
	filePaths := make([]string, 0, len(byFile))
	for fp := range byFile {
		filePaths = append(filePaths, fp)
	}
	sort.Strings(filePaths)

	for _, filePath := range filePaths {
		fixes := byFile[filePath]
		source, err := os.ReadFile(filePath) //nolint:gosec // paths from discoverFiles
		if err != nil {
			_, _ = fmt.Fprintf(w, "Error reading %s for fix: %v\n", filePath, err)
			hadErrors = true
			continue
		}

		result, conflicts := engine.ApplyFixes(source, fixes)
		nonConflicting := len(fixes) - len(conflicts)
		conflictCount += len(conflicts)

		for _, c := range conflicts {
			slog.Debug("fix conflict", "file", filePath, "reason", c.Reason)
		}

		if dryRun {
			diff := unifiedDiff(filePath, string(source), string(result))
			if diff != "" {
				_, _ = fmt.Fprintln(out, diff)
			}
			fixCount += nonConflicting
			continue
		}

		if err := atomicWrite(filePath, result); err != nil {
			_, _ = fmt.Fprintf(w, "Error writing %s: %v\n", filePath, err)
			hadErrors = true
			continue
		}

		// Count and mark only successfully written fixes.
		// Use occurrence counting: if a fixID appears N times in fixes
		// and M times in conflicts, it was accepted (N-M) times.
		// This correctly handles duplicate identical fixes where the
		// first is accepted and the second is a conflict.
		fixCount += nonConflicting
		counts := make(map[fixID]int, len(fixes))
		for _, f := range fixes {
			counts[fixKey(filePath, f)]++
		}
		for _, c := range conflicts {
			counts[fixKey(filePath, c.Fix)]--
		}
		for k, n := range counts {
			if n > 0 {
				applied[k] = true
			}
		}
	}

	return fixCount, conflictCount, applied, hadErrors
}

// atomicWrite writes data to a temporary file in the same directory and renames
// it over the target to avoid partial writes.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ralf-fix-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Preserve original file permissions.
	info, err := os.Stat(path)
	if err == nil {
		if chErr := tmp.Chmod(info.Mode()); chErr != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			return fmt.Errorf("chmod temp file: %w", chErr)
		}
	}

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

// unifiedDiff produces a minimal unified diff between two strings.
// Returns empty string if there is no difference.
func unifiedDiff(path, a, b string) string {
	if a == b {
		return ""
	}

	aLines := splitLines(a)
	bLines := splitLines(b)

	var buf strings.Builder
	fmt.Fprintf(&buf, "--- %s\n+++ %s\n", path, path)

	// Simple line-by-line diff — sufficient for fix preview.
	ai, bi := 0, 0
	for ai < len(aLines) || bi < len(bLines) {
		if ai < len(aLines) && bi < len(bLines) && aLines[ai] == bLines[bi] {
			ai++
			bi++
			continue
		}

		// Find the extent of the change.
		aStart, bStart := ai, bi
		// Advance past differing lines.
		for ai < len(aLines) && (bi >= len(bLines) || aLines[ai] != bLines[bi]) {
			ai++
		}
		for bi < len(bLines) && (ai >= len(aLines) || aLines[ai] != bLines[bi]) {
			bi++
		}

		fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", aStart+1, ai-aStart, bStart+1, bi-bStart)
		for i := aStart; i < ai; i++ {
			buf.WriteByte('-')
			buf.WriteString(aLines[i])
			buf.WriteByte('\n')
		}
		for i := bStart; i < bi; i++ {
			buf.WriteByte('+')
			buf.WriteString(bLines[i])
			buf.WriteByte('\n')
		}
	}

	return buf.String()
}

// splitLines splits a string into lines. A trailing newline produces a
// final empty string element so that diffs correctly represent EOF changes.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	// Always append the remainder — if s ends with '\n', this appends ""
	// so the diff can distinguish "foo\n" from "foo".
	lines = append(lines, s[start:])
	return lines
}

// lintWithCache runs the lint pipeline with optional caching.
// When caching is enabled, files are read once, hashed, and looked up in the cache.
// Cache misses are linted via LintSources, and results are stored back.
func lintWithCache(cmd *cobra.Command, eng *engine.Engine, cfg *config.Config, files []string, threads int, noCache, isFixMode bool) *engine.Result {
	ctx := cmd.Context()

	// Determine whether to use cache.
	useCache := !noCache && !isFixMode
	var cache *project.Cache
	if useCache {
		configHash, err := project.HashConfig(cfg)
		if err != nil {
			slog.Debug("cache disabled: config hash failed", "error", err)
		} else {
			c, err := project.Open(ctx, projectRootDir(), configHash)
			if err != nil {
				slog.Debug("cache disabled: open failed", "error", err)
			} else {
				cache = c
				defer func() { _ = cache.Close() }()
			}
		}
	}

	// If no cache, fall back to the standard path.
	if cache == nil {
		return eng.Lint(ctx, files, threads)
	}

	// Read files, hash, partition into cache hits vs misses.
	type readFile struct {
		path string
		hash uint64
	}
	var (
		cachedDiags []engine.Diagnostic
		toLint      []engine.FileSource
		readFiles   []readFile
		readErrors  []engine.FileError
	)

	for _, filePath := range files {
		if ctx.Err() != nil {
			break
		}
		source, err := os.ReadFile(filePath) //nolint:gosec // paths from discoverFiles
		if err != nil {
			readErrors = append(readErrors, engine.FileError{File: filePath, Err: err})
			continue
		}

		h := project.HashFile(source)

		diags, hit, err := cache.Lookup(ctx, filePath, h)
		if err != nil {
			slog.Debug("cache lookup error, linting file", "file", filePath, "error", err)
		}
		if hit {
			cachedDiags = append(cachedDiags, diags...)
			continue
		}

		toLint = append(toLint, engine.FileSource{Path: filePath, Source: source})
		readFiles = append(readFiles, readFile{path: filePath, hash: h})
	}

	// Lint cache-miss files.
	var result *engine.Result
	if len(toLint) > 0 {
		result = eng.LintSources(ctx, toLint, threads)
	} else {
		result = &engine.Result{}
	}
	result.Errors = append(result.Errors, readErrors...)

	// Store fresh results in cache.
	if len(readFiles) > 0 {
		freshByFile := make(map[string][]engine.Diagnostic, len(readFiles))
		for _, d := range result.Diagnostics {
			freshByFile[d.File] = append(freshByFile[d.File], d)
		}

		entries := make([]project.CacheEntry, len(readFiles))
		for i, rf := range readFiles {
			var modTimeNS int64
			if fi, err := os.Stat(rf.path); err == nil {
				modTimeNS = fi.ModTime().UnixNano()
			}
			entries[i] = project.CacheEntry{
				Path:        rf.path,
				ContentHash: rf.hash,
				ModTimeNS:   modTimeNS,
				Diagnostics: freshByFile[rf.path],
			}
		}
		if err := cache.StoreBatch(ctx, entries); err != nil {
			slog.Debug("cache store failed", "error", err)
		}
	}

	// Merge cached + fresh diagnostics.
	if len(cachedDiags) > 0 {
		all := make([]engine.Diagnostic, 0, len(cachedDiags)+len(result.Diagnostics))
		all = append(all, cachedDiags...)
		all = append(all, result.Diagnostics...)
		result.Diagnostics = all
		engine.SortDiagChunksByFile(result.Diagnostics)
	}

	slog.Debug("lint cache summary",
		"total", len(files),
		"cached", len(files)-len(toLint)-len(readErrors),
		"linted", len(toLint),
		"errors", len(readErrors))

	return result
}

// projectRootDir returns the project root for cache storage.
// Derives at call time to respect working directory changes (e.g. in tests).
func projectRootDir() string {
	if configPath != "" {
		return filepath.Dir(configPath)
	}
	dir, err := os.Getwd()
	if err != nil {
		slog.Debug("failed to get working directory for project root", "error", err)
		return cachedCwd
	}
	return dir
}

func loadConfig() (*config.Config, error) {
	var (
		cfg *config.Config
		dir string
		err error
	)

	if configPath != "" {
		cfg, err = config.LoadFile(configPath)
		if err != nil {
			return nil, err
		}
		dir = filepath.Dir(configPath)
	} else {
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		cfg, err = config.Load(dir)
		if err != nil {
			if errors.Is(err, config.ErrNoConfig) {
				slog.Debug("no config file found, using recommended built-in rules")
				return config.RecommendedConfig(), nil
			}
			return nil, err
		}
	}

	cfg, err = config.ResolveExtends(cfg, dir)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

// countBySeverity returns the number of error and warning diagnostics in a
// single pass.
func countBySeverity(diags []engine.Diagnostic) (errCount, warnCount int) {
	for _, d := range diags {
		switch d.Severity {
		case config.SeverityError:
			errCount++
		case config.SeverityWarn:
			warnCount++
		}
	}
	return errCount, warnCount
}
