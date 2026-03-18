package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var extendsDir = filepath.Join(testdataDir, "extends")

func TestResolveExtends_NoExtends(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"no-eval": {Regex: `\beval\b`, Severity: SeverityError},
		},
	}
	got, err := ResolveExtends(cfg, "/any/dir")
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	if len(got.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(got.Rules))
	}
	if _, ok := got.Rules["no-eval"]; !ok {
		t.Error("expected rule 'no-eval'")
	}
}

func TestResolveExtends_SingleFile(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Extends: []string{"./base.json"},
		Rules: map[string]RuleConfig{
			"no-var": {Regex: `\bvar\b`, Severity: SeverityWarn},
		},
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	// Should have base's no-eval + current's no-var
	if len(got.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(got.Rules))
	}
	if _, ok := got.Rules["no-eval"]; !ok {
		t.Error("expected rule 'no-eval' from base")
	}
	if _, ok := got.Rules["no-var"]; !ok {
		t.Error("expected rule 'no-var' from current")
	}
}

func TestResolveExtends_MultipleFiles(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Extends: []string{"./base.json", "./strict.json"},
		Rules:   map[string]RuleConfig{},
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	// base has no-eval, strict has no-console
	if len(got.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(got.Rules))
	}
	if _, ok := got.Rules["no-eval"]; !ok {
		t.Error("expected rule 'no-eval' from base")
	}
	if _, ok := got.Rules["no-console"]; !ok {
		t.Error("expected rule 'no-console' from strict")
	}
}

func TestResolveExtends_CurrentWins(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Extends: []string{"./base.json"},
		Rules: map[string]RuleConfig{
			"no-eval": {Regex: `\beval\b`, Severity: SeverityWarn, Message: "overridden"},
		},
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	rule := got.Rules["no-eval"]
	if rule.Severity != SeverityWarn {
		t.Errorf("severity = %q, want %q (current should win)", rule.Severity, SeverityWarn)
	}
	if rule.Message != "overridden" {
		t.Errorf("message = %q, want %q", rule.Message, "overridden")
	}
}

func TestResolveExtends_IgnoresMerge(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Extends: []string{"./base.json", "./strict.json"},
		Rules:   map[string]RuleConfig{},
		Ignores: []string{"vendor/**"},
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	// base: node_modules/**, strict: dist/**, current: vendor/**
	if len(got.Ignores) != 3 {
		t.Errorf("expected 3 ignores, got %d: %v", len(got.Ignores), got.Ignores)
	}
}

func TestResolveExtends_OverridesMerge(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Extends: []string{"./base.json"},
		Rules:   map[string]RuleConfig{},
		Overrides: []Override{{
			Files: []string{"*.spec.js"},
			Rules: map[string]RuleConfig{"no-var": {Severity: SeverityOff, Regex: "x"}},
		}},
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	// base.json has no overrides, current has 1
	if len(got.Overrides) != 1 {
		t.Errorf("expected 1 override, got %d", len(got.Overrides))
	}
}

func TestMergeInto_OverridesConcatenated(t *testing.T) {
	t.Parallel()
	dst := &Config{
		Rules:     make(map[string]RuleConfig),
		Overrides: []Override{{Files: []string{"*.test.js"}, Rules: map[string]RuleConfig{"a": {Regex: "a", Severity: SeverityOff}}}},
	}
	src := &Config{
		Rules:     map[string]RuleConfig{"r": {Regex: "r", Severity: SeverityError}},
		Overrides: []Override{{Files: []string{"*.spec.js"}, Rules: map[string]RuleConfig{"b": {Regex: "b", Severity: SeverityOff}}}},
	}
	mergeInto(dst, src)
	if len(dst.Overrides) != 2 {
		t.Errorf("expected 2 overrides after merge, got %d", len(dst.Overrides))
	}
}

