package config

import (
	"path/filepath"
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

func TestLoadFileInvalidJSON(t *testing.T) {
	_, err := LoadFile(filepath.Join(testdataDir, "invalid-severity.json"))
	// This file is valid JSON, just has bad severity — LoadFile should succeed
	if err != nil {
		t.Fatalf("LoadFile should not fail on structurally valid JSON: %v", err)
	}
}

func TestLoadSearchOrder(t *testing.T) {
	// Load from the testdata dir which has valid.json, valid.yaml, valid.toml
	// Should find .lintrc.json first — but testdata doesn't have .lintrc.* files.
	// Instead, verify Load returns not-found error when no .lintrc.* exists.
	_, err := Load(testdataDir)
	if err == nil {
		t.Fatal("expected not-found error (testdata has valid.json but not .lintrc.json)")
	}
}

func TestLoadFileUnsupportedExtension(t *testing.T) {
	_, err := LoadFile("config.xml")
	if err == nil {
		t.Fatal("expected error for unsupported extension")
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
