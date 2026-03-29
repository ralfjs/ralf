package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSpecifier_Relative(t *testing.T) {
	dir := t.TempDir()
	writeResolveFile(t, filepath.Join(dir, "utils.ts"), "export function foo() {}")
	writeResolveFile(t, filepath.Join(dir, "bar.js"), "export const bar = 1;")

	fromFile := filepath.Join(dir, "app.ts")

	t.Run("with extension", func(t *testing.T) {
		got, ok := ResolveSpecifier("./utils.ts", fromFile)
		if !ok {
			t.Fatal("expected resolution")
		}
		want := filepath.Join(dir, "utils.ts")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("without extension finds .ts", func(t *testing.T) {
		got, ok := ResolveSpecifier("./utils", fromFile)
		if !ok {
			t.Fatal("expected resolution")
		}
		want := filepath.Join(dir, "utils.ts")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("without extension finds .js", func(t *testing.T) {
		got, ok := ResolveSpecifier("./bar", fromFile)
		if !ok {
			t.Fatal("expected resolution")
		}
		want := filepath.Join(dir, "bar.js")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestResolveSpecifier_IndexFile(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "components")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeResolveFile(t, filepath.Join(subDir, "index.ts"), "export const Button = 1;")

	fromFile := filepath.Join(dir, "app.ts")

	got, ok := ResolveSpecifier("./components", fromFile)
	if !ok {
		t.Fatal("expected resolution via index file")
	}
	want := filepath.Join(subDir, "index.ts")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveSpecifier_BareSpecifier(t *testing.T) {
	_, ok := ResolveSpecifier("react", "/src/app.ts")
	if ok {
		t.Error("expected bare specifier to not resolve")
	}
}

func TestResolveSpecifier_Unresolvable(t *testing.T) {
	_, ok := ResolveSpecifier("./nonexistent", "/src/app.ts")
	if ok {
		t.Error("expected unresolvable specifier to return false")
	}
}

func TestResolveSpecifier_ParentDir(t *testing.T) {
	dir := t.TempDir()
	writeResolveFile(t, filepath.Join(dir, "utils.ts"), "export function foo() {}")

	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o750); err != nil {
		t.Fatal(err)
	}

	fromFile := filepath.Join(subDir, "app.ts")
	got, ok := ResolveSpecifier("../utils", fromFile)
	if !ok {
		t.Fatal("expected resolution")
	}
	want := filepath.Join(dir, "utils.ts")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func writeResolveFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
