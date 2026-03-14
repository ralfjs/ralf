package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hideart/bepro/internal/config"
	"github.com/Hideart/bepro/internal/engine"
)

func TestStylishFormatter(t *testing.T) {
	f := stylishFormat{}

	t.Run("empty diagnostics", func(t *testing.T) {
		var buf bytes.Buffer
		if err := f.Format(&buf, nil); err != nil {
			t.Fatal(err)
		}
		if buf.Len() != 0 {
			t.Errorf("expected empty output, got %q", buf.String())
		}
	})

	t.Run("single diagnostic", func(t *testing.T) {
		var buf bytes.Buffer
		diags := []engine.Diagnostic{
			{File: "/abs/src/index.js", Line: 3, Col: 4, Rule: "no-var", Message: "No var", Severity: config.SeverityError},
		}
		if err := f.Format(&buf, diags); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "3:5") { // col displayed as 1-based
			t.Errorf("expected 1-based col 5, got: %s", out)
		}
		if !strings.Contains(out, "error") {
			t.Errorf("expected 'error', got: %s", out)
		}
		if !strings.Contains(out, "no-var") {
			t.Errorf("expected rule name, got: %s", out)
		}
		if !strings.Contains(out, "1 problem") {
			t.Errorf("expected summary, got: %s", out)
		}
	})

	t.Run("multi-file summary", func(t *testing.T) {
		var buf bytes.Buffer
		diags := []engine.Diagnostic{
			{File: "/abs/a.js", Line: 1, Col: 0, Rule: "r1", Message: "M1", Severity: config.SeverityError},
			{File: "/abs/a.js", Line: 2, Col: 0, Rule: "r2", Message: "M2", Severity: config.SeverityWarn},
			{File: "/abs/b.js", Line: 1, Col: 0, Rule: "r1", Message: "M1", Severity: config.SeverityError},
		}
		if err := f.Format(&buf, diags); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "3 problems") {
			t.Errorf("expected '3 problems', got: %s", out)
		}
		if !strings.Contains(out, "2 errors") {
			t.Errorf("expected '2 errors', got: %s", out)
		}
		if !strings.Contains(out, "1 warning") {
			t.Errorf("expected '1 warning', got: %s", out)
		}
	})
}

func TestJSONFormatter(t *testing.T) {
	f := jsonFormat{}

	var buf bytes.Buffer
	diags := []engine.Diagnostic{
		{File: "/src/index.js", Line: 3, Col: 4, EndLine: 3, EndCol: 7, Rule: "no-var", Message: "No var", Severity: config.SeverityError},
	}
	if err := f.Format(&buf, diags); err != nil {
		t.Fatal(err)
	}

	var parsed []jsonDiagnostic
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}
	if len(parsed) != 1 {
		t.Fatalf("expected 1 item, got %d", len(parsed))
	}
	if parsed[0].Col != 5 { // 1-based
		t.Errorf("expected col 5, got %d", parsed[0].Col)
	}
}

func TestCompactFormatter(t *testing.T) {
	f := compactFormat{}

	var buf bytes.Buffer
	diags := []engine.Diagnostic{
		{File: "/src/index.js", Line: 3, Col: 4, Rule: "no-var", Message: "No var", Severity: config.SeverityError},
	}
	if err := f.Format(&buf, diags); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, ":3:5:") { // 1-based col
		t.Errorf("expected :3:5:, got: %s", out)
	}
	if !strings.Contains(out, "[no-var]") {
		t.Errorf("expected [no-var], got: %s", out)
	}
}

func TestGithubFormatter(t *testing.T) {
	f := githubFormat{}

	var buf bytes.Buffer
	diags := []engine.Diagnostic{
		{File: "/src/index.js", Line: 3, Col: 4, Rule: "no-var", Message: "No var", Severity: config.SeverityError},
		{File: "/src/index.js", Line: 7, Col: 0, Rule: "no-magic", Message: "Extract", Severity: config.SeverityWarn},
	}
	if err := f.Format(&buf, diags); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "::error ") {
		t.Errorf("expected ::error prefix, got: %s", lines[0])
	}
	if !strings.HasPrefix(lines[1], "::warning ") {
		t.Errorf("expected ::warning prefix, got: %s", lines[1])
	}
}

func TestNewFormatterUnknown(t *testing.T) {
	_, err := newFormatter("nope")
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}
