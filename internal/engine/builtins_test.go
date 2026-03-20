package engine

import (
	"testing"

	"github.com/Hideart/ralf/internal/config"
)

func TestBuiltinRulesCompile(t *testing.T) {
	cfg := config.RecommendedConfig()
	eng, errs := New(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("compile error: %v", e)
		}
		t.FailNow()
	}
	if eng == nil {
		t.Fatal("engine is nil")
	}
	if len(eng.regexRules) != 23 {
		t.Errorf("expected 23 compiled regex rules, got %d", len(eng.regexRules))
	}
	if len(eng.patternRules) != 4 {
		t.Errorf("expected 4 compiled pattern rules, got %d", len(eng.patternRules))
	}
	if len(eng.structuralRules) != 1 {
		t.Errorf("expected 1 compiled structural rule, got %d", len(eng.structuralRules))
	}
	if len(eng.builtinRules) != 33 {
		t.Errorf("expected 33 compiled builtin rules, got %d", len(eng.builtinRules))
	}
}
