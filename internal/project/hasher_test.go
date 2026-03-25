package project

import (
	"testing"

	"github.com/ralfjs/ralf/internal/config"
)

func TestHashFile_Deterministic(t *testing.T) {
	data := []byte("hello world")
	h1 := HashFile(data)
	h2 := HashFile(data)
	if h1 != h2 {
		t.Errorf("expected same hash, got %d and %d", h1, h2)
	}
}

func TestHashFile_DifferentInput(t *testing.T) {
	h1 := HashFile([]byte("hello"))
	h2 := HashFile([]byte("world"))
	if h1 == h2 {
		t.Error("expected different hashes for different input")
	}
}

func TestHashFile_Empty(t *testing.T) {
	h := HashFile([]byte{})
	if h == 0 {
		t.Error("expected nonzero hash for empty input")
	}
}

func TestHashConfig_Deterministic(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {Severity: config.SeverityError},
		},
	}
	h1, err := HashConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("expected same hash, got %d and %d", h1, h2)
	}
}

func TestHashConfig_DifferentRules(t *testing.T) {
	cfg1 := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var": {Severity: config.SeverityError},
		},
	}
	cfg2 := &config.Config{
		Rules: map[string]config.RuleConfig{
			"no-var":     {Severity: config.SeverityError},
			"no-console": {Severity: config.SeverityWarn},
		},
	}
	h1, _ := HashConfig(cfg1)
	h2, _ := HashConfig(cfg2)
	if h1 == h2 {
		t.Error("expected different hashes for different configs")
	}
}
