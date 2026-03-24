package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
)

func TestInit_DefaultConfig(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	code := Execute()
	if code != ExitOK {
		t.Fatalf("expected exit 0 for init, got %d", code)
	}

	path := filepath.Join(dir, ".ralfrc.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected .ralfrc.json to exist: %v", err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("generated config not loadable: %v", err)
	}
	if len(cfg.Rules) != 61 {
		t.Errorf("expected 61 rules, got %d", len(cfg.Rules))
	}
}

func TestInit_FormatYAML(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.Args = []string{"ralf", "init", "--format", "yaml"}
	code := Execute()
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}

	path := filepath.Join(dir, ".ralfrc.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected .ralfrc.yaml to exist: %v", err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("generated YAML config not loadable: %v", err)
	}
	if len(cfg.Rules) != 61 {
		t.Errorf("expected 61 rules, got %d", len(cfg.Rules))
	}
}

func TestInit_FormatTOML(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.Args = []string{"ralf", "init", "--format", "toml"}
	code := Execute()
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}

	path := filepath.Join(dir, ".ralfrc.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected .ralfrc.toml to exist: %v", err)
	}

	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("generated TOML config not loadable: %v", err)
	}
	if len(cfg.Rules) != 61 {
		t.Errorf("expected 61 rules, got %d", len(cfg.Rules))
	}
}

func TestInit_ExistingConfigError(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Create existing config.
	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.json"), []byte(`{"rules":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"ralf", "init"}
	code := Execute()
	if code != ExitUsageError {
		t.Fatalf("expected exit %d for existing config, got %d", ExitUsageError, code)
	}
}

func TestInit_Force(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.json"), []byte(`{"rules":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"ralf", "init", "--force"}
	code := Execute()
	if code != ExitOK {
		t.Fatalf("expected exit 0 with --force, got %d", code)
	}

	cfg, err := config.LoadFile(filepath.Join(dir, ".ralfrc.json"))
	if err != nil {
		t.Fatalf("config not loadable after force: %v", err)
	}
	if len(cfg.Rules) != 61 {
		t.Errorf("expected 61 rules, got %d", len(cfg.Rules))
	}
}

func TestInit_ForceMultipleConfigs(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// Create multiple config files.
	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.json"), []byte(`{"rules":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.yaml"), []byte("rules: {}"), 0o600); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"ralf", "init", "--force", "--format", "toml"}
	code := Execute()
	if code != ExitOK {
		t.Fatalf("expected exit 0, got %d", code)
	}

	// Old configs should be removed.
	if _, err := os.Stat(filepath.Join(dir, ".ralfrc.json")); !os.IsNotExist(err) {
		t.Error("expected .ralfrc.json to be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, ".ralfrc.yaml")); !os.IsNotExist(err) {
		t.Error("expected .ralfrc.yaml to be removed")
	}
	// New config should exist.
	if _, err := os.Stat(filepath.Join(dir, ".ralfrc.toml")); err != nil {
		t.Fatalf("expected .ralfrc.toml to exist: %v", err)
	}
}

func TestInit_MutualExclusion(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.Args = []string{"ralf", "init", "--from-eslint", "--from-biome"}
	code := Execute()
	if code != ExitUsageError {
		t.Fatalf("expected exit %d for mutual exclusion, got %d", ExitUsageError, code)
	}
}

func TestInit_InvalidFormat(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	os.Args = []string{"ralf", "init", "--format", "xml"}
	code := Execute()
	if code != ExitUsageError {
		t.Fatalf("expected exit %d for invalid format, got %d", ExitUsageError, code)
	}
}

func TestInit_ConfigFileExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.yaml"), []byte("rules: {}"), 0o600); err != nil {
		t.Fatal(err)
	}

	name, found := configFileExists(dir)
	if !found {
		t.Fatal("expected config to be found")
	}
	if !strings.Contains(name, ".ralfrc") {
		t.Errorf("expected ralfrc filename, got %q", name)
	}
}

// chdir changes to dir for the test and sets os.Args for Execute().
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	os.Args = []string{"ralf", "init"}
}
