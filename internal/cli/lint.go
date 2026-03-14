package cli

import (
	"fmt"
	"os"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/engine"
	"github.com/spf13/cobra"
)

func lintCmd() *cobra.Command {
	var (
		format      string
		threads     int
		maxWarnings int
	)

	cmd := &cobra.Command{
		Use:   "lint [paths...]",
		Short: "Lint files for rule violations",
		Long:  "Lint JavaScript and TypeScript files using rules defined in config.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd, args, format, threads, maxWarnings)
		},
	}

	cmd.Flags().StringVar(&format, "format", "stylish", "output format (stylish|json|compact|github)")
	cmd.Flags().IntVar(&threads, "threads", 0, "number of parallel workers (0 = num CPUs)")
	cmd.Flags().IntVar(&maxWarnings, "max-warnings", -1, "max warnings before non-zero exit (-1 = unlimited)")

	return cmd
}

func runLint(cmd *cobra.Command, args []string, format string, threads, maxWarnings int) error {
	w := cmd.ErrOrStderr()

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

	formatter, err := newFormatter(format)
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error:", err)
		exitCode = ExitUsageError
		return nil
	}

	if err := formatter.Format(cmd.OutOrStdout(), result.Diagnostics); err != nil {
		_, _ = fmt.Fprintln(w, "Error formatting output:", err)
		exitCode = ExitInternal
		return nil
	}

	if hasErrors(result.Diagnostics) {
		exitCode = ExitLintErrors
		return nil
	}

	if maxWarnings >= 0 && countWarnings(result.Diagnostics) > maxWarnings {
		_, _ = fmt.Fprintf(w, "Too many warnings (%d), max allowed is %d\n",
			countWarnings(result.Diagnostics), maxWarnings)
		exitCode = ExitLintErrors
		return nil
	}

	return nil
}

func loadConfig() (*config.Config, error) {
	if configPath != "" {
		return config.LoadFile(configPath)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	return config.Load(cwd)
}

func hasErrors(diags []engine.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == config.SeverityError {
			return true
		}
	}
	return false
}

func countWarnings(diags []engine.Diagnostic) int {
	n := 0
	for _, d := range diags {
		if d.Severity == config.SeverityWarn {
			n++
		}
	}
	return n
}
