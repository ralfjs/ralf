package config

import "testing"

func TestBuiltinRules(t *testing.T) {
	rules := BuiltinRules()

	t.Run("returns expected rule count", func(t *testing.T) {
		if got := len(rules); got != 49 {
			t.Errorf("BuiltinRules() returned %d rules, want 49", got)
		}
	})

	t.Run("every rule has a matcher, message, and valid severity", func(t *testing.T) {
		for name, r := range rules {
			hasMatcher := r.Regex != "" || r.Pattern != "" || r.AST != nil || r.Imports != nil || r.Builtin
			if !hasMatcher {
				t.Errorf("rule %q: no matcher set", name)
			}
			if r.Message == "" {
				t.Errorf("rule %q: Message is empty", name)
			}
			switch r.Severity {
			case SeverityError, SeverityWarn:
				// valid
			default:
				t.Errorf("rule %q: invalid severity %q", name, r.Severity)
			}
		}
	})

	t.Run("returns fresh map each call", func(t *testing.T) {
		a := BuiltinRules()
		b := BuiltinRules()
		a["no-var"] = RuleConfig{}
		if _, ok := b["no-var"]; !ok || b["no-var"].Regex == "" {
			t.Error("mutating one map affected the other")
		}
	})
}

func TestRecommendedConfig(t *testing.T) {
	cfg := RecommendedConfig()

	if cfg == nil {
		t.Fatal("RecommendedConfig() returned nil")
	}
	if len(cfg.Rules) != 49 {
		t.Errorf("RecommendedConfig() has %d rules, want 49", len(cfg.Rules))
	}

	if err := Validate(cfg); err != nil {
		t.Errorf("RecommendedConfig() fails validation: %v", err)
	}
}
