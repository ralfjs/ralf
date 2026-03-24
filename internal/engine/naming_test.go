package engine

import (
	"errors"
	"testing"

	"github.com/ralfjs/ralf/internal/config"
)

func TestCompileNaming(t *testing.T) {
	t.Run("valid regex", func(t *testing.T) {
		t.Parallel()
		cn, err := compileNaming("test", &config.NamingMatcher{
			Match:   "^[a-z][a-zA-Z0-9]*$",
			Message: "must be camelCase",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cn == nil {
			t.Fatal("expected non-nil compiledNaming")
		}
		if cn.message != "must be camelCase" {
			t.Errorf("message = %q, want %q", cn.message, "must be camelCase")
		}
	})

	t.Run("invalid regex", func(t *testing.T) {
		t.Parallel()
		_, err := compileNaming("test", &config.NamingMatcher{
			Match: "[invalid",
		})
		if err == nil {
			t.Fatal("expected error for invalid regex")
		}
		if !errors.Is(err, ErrNamingCompile) {
			t.Errorf("error should wrap ErrNamingCompile, got: %v", err)
		}
	})

	t.Run("empty match", func(t *testing.T) {
		t.Parallel()
		_, err := compileNaming("test", &config.NamingMatcher{Match: ""})
		if err == nil {
			t.Fatal("expected error for empty match")
		}
		if !errors.Is(err, ErrNamingCompile) {
			t.Errorf("error should wrap ErrNamingCompile, got: %v", err)
		}
	})

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()
		cn, err := compileNaming("test", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cn != nil {
			t.Fatal("expected nil for nil input")
		}
	})
}

func TestCompiledNamingMatches(t *testing.T) {
	t.Run("conforming name", func(t *testing.T) {
		t.Parallel()
		cn, err := compileNaming("test", &config.NamingMatcher{Match: "^[a-z][a-zA-Z0-9]*$"})
		if err != nil {
			t.Fatal(err)
		}
		if !cn.matches("camelCase") {
			t.Error("expected camelCase to match")
		}
	})

	t.Run("violating name", func(t *testing.T) {
		t.Parallel()
		cn, err := compileNaming("test", &config.NamingMatcher{Match: "^[a-z][a-zA-Z0-9]*$"})
		if err != nil {
			t.Fatal(err)
		}
		if cn.matches("PascalCase") {
			t.Error("expected PascalCase to not match")
		}
	})

	t.Run("nil receiver", func(t *testing.T) {
		t.Parallel()
		var cn *compiledNaming
		if !cn.matches("anything") {
			t.Error("nil receiver should always return true")
		}
	})
}
