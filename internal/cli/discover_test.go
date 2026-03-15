package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverFiles(t *testing.T) {
	t.Run("single JS file", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "index.js"), "var x = 1;")

		files, err := discoverFiles([]string{filepath.Join(dir, "index.js")}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file, got %d", len(files))
		}
	})

	t.Run("directory recursion", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "a.js"), "var x;")
		if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o750); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(dir, "sub", "b.ts"), "let y: number;")

		files, err := discoverFiles([]string{dir}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 {
			t.Fatalf("expected 2 files, got %d: %v", len(files), files)
		}
	})

	t.Run("skips node_modules", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "index.js"), "var x;")
		nm := filepath.Join(dir, "node_modules", "pkg")
		if err := os.MkdirAll(nm, 0o750); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(nm, "index.js"), "var y;")

		files, err := discoverFiles([]string{dir}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file (node_modules skipped), got %d", len(files))
		}
	})

	t.Run("ignore patterns", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "index.js"), "var x;")
		writeTestFile(t, filepath.Join(dir, "generated.js"), "var y;")

		files, err := discoverFiles([]string{dir}, []string{"generated.js"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file (ignored), got %d: %v", len(files), files)
		}
	})

	t.Run("unsupported extensions skipped", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "readme.md"), "# Hello")
		writeTestFile(t, filepath.Join(dir, "main.go"), "package main")
		writeTestFile(t, filepath.Join(dir, "index.js"), "var x;")

		files, err := discoverFiles([]string{dir}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file, got %d: %v", len(files), files)
		}
	})

	t.Run("non-existent path errors", func(t *testing.T) {
		_, err := discoverFiles([]string{"/definitely/not/a/path"}, nil)
		if err == nil {
			t.Fatal("expected error for non-existent path")
		}
	})

	t.Run("sorted output", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "c.js"), "")
		writeTestFile(t, filepath.Join(dir, "a.js"), "")
		writeTestFile(t, filepath.Join(dir, "b.js"), "")

		files, err := discoverFiles([]string{dir}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Fatalf("expected 3 files, got %d", len(files))
		}
		for i := 1; i < len(files); i++ {
			if files[i-1] >= files[i] {
				t.Errorf("files not sorted: %v", files)
				break
			}
		}
	})
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
