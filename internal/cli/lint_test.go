package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLintIntegration(t *testing.T) {
	// Create a temp project with config + source files.
	dir := t.TempDir()

	configJSON := `{
  "rules": {
    "no-var": {
      "severity": "error",
      "regex": "\\bvar\\b",
      "message": "Use let or const instead of var"
    }
  }
}`
	writeTestFile(t, filepath.Join(dir, ".ralfrc.json"), configJSON)
	writeTestFile(t, filepath.Join(dir, "bad.js"), "var x = 1;\nlet y = 2;")
	writeTestFile(t, filepath.Join(dir, "clean.js"), "let a = 1;\nconst b = 2;")

	t.Run("finds violations and exits 1", func(t *testing.T) {
		// Reset global state.
		exitCode = 0
		configPath = ""

		var stdout, stderr bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"lint", "--config", filepath.Join(dir, ".ralfrc.json"), dir})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected cobra error: %v", err)
		}
		if exitCode != ExitLintErrors {
			t.Errorf("expected exit code %d, got %d", ExitLintErrors, exitCode)
		}
		if !bytes.Contains(stdout.Bytes(), []byte("no-var")) {
			t.Errorf("expected no-var in output, got: %s", stdout.String())
		}
	})

	t.Run("clean project exits 0", func(t *testing.T) {
		cleanDir := t.TempDir()
		writeTestFile(t, filepath.Join(cleanDir, ".ralfrc.json"), configJSON)
		writeTestFile(t, filepath.Join(cleanDir, "clean.js"), "let a = 1;")

		exitCode = 0
		configPath = ""

		var stdout, stderr bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"lint", "--config", filepath.Join(cleanDir, ".ralfrc.json"), cleanDir})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected cobra error: %v", err)
		}
		if exitCode != ExitOK {
			t.Errorf("expected exit code %d, got %d\nstdout: %s\nstderr: %s",
				ExitOK, exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("json format", func(t *testing.T) {
		exitCode = 0
		configPath = ""

		var stdout bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"lint", "--format", "json", "--config", filepath.Join(dir, ".ralfrc.json"), dir})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected cobra error: %v", err)
		}
		if !bytes.Contains(stdout.Bytes(), []byte(`"rule"`)) {
			t.Errorf("expected JSON output, got: %s", stdout.String())
		}
	})

	t.Run("max-warnings triggers exit 1", func(t *testing.T) {
		warnDir := t.TempDir()
		warnConfig := `{
  "rules": {
    "no-var": {
      "severity": "warn",
      "regex": "\\bvar\\b",
      "message": "Use let or const"
    }
  }
}`
		writeTestFile(t, filepath.Join(warnDir, ".ralfrc.json"), warnConfig)
		writeTestFile(t, filepath.Join(warnDir, "a.js"), "var x;\nvar y;")

		exitCode = 0
		configPath = ""

		var stdout, stderr bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"lint", "--max-warnings", "0", "--config", filepath.Join(warnDir, ".ralfrc.json"), warnDir})

		_ = cmd.Execute()
		if exitCode != ExitLintErrors {
			t.Errorf("expected exit code %d, got %d", ExitLintErrors, exitCode)
		}
	})

	t.Run("no config falls back to recommended", func(t *testing.T) {
		noConfigDir := t.TempDir()
		writeTestFile(t, filepath.Join(noConfigDir, "bad.js"), "var x = 1;")

		exitCode = 0
		configPath = ""

		var stdout, stderr bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"lint", noConfigDir})

		// Override working directory so loadConfig() searches the temp dir.
		// t.Chdir automatically restores cwd when the test ends.
		t.Chdir(noConfigDir)

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected cobra error: %v", err)
		}
		if exitCode != ExitLintErrors {
			t.Errorf("expected exit code %d, got %d\nstdout: %s\nstderr: %s",
				ExitLintErrors, exitCode, stdout.String(), stderr.String())
		}
		if !bytes.Contains(stdout.Bytes(), []byte("no-var")) {
			t.Errorf("expected no-var diagnostic in output, got: %s", stdout.String())
		}
	})

	t.Run("missing config exits 2", func(t *testing.T) {
		emptyDir := t.TempDir()

		// Point --config at a non-existent file to trigger config error
		// without relying on os.Chdir (which is process-global and unsafe
		// with parallel tests).
		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"lint", "--config", filepath.Join(emptyDir, ".ralfrc.json")})

		_ = cmd.Execute()
		if exitCode != ExitUsageError {
			t.Errorf("expected exit code %d, got %d", ExitUsageError, exitCode)
		}
	})

	t.Run("--fix applies fixes and writes file", func(t *testing.T) {
		fixDir := t.TempDir()
		fixConfig := `{
  "rules": {
    "no-var": {
      "severity": "error",
      "regex": "\\bvar\\b",
      "message": "Use let or const",
      "fix": "let"
    }
  }
}`
		writeTestFile(t, filepath.Join(fixDir, ".ralfrc.json"), fixConfig)
		writeTestFile(t, filepath.Join(fixDir, "a.js"), "var x = 1;\nlet y = 2;\nvar z = 3;")

		exitCode = 0
		configPath = ""

		var stdout, stderr bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"lint", "--fix", "--config", filepath.Join(fixDir, ".ralfrc.json"), fixDir})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected cobra error: %v", err)
		}

		// File should be fixed.
		fixedPath := filepath.Join(fixDir, "a.js")
		got, err := os.ReadFile(fixedPath) //nolint:gosec // test-only, path is from t.TempDir
		if err != nil {
			t.Fatal(err)
		}
		want := "let x = 1;\nlet y = 2;\nlet z = 3;"
		if string(got) != want {
			t.Errorf("file content = %q, want %q", got, want)
		}

		// Stderr should report fix count.
		if !bytes.Contains(stderr.Bytes(), []byte("Fixed")) {
			t.Errorf("expected fix report in stderr, got: %s", stderr.String())
		}

		// All diagnostics were fixable, so exit 0.
		if exitCode != ExitOK {
			t.Errorf("expected exit code %d, got %d\nstdout: %s\nstderr: %s",
				ExitOK, exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("--fix-dry-run shows diff without writing", func(t *testing.T) {
		dryDir := t.TempDir()
		dryConfig := `{
  "rules": {
    "no-var": {
      "severity": "error",
      "regex": "\\bvar\\b",
      "message": "Use let or const",
      "fix": "let"
    }
  }
}`
		writeTestFile(t, filepath.Join(dryDir, ".ralfrc.json"), dryConfig)
		writeTestFile(t, filepath.Join(dryDir, "a.js"), "var x = 1;")

		exitCode = 0
		configPath = ""

		var stdout, stderr bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"lint", "--fix-dry-run", "--config", filepath.Join(dryDir, ".ralfrc.json"), dryDir})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected cobra error: %v", err)
		}

		// Stdout should contain diff.
		if !bytes.Contains(stdout.Bytes(), []byte("---")) {
			t.Errorf("expected diff in stdout, got: %s", stdout.String())
		}

		// File should NOT be modified.
		dryPath := filepath.Join(dryDir, "a.js")
		got, _ := os.ReadFile(dryPath) //nolint:gosec // test-only, path is from t.TempDir
		if string(got) != "var x = 1;" {
			t.Errorf("file should not be modified in dry-run, got %q", got)
		}
	})

	t.Run("cache creates .ralf directory", func(t *testing.T) {
		cacheDir := t.TempDir()
		writeTestFile(t, filepath.Join(cacheDir, ".ralfrc.json"), configJSON)
		writeTestFile(t, filepath.Join(cacheDir, "a.js"), "var x = 1;")

		exitCode = 0
		configPath = ""

		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"lint", "--config", filepath.Join(cacheDir, ".ralfrc.json"), cacheDir})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exitCode != ExitLintErrors {
			t.Fatalf("expected exit %d, got %d", ExitLintErrors, exitCode)
		}

		if _, err := os.Stat(filepath.Join(cacheDir, ".ralf", "cache.db")); err != nil {
			t.Errorf("expected .ralf/cache.db after lint, got: %v", err)
		}
	})

	t.Run("second lint run uses cache", func(t *testing.T) {
		cacheDir := t.TempDir()
		writeTestFile(t, filepath.Join(cacheDir, ".ralfrc.json"), configJSON)
		writeTestFile(t, filepath.Join(cacheDir, "a.js"), "var x = 1;")

		configFile := filepath.Join(cacheDir, ".ralfrc.json")

		// First run — populates cache.
		exitCode = 0
		configPath = ""
		cmd1 := newRootCmd()
		var out1 bytes.Buffer
		cmd1.SetOut(&out1)
		cmd1.SetErr(&bytes.Buffer{})
		cmd1.SetArgs([]string{"lint", "--format", "json", "--config", configFile, cacheDir})
		if err := cmd1.Execute(); err != nil {
			t.Fatalf("first run error: %v", err)
		}
		if exitCode != ExitLintErrors {
			t.Fatalf("first run: expected exit %d, got %d", ExitLintErrors, exitCode)
		}

		// Second run — should produce identical output from cache.
		exitCode = 0
		configPath = ""
		cmd2 := newRootCmd()
		var out2 bytes.Buffer
		cmd2.SetOut(&out2)
		cmd2.SetErr(&bytes.Buffer{})
		cmd2.SetArgs([]string{"lint", "--format", "json", "--config", configFile, cacheDir})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("second run error: %v", err)
		}
		if exitCode != ExitLintErrors {
			t.Fatalf("second run: expected exit %d, got %d", ExitLintErrors, exitCode)
		}

		if out1.String() != out2.String() {
			t.Errorf("cached run produced different output:\nfirst:  %s\nsecond: %s", out1.String(), out2.String())
		}
	})

	t.Run("--no-cache does not create .ralf directory", func(t *testing.T) {
		noCacheDir := t.TempDir()
		writeTestFile(t, filepath.Join(noCacheDir, ".ralfrc.json"), configJSON)
		writeTestFile(t, filepath.Join(noCacheDir, "a.js"), "var x = 1;")

		exitCode = 0
		configPath = ""

		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"lint", "--no-cache", "--config", filepath.Join(noCacheDir, ".ralfrc.json"), noCacheDir})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exitCode != ExitLintErrors {
			t.Fatalf("expected exit %d, got %d", ExitLintErrors, exitCode)
		}

		if _, err := os.Stat(filepath.Join(noCacheDir, ".ralf")); !os.IsNotExist(err) {
			t.Error("expected .ralf directory to not exist with --no-cache")
		}
	})
}
