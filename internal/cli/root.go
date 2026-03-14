// Package cli implements the bepro command-line interface using cobra.
package cli

import (
	"fmt"
	"os"

	"github.com/Hideart/bepro/internal/version"
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
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		// cobra already printed the error; just return code.
		return ExitUsageError
	}

	// The lint subcommand sets exitCode via RunE.
	return exitCode
}

// exitCode is set by subcommands to override the default 0.
var exitCode int

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "bepro",
		Short:   "Fast, project-aware JS/TS linter",
		Version: version.Version,
		// Silence cobra's default error/usage printing on RunE errors
		// so we control output.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.SetVersionTemplate(fmt.Sprintf("bepro %s\n", version.Version))
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	cmd.PersistentFlags().StringVar(&configPath, "config", "", "explicit config file path")

	cmd.AddCommand(lintCmd())

	return cmd
}
