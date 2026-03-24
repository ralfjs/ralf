package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testdataDir = "../../testdata/config"

func TestLoadFileJSON(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "valid.json"))
	if err != nil {
		t.Fatalf("LoadFile JSON: %v", err)
	}
	assertValidConfig(t, cfg)
}

func TestLoadFileYAML(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "valid.yaml"))
	if err != nil {
		t.Fatalf("LoadFile YAML: %v", err)
	}
	assertValidConfig(t, cfg)
}

func TestLoadFileTOML(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "valid.toml"))
	if err != nil {
		t.Fatalf("LoadFile TOML: %v", err)
	}
	assertValidConfig(t, cfg)
}

func TestLoadFileMinimal(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "minimal.json"))
	if err != nil {
		t.Fatalf("LoadFile minimal: %v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(cfg.Rules))
	}
	rule, ok := cfg.Rules["no-eval"]
	if !ok {
		t.Fatal("expected rule 'no-eval'")
	}
	if rule.Regex != `\beval\b` {
		t.Errorf("regex = %q, want %q", rule.Regex, `\beval\b`)
	}
}

func TestLoadNotFound(t *testing.T) {
	_, err := Load("/nonexistent/dir")
	if err == nil {
		t.Fatal("expected error for missing config dir")
	}
}

func TestLoadFileDoesNotValidateSemantics(t *testing.T) {
	// invalid-severity.json is valid JSON but has bad severity value.
	// LoadFile only parses — semantic validation is Validate's job.
	_, err := LoadFile(filepath.Join(testdataDir, "invalid-severity.json"))
	if err != nil {
		t.Fatalf("LoadFile should not fail on structurally valid JSON: %v", err)
	}
}

func TestLoadSearchPriority(t *testing.T) {
	dir := t.TempDir()

	// Create both .ralfrc.yaml and .ralfrc.json — JSON has higher priority
	yamlContent := []byte("rules:\n  yaml-rule:\n    regex: y\n    severity: warn\n")
	jsonContent := []byte(`{"rules":{"json-rule":{"regex":"j","severity":"error"}}}`)

	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.yaml"), yamlContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.json"), jsonContent, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// JSON is higher priority than YAML, so json-rule should be present
	if _, ok := cfg.Rules["json-rule"]; !ok {
		t.Error("expected json-rule (JSON has higher priority than YAML)")
	}
	if _, ok := cfg.Rules["yaml-rule"]; ok {
		t.Error("yaml-rule should not be present (JSON has higher priority)")
	}
}

func TestLoadNotFoundDir(t *testing.T) {
	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected not-found error for empty dir")
	}
	if !errors.Is(err, ErrNoConfig) {
		t.Errorf("expected ErrNoConfig, got: %v", err)
	}
}

func TestLoadSearchPriority_JSOverJSON(t *testing.T) {
	dir := t.TempDir()

	jsonContent := []byte(`{"rules":{"json-rule":{"regex":"j","severity":"error"}}}`)
	jsContent := []byte(`module.exports = {rules:{"js-rule":{regex:"j",severity:"error"}}};`)

	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.json"), jsonContent, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ralfrc.js"), jsContent, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, ok := cfg.Rules["js-rule"]; !ok {
		t.Error("expected js-rule (JS has higher priority than JSON)")
	}
	if _, ok := cfg.Rules["json-rule"]; ok {
		t.Error("json-rule should not be present (JS has higher priority)")
	}
}

func TestLoadFileUnsupportedExtension(t *testing.T) {
	// Create an actual file so os.ReadFile succeeds and we hit the extension check
	path := filepath.Join(t.TempDir(), "config.xml")
	if err := os.WriteFile(path, []byte("<config/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected unsupported extension error, got: %v", err)
	}
}

func assertValidConfig(t *testing.T, cfg *Config) {
	t.Helper()

	if len(cfg.Rules) != 3 {
		t.Errorf("expected 3 rules, got %d", len(cfg.Rules))
	}

	// Check regex rule
	if rule, ok := cfg.Rules["no-magic-timeouts"]; ok {
		if rule.Severity != SeverityWarn {
			t.Errorf("no-magic-timeouts severity = %q, want %q", rule.Severity, SeverityWarn)
		}
		if rule.Regex == "" {
			t.Error("no-magic-timeouts regex is empty")
		}
	} else {
		t.Error("missing rule 'no-magic-timeouts'")
	}

	// Check pattern rule
	if rule, ok := cfg.Rules["no-console-in-prod"]; ok {
		if rule.Severity != SeverityError {
			t.Errorf("no-console-in-prod severity = %q, want %q", rule.Severity, SeverityError)
		}
		if rule.Pattern == "" {
			t.Error("no-console-in-prod pattern is empty")
		}
		if rule.Fix != "// removed" {
			t.Errorf("no-console-in-prod fix = %q, want %q", rule.Fix, "// removed")
		}
	} else {
		t.Error("missing rule 'no-console-in-prod'")
	}

	// Check AST rule
	if rule, ok := cfg.Rules["require-error-boundary"]; ok {
		if rule.AST == nil {
			t.Error("require-error-boundary ast is nil")
		} else if rule.AST.Kind != "jsx_element" {
			t.Errorf("require-error-boundary ast.kind = %q, want %q", rule.AST.Kind, "jsx_element")
		}
	} else {
		t.Error("missing rule 'require-error-boundary'")
	}

	// Check ignores
	if len(cfg.Ignores) != 2 {
		t.Errorf("expected 2 ignores, got %d", len(cfg.Ignores))
	}

	// Check overrides
	if len(cfg.Overrides) != 1 {
		t.Errorf("expected 1 override, got %d", len(cfg.Overrides))
	}
}
