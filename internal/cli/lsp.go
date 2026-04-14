package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/crossfile"
	"github.com/ralfjs/ralf/internal/engine"
	"github.com/ralfjs/ralf/internal/lsp"
	"github.com/ralfjs/ralf/internal/project"
	"github.com/spf13/cobra"
)

func lspCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lsp",
		Short: "Start Language Server Protocol server",
		Long:  "Start the ralf LSP server over stdio (JSON-RPC 2.0).\nEditors start this command automatically.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runLSP(cmd)
		},
	}

	return cmd
}

func runLSP(cmd *cobra.Command) error {
	// Send slog output to stderr — stdout is the JSON-RPC transport.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelWarn,
	})))

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

	var graph *project.Graph
	if crossfile.HasActiveRules(cfg) {
		graph = project.NewGraph(
			make(map[string][]project.ExportInfo),
			make(map[string][]project.ImportInfo),
		)
	}

	srv := lsp.NewServer(eng, cfg, graph)

	slog.Info("ralf LSP server starting")

	if err := srv.Run(cmd.Context(), os.Stdin, os.Stdout); err != nil {
		slog.Error("LSP server error", "error", err)
		exitCode = ExitInternal
		return nil
	}

	// Map LSP exit codes to CLI exit codes:
	// 0 (clean shutdown) → ExitOK, 1 (exit without shutdown) → ExitInternal.
	if code := srv.ExitCode(); code > 0 {
		exitCode = ExitInternal
	}

	return nil
}
