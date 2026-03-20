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
		Use:     "ralf",
		Short:   "Fast, project-aware JS/TS linter",
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
