// Package cli implements the ralf command-line interface using cobra.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Hideart/ralf/internal/version"
	"github.com/spf13/cobra"
)

// Exit codes.
const (
	ExitOK         = 0
	ExitLintErrors = 1
	ExitUsageError = 2
	ExitInternal   = 3
)

var configPath string

// Execute builds the command tree, runs it, and returns an exit code.
// It does NOT call os.Exit — the caller is responsible.
func Execute() int {
	// Reset global state so Execute is safe to call more than once
	// (e.g. in tests).
	exitCode = 0
	configPath = ""
	cachedCwd, _ = filepath.Abs(".")

	root := newRootCmd()
	if err := root.Execute(); err != nil {
		// cobra already printed the error; just return code.
		return ExitUsageError
	}

	return exitCode
}

// exitCode is set by subcommands to override the default 0.
var exitCode int

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ralf",
		Short: "Fast, project-aware JS/TS linter",
		Long: `ralf — fast, project-aware JS/TS linter written in Go.

61 built-in rules (ESLint/Biome compatible). Zero config required.
Supports .ralfrc.json, .ralfrc.yaml, .ralfrc.toml, and .ralfrc.js configs.

Exit codes:
  0  No lint errors
  1  Lint errors found (or warnings exceeded --max-warnings)
  2  Config or usage error
  3  Internal error

Documentation: https://github.com/Hideart/ralf`,
		Example: `  ralf lint                          # lint cwd with auto-discovered config
  ralf lint src/ tests/              # lint specific paths
  ralf lint --format sarif src/      # SARIF output for CI
  ralf init                          # generate .ralfrc.json with all rules
  ralf init --from-eslint            # migrate from ESLint config
  ralf init --from-biome --format yaml  # migrate from Biome to YAML`,
		Version: version.Version,
		// Silence cobra's default error/usage printing on RunE errors
		// so we control output.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetVersionTemplate(fmt.Sprintf("ralf %s\n", version.Version))
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	cmd.PersistentFlags().StringVar(&configPath, "config", "", "explicit config file path")

	cmd.AddCommand(lintCmd())
	cmd.AddCommand(initCmd())

	return cmd
}
