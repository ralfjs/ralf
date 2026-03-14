package config

import "testing"

func TestMergeNoOverrides(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"rule-a": {Severity: SeverityError, Regex: "foo"},
		},
	}

	result := Merge(cfg, "src/app.js")
	if len(result) != 1 {
		t.Errorf("expected 1 rule, got %d", len(result))
	}
	if result["rule-a"].Severity != SeverityError {
		t.Errorf("severity = %q, want %q", result["rule-a"].Severity, SeverityError)
	}
}

func TestMergeMatchingOverride(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"rule-a": {Severity: SeverityError, Regex: "foo"},
		},
		Overrides: []Override{
			{
				Files: []string{"*.test.js"},
				Rules: map[string]RuleConfig{
					"rule-a": {Severity: SeverityOff, Regex: "foo"},
				},
			},
		},
	}

	result := Merge(cfg, "app.test.js")
	if result["rule-a"].Severity != SeverityOff {
		t.Errorf("severity = %q, want %q (override should apply)", result["rule-a"].Severity, SeverityOff)
	}
}

func TestMergeNonMatchingOverride(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"rule-a": {Severity: SeverityError, Regex: "foo"},
		},
		Overrides: []Override{
			{
				Files: []string{"*.test.js"},
				Rules: map[string]RuleConfig{
					"rule-a": {Severity: SeverityOff, Regex: "foo"},
				},
			},
		},
	}

	result := Merge(cfg, "app.js")
	if result["rule-a"].Severity != SeverityError {
		t.Errorf("severity = %q, want %q (override should NOT apply)", result["rule-a"].Severity, SeverityError)
	}
}

func TestMergeOverrideAddsNewRule(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"rule-a": {Severity: SeverityError, Regex: "foo"},
		},
		Overrides: []Override{
			{
				Files: []string{"*.test.js"},
				Rules: map[string]RuleConfig{
					"rule-b": {Severity: SeverityWarn, Regex: "bar"},
				},
			},
		},
	}

	result := Merge(cfg, "app.test.js")
	if len(result) != 2 {
		t.Errorf("expected 2 rules, got %d", len(result))
	}
	if result["rule-b"].Severity != SeverityWarn {
		t.Errorf("rule-b severity = %q, want %q", result["rule-b"].Severity, SeverityWarn)
	}
}

func TestMergeLaterOverrideWins(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleConfig{
			"rule-a": {Severity: SeverityError, Regex: "foo"},
		},
		Overrides: []Override{
			{
				Files: []string{"*.js"},
				Rules: map[string]RuleConfig{
					"rule-a": {Severity: SeverityWarn, Regex: "foo"},
				},
			},
			{
				Files: []string{"*.js"},
				Rules: map[string]RuleConfig{
					"rule-a": {Severity: SeverityOff, Regex: "foo"},
				},
			},
		},
	}

	result := Merge(cfg, "app.js")
	if result["rule-a"].Severity != SeverityOff {
		t.Errorf("severity = %q, want %q (later override should win)", result["rule-a"].Severity, SeverityOff)
	}
}
