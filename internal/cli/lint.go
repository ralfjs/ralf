package cli

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/engine"
	"github.com/spf13/cobra"
)

func lintCmd() *cobra.Command {
	var (
		format      string
		threads     int
		maxWarnings int
		fix         bool
		fixDryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "lint [paths...]",
		Short: "Lint files for rule violations",
		Long:  "Lint JavaScript and TypeScript files using rules defined in config.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd, args, format, threads, maxWarnings, fix, fixDryRun)
		},
	}

	cmd.Flags().StringVar(&format, "format", "stylish", "output format (stylish|json|compact|github)")
	cmd.Flags().IntVar(&threads, "threads", 0, "number of parallel workers (0 = num CPUs)")
	cmd.Flags().IntVar(&maxWarnings, "max-warnings", -1, "max warnings before non-zero exit (-1 = unlimited)")
	cmd.Flags().BoolVar(&fix, "fix", false, "apply auto-fixes and write files back")
	cmd.Flags().BoolVar(&fixDryRun, "fix-dry-run", false, "show unified diffs without writing")

	return cmd
}

func runLint(cmd *cobra.Command, args []string, format string, threads, maxWarnings int, fix, fixDryRun bool) error {
	w := cmd.ErrOrStderr()

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

	result := eng.Lint(cmd.Context(), files, threads)

	for _, fe := range result.Errors {
		_, _ = fmt.Fprintf(w, "Error reading %s: %v\n", fe.File, fe.Err)
	}
	if len(result.Errors) > 0 {
		exitCode = ExitInternal
	}

	// Apply fixes if requested.
	if fix || fixDryRun {
		fixCount, conflictCount := applyFixResults(cmd, result.Diagnostics, fixDryRun)

		// Remove fixed diagnostics from output.
		var unfixed []engine.Diagnostic
		for _, d := range result.Diagnostics {
			if d.Fix == nil {
				unfixed = append(unfixed, d)
			}
		}

		if fixCount > 0 || conflictCount > 0 {
			_, _ = fmt.Fprintf(w, "Fixed %d %s", fixCount, pluralize("problem", fixCount))
			if conflictCount > 0 {
				_, _ = fmt.Fprintf(w, " (%d conflicting %s skipped)",
					conflictCount, pluralize("fix", conflictCount))
			}
			_, _ = fmt.Fprintln(w)
		}

		result.Diagnostics = unfixed
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

// applyFixResults groups fixable diagnostics by file, applies fixes, and
// either writes the files back (--fix) or prints unified diffs (--fix-dry-run).
// Returns the number of fixes applied and conflicts encountered.
func applyFixResults(cmd *cobra.Command, diags []engine.Diagnostic, dryRun bool) (fixCount, conflictCount int) {
	w := cmd.ErrOrStderr()
	out := cmd.OutOrStdout()

	// Group fixable diagnostics by file.
	byFile := make(map[string][]engine.Fix)
	for _, d := range diags {
		if d.Fix != nil {
			byFile[d.File] = append(byFile[d.File], *d.Fix)
		}
	}

	for filePath, fixes := range byFile {
		source, err := os.ReadFile(filePath) //nolint:gosec // paths from discoverFiles
		if err != nil {
			_, _ = fmt.Fprintf(w, "Error reading %s for fix: %v\n", filePath, err)
			continue
		}

		result, conflicts := engine.ApplyFixes(source, fixes)
		fixCount += len(fixes) - len(conflicts)
		conflictCount += len(conflicts)

		for _, c := range conflicts {
			slog.Debug("fix conflict", "file", filePath, "reason", c.Reason)
		}

		if dryRun {
			diff := unifiedDiff(filePath, string(source), string(result))
			if diff != "" {
				_, _ = fmt.Fprintln(out, diff)
			}
			continue
		}

		if err := atomicWrite(filePath, result); err != nil {
			_, _ = fmt.Fprintf(w, "Error writing %s: %v\n", filePath, err)
		}
	}

	return fixCount, conflictCount
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

	var result string
	result += fmt.Sprintf("--- %s\n+++ %s\n", path, path)

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

		result += fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", aStart+1, ai-aStart, bStart+1, bi-bStart)
		for i := aStart; i < ai; i++ {
			result += "-" + aLines[i] + "\n"
		}
		for i := bStart; i < bi; i++ {
			result += "+" + bLines[i] + "\n"
		}
	}

	return result
}

// splitLines splits a string into lines, preserving empty trailing line.
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
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.LoadFile(configPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			slog.Debug("no config file found, using recommended built-in rules")
			return config.RecommendedConfig(), nil
		}
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
