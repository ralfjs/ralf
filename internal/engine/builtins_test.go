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
	if len(eng.regexRules) != 20 {
		t.Errorf("expected 20 compiled regex rules, got %d", len(eng.regexRules))
	}
}
