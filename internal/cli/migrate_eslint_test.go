package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

func TestMigrateESLint_BasicJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".eslintrc.json", `{
		"rules": {
			"no-var": "error",
			"no-console": "warn",
			"eqeqeq": "off"
		}
	}`)

	cfg, report, err := migrateESLint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.migratedCount != 3 {
		t.Errorf("expected 3 migrated, got %d", report.migratedCount)
	}
	if cfg.Rules["no-var"].Severity != config.SeverityError {
		t.Errorf("expected no-var error, got %s", cfg.Rules["no-var"].Severity)
	}
	if cfg.Rules["eqeqeq"].Severity != config.SeverityOff {
		t.Errorf("expected eqeqeq off, got %s", cfg.Rules["eqeqeq"].Severity)
	}
	// All 61 builtins should be present.
	if len(cfg.Rules) != 61 {
		t.Errorf("expected 61 rules, got %d", len(cfg.Rules))
	}
}

func TestMigrateESLint_NumericSeverity(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".eslintrc.json", `{
		"rules": {
			"no-var": 2,
			"no-console": 1,
			"eqeqeq": 0
		}
	}`)

	cfg, _, err := migrateESLint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules["no-var"].Severity != config.SeverityError {
		t.Errorf("expected error for 2, got %s", cfg.Rules["no-var"].Severity)
	}
	if cfg.Rules["no-console"].Severity != config.SeverityWarn {
		t.Errorf("expected warn for 1, got %s", cfg.Rules["no-console"].Severity)
	}
	if cfg.Rules["eqeqeq"].Severity != config.SeverityOff {
		t.Errorf("expected off for 0, got %s", cfg.Rules["eqeqeq"].Severity)
	}
}

func TestMigrateESLint_ArrayForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".eslintrc.json", `{
		"rules": {
			"no-console": ["warn", {"allow": ["error"]}],
			"eqeqeq": [2, "always"]
		}
	}`)

	cfg, _, err := migrateESLint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules["no-console"].Severity != config.SeverityWarn {
		t.Errorf("expected warn, got %s", cfg.Rules["no-console"].Severity)
	}
	if cfg.Rules["eqeqeq"].Severity != config.SeverityError {
		t.Errorf("expected error, got %s", cfg.Rules["eqeqeq"].Severity)
	}
}

func TestMigrateESLint_IgnorePatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".eslintrc.json", `{
		"rules": {},
		"ignorePatterns": ["dist/", "*.test.js"]
	}`)

	cfg, report, err := migrateESLint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Ignores) != 2 {
		t.Errorf("expected 2 ignores, got %d", len(cfg.Ignores))
	}
	if report.ignoreCount != 2 {
		t.Errorf("expected ignoreCount 2, got %d", report.ignoreCount)
	}
}

func TestMigrateESLint_UnmappedRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".eslintrc.json", `{
		"rules": {
			"no-var": "error",
			"@typescript-eslint/no-unused-vars": "error",
			"import/order": "warn"
		}
	}`)

	_, report, err := migrateESLint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.migratedCount != 1 {
		t.Errorf("expected 1 migrated, got %d", report.migratedCount)
	}
	if len(report.unsupportedRules) != 2 {
		t.Errorf("expected 2 unsupported, got %d", len(report.unsupportedRules))
	}
}

func TestMigrateESLint_YAMLConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".eslintrc.yaml", "rules:\n  no-var: error\n  no-console: warn\n")

	cfg, report, err := migrateESLint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.migratedCount != 2 {
		t.Errorf("expected 2 migrated, got %d", report.migratedCount)
	}
	if cfg.Rules["no-var"].Severity != config.SeverityError {
		t.Errorf("expected no-var error, got %s", cfg.Rules["no-var"].Severity)
	}
}

func TestMigrateESLint_JSConfigError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".eslintrc.js", "module.exports = {};")

	_, _, err := migrateESLint(dir)
	if err == nil {
		t.Fatal("expected error for JS config")
	}
}

func TestMigrateESLint_NoConfigFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := migrateESLint(dir)
	if err == nil {
		t.Fatal("expected error when no config found")
	}
}

func TestParseESLintSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  config.Severity
		ok    bool
	}{
		{`"off"`, config.SeverityOff, true},
		{`"warn"`, config.SeverityWarn, true},
		{`"error"`, config.SeverityError, true},
		{`0`, config.SeverityOff, true},
		{`1`, config.SeverityWarn, true},
		{`2`, config.SeverityError, true},
		{`["error", {}]`, config.SeverityError, true},
		{`[1, "always"]`, config.SeverityWarn, true},
		{`"invalid"`, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := parseESLintSeverity(json.RawMessage(tc.input))
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
