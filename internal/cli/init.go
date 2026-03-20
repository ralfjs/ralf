package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
	"github.com/Hideart/ralf/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func initCmd() *cobra.Command {
	var (
		fromESLint bool
		fromBiome  bool
		force      bool
		outFormat  string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate a ralf config file",
		Long: `Generate a .ralfrc config file with all 61 built-in rules.
Use --from-eslint or --from-biome to migrate from an existing config.
Migration preserves source severities and lists unsupported rules.`,
		Example: `  ralf init
  ralf init --from-eslint
  ralf init --from-biome --format yaml
  ralf init --force`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, fromESLint, fromBiome, force, outFormat)
		},
	}

	cmd.Flags().BoolVar(&fromESLint, "from-eslint", false, "migrate from ESLint config")
	cmd.Flags().BoolVar(&fromBiome, "from-biome", false, "migrate from Biome config")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config")
	cmd.Flags().StringVar(&outFormat, "format", "json", "output format (json|yaml|toml)")

	return cmd
}

func runInit(cmd *cobra.Command, fromESLint, fromBiome, force bool, outFormat string) error {
	w := cmd.ErrOrStderr()

	if fromESLint && fromBiome {
		_, _ = fmt.Fprintln(w, "Error: --from-eslint and --from-biome are mutually exclusive")
		exitCode = ExitUsageError
		return nil
	}

	ext, ok := formatExtension(outFormat)
	if !ok {
		_, _ = fmt.Fprintf(w, "Error: unsupported format %q (must be json, yaml, or toml)\n", outFormat)
		exitCode = ExitUsageError
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error:", err)
		exitCode = ExitInternal
		return nil
	}

	outFile := filepath.Join(cwd, ".ralfrc"+ext)

	if existing, found := configFileExists(cwd); found {
		if !force {
			_, _ = fmt.Fprintf(w, "Error: config file already exists: %s (use --force to overwrite)\n", existing)
			exitCode = ExitUsageError
			return nil
		}
		// Remove all existing ralf config files so the newly generated
		// file is always the one that config.Load discovers.
		for _, name := range config.SearchNames {
			if err := os.Remove(filepath.Join(cwd, name)); err != nil && !os.IsNotExist(err) {
				_, _ = fmt.Fprintf(w, "Error: remove %s: %v\n", name, err)
				exitCode = ExitInternal
				return nil
			}
		}
	}

	var (
		cfg    *config.Config
		report *migrationReport
	)

	switch {
	case fromESLint:
		cfg, report, err = migrateESLint(cwd)
	case fromBiome:
		cfg, report, err = migrateBiome(cwd)
	default:
		cfg = config.RecommendedConfig()
	}
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error:", err)
		exitCode = ExitUsageError
		return nil
	}

	data, err := serializeConfig(cfg, outFormat)
	if err != nil {
		_, _ = fmt.Fprintln(w, "Error:", err)
		exitCode = ExitInternal
		return nil
	}

	if err := os.WriteFile(outFile, data, 0o644); err != nil { //nolint:gosec // config files are world-readable
		_, _ = fmt.Fprintf(w, "Error: write %s: %v\n", outFile, err)
		exitCode = ExitInternal
		return nil
	}

	if report != nil {
		printMigrationReport(w, report)
	}

	_, _ = fmt.Fprintf(w, "Created %s with %d rules\n", filepath.Base(outFile), len(cfg.Rules))
	return nil
}

// configFileExists checks if any ralf config file exists in dir.
func configFileExists(dir string) (string, bool) {
	for _, name := range config.SearchNames {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return name, true
		}
	}
	return "", false
}

func formatExtension(format string) (string, bool) {
	switch format {
	case "json":
		return ".json", true
	case "yaml":
		return ".yaml", true
	case "toml":
		return ".toml", true
	default:
		return "", false
	}
}

func serializeConfig(cfg *config.Config, format string) ([]byte, error) {
	switch format {
	case "json":
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal JSON: %w", err)
		}
		return append(data, '\n'), nil
	case "yaml":
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("marshal YAML: %w", err)
		}
		return data, nil
	case "toml":
		var buf bytes.Buffer
		enc := toml.NewEncoder(&buf)
		if err := enc.Encode(cfg); err != nil {
			return nil, fmt.Errorf("marshal TOML: %w", err)
		}
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

// parseSeverityString maps "off"/"warn"/"error" to config.Severity.
// Shared by ESLint and Biome migration.
func parseSeverityString(s string) (config.Severity, bool) {
	switch s {
	case "off":
		return config.SeverityOff, true
	case "warn":
		return config.SeverityWarn, true
	case "error":
		return config.SeverityError, true
	}
	return "", false
}

// migrationReport summarizes a config migration.
type migrationReport struct {
	source           string
	sourceFile       string
	migratedCount    int
	unsupportedRules []string
	ignoreCount      int
}

func printMigrationReport(w io.Writer, r *migrationReport) {
	_, _ = fmt.Fprintf(w, "Migrated %d rules from %s (%s)\n", r.migratedCount, r.source, r.sourceFile)
	if len(r.unsupportedRules) > 0 {
		sort.Strings(r.unsupportedRules)
		_, _ = fmt.Fprintf(w, "Skipped %d unsupported rules:\n", len(r.unsupportedRules))
		for _, name := range r.unsupportedRules {
			_, _ = fmt.Fprintf(w, "  %s\n", name)
		}
	}
	if r.ignoreCount > 0 {
		_, _ = fmt.Fprintf(w, "Migrated %d ignore patterns\n", r.ignoreCount)
	}
}
