package config

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestValidateValidConfig(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "valid.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("valid config failed validation: %v", err)
	}
}

func TestValidateMinimalConfig(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "minimal.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("minimal config failed validation: %v", err)
	}
}

func TestValidateNoMatcher(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "invalid-no-matcher.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	err = Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for no-matcher rule")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if len(ve.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(ve.Errors))
	}
	if ve.Errors[0].Field != "matcher" {
		t.Errorf("error field = %q, want %q", ve.Errors[0].Field, "matcher")
	}
}

func TestValidateBadSeverity(t *testing.T) {
	cfg, err := LoadFile(filepath.Join(testdataDir, "invalid-severity.json"))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	err = Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for bad severity")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e.Field == "severity" {
			found = true
		}
	}
	if !found {
		t.Error("expected a severity field error")
	}
}

func TestValidateMultipleMatchers(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"double-matcher": {
				Severity: SeverityError,
				Regex:    "foo",
				Pattern:  "bar",
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for multiple matchers")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
}

func TestValidateMissingSeverity(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"no-severity": {
				Regex: "foo",
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing severity")
	}
}

func TestValidateEmptyOverrideFiles(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"ok": {Severity: SeverityError, Regex: "x"},
		},
		Overrides: []Override{
			{Files: []string{}, Rules: map[string]RuleConfig{}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty override files")
	}
}

func TestValidateMalformedGlob(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"ok": {Severity: SeverityError, Regex: "x"},
		},
		Overrides: []Override{
			{Files: []string{"[invalid"}, Rules: map[string]RuleConfig{}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for malformed glob")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e.Field == "files[0]" {
			found = true
		}
	}
	if !found {
		t.Error("expected files[0] field error for malformed glob")
	}
}

func TestValidateEmptyRules(t *testing.T) {
	cfg := &Config{Rules: map[string]RuleConfig{}}
	if err := Validate(cfg); err != nil {
		t.Errorf("empty rules should be valid: %v", err)
	}
}

func TestValidateEmptyGlobString(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"ok": {Severity: SeverityError, Regex: "x"},
		},
		Overrides: []Override{
			{Files: []string{""}, Rules: map[string]RuleConfig{}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty glob string")
	}
}

func TestValidateInvalidOverrideRule(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"ok": {Severity: SeverityError, Regex: "x"},
		},
		Overrides: []Override{
			{
				Files: []string{"*.test.js"},
				Rules: map[string]RuleConfig{
					"bad-override": {Message: "no matcher or severity"},
				},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid override rule")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	// Should have errors for both missing severity and missing matcher
	if len(ve.Errors) < 2 {
		t.Errorf("expected at least 2 errors, got %d: %v", len(ve.Errors), ve)
	}
}

func TestValidateAllMatcherTypes(t *testing.T) {
	tests := []struct {
		name string
		rule RuleConfig
	}{
		{"regex", RuleConfig{Severity: SeverityWarn, Regex: "foo"}},
		{"pattern", RuleConfig{Severity: SeverityWarn, Pattern: "bar"}},
		{"ast", RuleConfig{Severity: SeverityWarn, AST: &ASTMatcher{Kind: "x"}}},
		{"imports", RuleConfig{Severity: SeverityWarn, Imports: &ImportsMatcher{Groups: []string{"a"}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Rules: map[string]RuleConfig{tt.name: tt.rule}}
			if err := Validate(cfg); err != nil {
				t.Errorf("valid %s rule failed: %v", tt.name, err)
			}
		})
	}
}

func TestValidateNamingStandaloneError(t *testing.T) {
	// Naming alone (without ast) is not a valid matcher.
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"bad": {Severity: SeverityWarn, Naming: &NamingMatcher{Match: "^[a-z]"}},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for standalone naming")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	// Should have errors: no matcher + naming requires ast
	foundMatcher := false
	foundNaming := false
	for _, e := range ve.Errors {
		if e.Field == "matcher" {
			foundMatcher = true
		}
		if e.Field == "naming" {
			foundNaming = true
		}
	}
	if !foundMatcher {
		t.Error("expected matcher field error")
	}
	if !foundNaming {
		t.Error("expected naming field error")
	}
}

func TestValidateASTWithNaming(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"camelcase-fn": {
				Severity: SeverityError,
				AST:      &ASTMatcher{Kind: "function_declaration"},
				Naming:   &NamingMatcher{Match: "^[a-z][a-zA-Z0-9]*$", Message: "must be camelCase"},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("ast+naming should be valid: %v", err)
	}
}

func TestValidateNamingWithoutAST(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"bad": {
				Severity: SeverityError,
				Regex:    "foo",
				Naming:   &NamingMatcher{Match: "^[a-z]"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for naming without ast")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	foundNaming := false
	for _, e := range ve.Errors {
		if e.Field == "naming" && e.Message == "naming can only be combined with ast" {
			foundNaming = true
		}
	}
	if !foundNaming {
		t.Error("expected 'naming can only be combined with ast' error")
	}
}

func TestValidateImportsEmptyGroups(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"import-order": {
				Severity: SeverityWarn,
				Imports:  &ImportsMatcher{Groups: []string{}},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty imports.groups")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e.Field == "imports.groups" {
			found = true
		}
	}
	if !found {
		t.Error("expected imports.groups field error")
	}
}

func TestValidateImportsEmptyGroupName(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"import-order": {
				Severity: SeverityWarn,
				Imports:  &ImportsMatcher{Groups: []string{"builtin", ""}},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty group name")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e.Field == "imports.groups[1]" {
			found = true
		}
	}
	if !found {
		t.Error("expected imports.groups[1] field error")
	}
}

func TestValidateImportsDuplicateGroup(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"import-order": {
				Severity: SeverityWarn,
				Imports:  &ImportsMatcher{Groups: []string{"builtin", "external", "builtin"}},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for duplicate group")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e.Field == "imports.groups[2]" {
			found = true
		}
	}
	if !found {
		t.Error("expected imports.groups[2] field error for duplicate")
	}
}

func TestValidateImportsValid(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"import-order": {
				Severity: SeverityWarn,
				Imports: &ImportsMatcher{
					Groups:      []string{"builtin", "external"},
					Alphabetize: true,
				},
			},
		},
	}
	if err := Validate(cfg); err != nil {
		t.Errorf("valid imports config failed validation: %v", err)
	}
}

func TestValidateNamingEmptyMatch(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"bad": {
				Severity: SeverityError,
				AST:      &ASTMatcher{Kind: "function_declaration"},
				Naming:   &NamingMatcher{Message: "no match field"},
			},
		},
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected error for empty naming.match")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	found := false
	for _, e := range ve.Errors {
		if e.Field == "naming.match" {
			found = true
		}
	}
	if !found {
		t.Error("expected naming.match field error")
	}
}