func TestResolveExtends_Recursive(t *testing.T) {
	t.Parallel()
	cfg, err := LoadFile(filepath.Join(extendsDir, "recursive-a.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	// recursive-a has no-var, extends recursive-b (no-console), which extends recursive-c (no-eval)
	if len(got.Rules) != 3 {
		t.Errorf("expected 3 rules, got %d: %v", len(got.Rules), ruleNames(got))
	}
	for _, name := range []string{"no-eval", "no-console", "no-var"} {
		if _, ok := got.Rules[name]; !ok {
			t.Errorf("expected rule %q", name)
		}
	}
}

func TestResolveExtends_Circular(t *testing.T) {
	t.Parallel()
	cfg, err := LoadFile(filepath.Join(extendsDir, "circular-a.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	_, err = ResolveExtends(cfg, extendsDir)
	if err == nil {
		t.Fatal("expected error for circular extends")
	}
	if !errors.Is(err, ErrCircularExtend) {
		t.Errorf("expected ErrCircularExtend, got: %v", err)
	}
}

func TestResolveExtends_NotFound(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Extends: []string{"./nonexistent.json"},
		Rules:   map[string]RuleConfig{},
	}
	_, err := ResolveExtends(cfg, extendsDir)
	if err == nil {
		t.Fatal("expected error for missing extends file")
	}
}

func TestResolveExtends_RelativePaths(t *testing.T) {
	t.Parallel()
	// child.json extends ./base.json — resolved from child's directory
	cfg, err := LoadFile(filepath.Join(extendsDir, "child.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	// child has no-var, base has no-eval
	if _, ok := got.Rules["no-eval"]; !ok {
		t.Error("expected rule 'no-eval' from base (relative path)")
	}
	if _, ok := got.Rules["no-var"]; !ok {
		t.Error("expected rule 'no-var' from child")
	}
}

func TestResolveExtends_NamedPresetError(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Extends: []string{"@org/rules"},
		Rules:   map[string]RuleConfig{},
	}
	_, err := ResolveExtends(cfg, extendsDir)
	if err == nil {
		t.Fatal("expected error for named preset")
	}
	if got := err.Error(); !strings.Contains(got, "named presets not yet supported") {
		t.Errorf("expected named preset error, got: %v", err)
	}
}

func TestResolveExtends_MultiFixture(t *testing.T) {
	t.Parallel()
	cfg, err := LoadFile(filepath.Join(extendsDir, "multi.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	got, err := ResolveExtends(cfg, extendsDir)
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	// multi extends base (no-eval) + strict (no-console), adds no-debugger
	for _, name := range []string{"no-eval", "no-console", "no-debugger"} {
		if _, ok := got.Rules[name]; !ok {
			t.Errorf("expected rule %q", name)
		}
	}
}

func TestResolveExtends_AbsolutePath(t *testing.T) {
	t.Parallel()
	abs, err := filepath.Abs(filepath.Join(extendsDir, "base.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		Extends: []string{abs},
		Rules:   map[string]RuleConfig{},
	}
	got, err := ResolveExtends(cfg, "/irrelevant")
	if err != nil {
		t.Fatalf("ResolveExtends: %v", err)
	}
	if _, ok := got.Rules["no-eval"]; !ok {
		t.Error("expected rule 'no-eval' from absolute path extends")
	}
}

func TestResolveExtends_Diamond(t *testing.T) {
	t.Parallel()
	// A extends both B and C; B and C both extend D.
	// This is a diamond, not a cycle — must succeed.
	dir := t.TempDir()

	writeJSON := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeJSON("d.json", `{"rules":{"from-d":{"regex":"d","severity":"error"}}}`)
	writeJSON("b.json", `{"extends":["./d.json"],"rules":{"from-b":{"regex":"b","severity":"warn"}}}`)
	writeJSON("c.json", `{"extends":["./d.json"],"rules":{"from-c":{"regex":"c","severity":"warn"}}}`)

	cfg := &Config{
		Extends: []string{"./b.json", "./c.json"},
		Rules:   map[string]RuleConfig{},
	}
	got, err := ResolveExtends(cfg, dir)
	if err != nil {
		t.Fatalf("ResolveExtends diamond: %v", err)
	}
	for _, name := range []string{"from-d", "from-b", "from-c"} {
		if _, ok := got.Rules[name]; !ok {
			t.Errorf("expected rule %q in diamond resolution", name)
		}
	}
}

func ruleNames(cfg *Config) []string {
	names := make([]string, 0, len(cfg.Rules))
	for name := range cfg.Rules {
		names = append(names, name)
	}
	return names
}
