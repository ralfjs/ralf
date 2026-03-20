package cli

import (
	"encoding/json"
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

func TestMigrateBiome_BasicJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "biome.json", `{
		"linter": {
			"rules": {
				"suspicious": {
					"noDebugger": "error",
					"noDoubleEquals": "warn"
				},
				"style": {
					"noVar": "error"
				}
			}
		}
	}`)

	cfg, report, err := migrateBiome(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.migratedCount != 3 {
		t.Errorf("expected 3 migrated, got %d", report.migratedCount)
	}
	if cfg.Rules["no-debugger"].Severity != config.SeverityError {
		t.Errorf("expected no-debugger error, got %s", cfg.Rules["no-debugger"].Severity)
	}
	if cfg.Rules["eqeqeq"].Severity != config.SeverityWarn {
		t.Errorf("expected eqeqeq warn, got %s", cfg.Rules["eqeqeq"].Severity)
	}
	if len(cfg.Rules) != 61 {
		t.Errorf("expected 61 rules, got %d", len(cfg.Rules))
	}
}

func TestMigrateBiome_ObjectSeverity(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "biome.json", `{
		"linter": {
			"rules": {
				"suspicious": {
					"noDebugger": {"level": "warn"}
				}
			}
		}
	}`)

	cfg, _, err := migrateBiome(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Rules["no-debugger"].Severity != config.SeverityWarn {
		t.Errorf("expected warn, got %s", cfg.Rules["no-debugger"].Severity)
	}
}

func TestMigrateBiome_IgnorePatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "biome.json", `{
		"linter": {"rules": {}},
		"files": {"ignore": ["dist/**", "*.test.js"]}
	}`)

	cfg, report, err := migrateBiome(dir)
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

func TestMigrateBiome_UnmappedRules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "biome.json", `{
		"linter": {
			"rules": {
				"suspicious": {
					"noDebugger": "error",
					"noExplicitAny": "error"
				}
			}
		}
	}`)

	_, report, err := migrateBiome(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.migratedCount != 1 {
		t.Errorf("expected 1 migrated, got %d", report.migratedCount)
	}
	if len(report.unsupportedRules) != 1 {
		t.Errorf("expected 1 unsupported, got %d", len(report.unsupportedRules))
	}
}

func TestMigrateBiome_JSONC(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "biome.jsonc", `{
		// Enable linter
		"linter": {
			"rules": {
				"suspicious": {
					"noDebugger": "error" /* always flag */
				}
			}
		}
	}`)

	cfg, report, err := migrateBiome(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.migratedCount != 1 {
		t.Errorf("expected 1 migrated, got %d", report.migratedCount)
	}
	if cfg.Rules["no-debugger"].Severity != config.SeverityError {
		t.Errorf("expected error, got %s", cfg.Rules["no-debugger"].Severity)
	}
}

func TestMigrateBiome_NoConfigFound(t *testing.T) {
	dir := t.TempDir()
	_, _, err := migrateBiome(dir)
	if err == nil {
		t.Fatal("expected error when no config found")
	}
}

func TestParseBiomeSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  config.Severity
		ok    bool
	}{
		{`"error"`, config.SeverityError, true},
		{`"warn"`, config.SeverityWarn, true},
		{`"off"`, config.SeverityOff, true},
		{`{"level":"error"}`, config.SeverityError, true},
		{`{"level":"warn","options":{}}`, config.SeverityWarn, true},
		{`"invalid"`, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, ok := parseBiomeSeverity(json.RawMessage(tc.input))
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("severity = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStripJSONC(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no comments", `{"a": 1}`, `{"a": 1}`},
		{"single-line comment", "{// comment\n\"a\": 1}", "{\n\"a\": 1}"},
		{"multi-line comment", `{"a": /* block */ 1}`, `{"a":  1}`},
		{"comment inside string", `{"a": "// not a comment"}`, `{"a": "// not a comment"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(stripJSONC([]byte(tc.input)))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
