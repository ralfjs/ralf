package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/engine"
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

func TestSARIFFormatter(t *testing.T) {
	f := sarifFormat{}

	t.Run("empty diagnostics", func(t *testing.T) {
		var buf bytes.Buffer
		if err := f.Format(&buf, nil); err != nil {
			t.Fatal(err)
		}
		var parsed sarifLog
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed.Version != "2.1.0" {
			t.Errorf("expected version 2.1.0, got %q", parsed.Version)
		}
		if len(parsed.Runs) != 1 {
			t.Fatalf("expected 1 run, got %d", len(parsed.Runs))
		}
		if len(parsed.Runs[0].Results) != 0 {
			t.Errorf("expected 0 results, got %d", len(parsed.Runs[0].Results))
		}
		if len(parsed.Runs[0].Tool.Driver.Rules) != 0 {
			t.Errorf("expected 0 rules, got %d", len(parsed.Runs[0].Tool.Driver.Rules))
		}
	})

	t.Run("basic diagnostic", func(t *testing.T) {
		var buf bytes.Buffer
		diags := []engine.Diagnostic{
			{File: "/src/index.js", Line: 3, Col: 4, EndLine: 3, EndCol: 7, Rule: "no-var", Message: "Use let or const", Severity: config.SeverityError},
		}
		if err := f.Format(&buf, diags); err != nil {
			t.Fatal(err)
		}
		var parsed sarifLog
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed.Schema != "https://json.schemastore.org/sarif-2.1.0.json" {
			t.Errorf("unexpected schema: %s", parsed.Schema)
		}
		run := parsed.Runs[0]
		if run.Tool.Driver.Name != "ralf" {
			t.Errorf("expected tool name ralf, got %q", run.Tool.Driver.Name)
		}
		if run.Tool.Driver.SemanticVersion == "" {
			t.Error("expected semanticVersion to be set")
		}
		if len(run.Tool.Driver.Rules) != 1 {
			t.Fatalf("expected 1 rule, got %d", len(run.Tool.Driver.Rules))
		}
		if run.Tool.Driver.Rules[0].ID != "no-var" {
			t.Errorf("expected rule id no-var, got %q", run.Tool.Driver.Rules[0].ID)
		}
		if len(run.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(run.Results))
		}
		r := run.Results[0]
		if r.RuleID != "no-var" {
			t.Errorf("expected ruleId no-var, got %q", r.RuleID)
		}
		if r.RuleIndex != 0 {
			t.Errorf("expected ruleIndex 0, got %d", r.RuleIndex)
		}
		if r.Level != "error" {
			t.Errorf("expected level error, got %q", r.Level)
		}
		loc := r.Locations[0].PhysicalLocation
		if loc.Region.StartColumn != 5 { // 0-based → 1-based
			t.Errorf("expected startColumn 5, got %d", loc.Region.StartColumn)
		}
		if loc.ArtifactLocation.URIBaseID != "%SRCROOT%" {
			t.Errorf("expected uriBaseId %%SRCROOT%%, got %q", loc.ArtifactLocation.URIBaseID)
		}
		fp, ok := r.PartialFingerprints["primaryLocationLineHash"]
		if !ok || fp == "" {
			t.Error("expected partialFingerprints.primaryLocationLineHash to be set")
		}
	})

	t.Run("fingerprint stability", func(t *testing.T) {
		diags := []engine.Diagnostic{
			{File: "/src/index.js", Line: 3, Col: 4, EndLine: 3, EndCol: 7, Rule: "no-var", Message: "Use let or const", Severity: config.SeverityError},
		}
		var buf1, buf2 bytes.Buffer
		if err := f.Format(&buf1, diags); err != nil {
			t.Fatal(err)
		}
		if err := f.Format(&buf2, diags); err != nil {
			t.Fatal(err)
		}
		var p1, p2 sarifLog
		if err := json.Unmarshal(buf1.Bytes(), &p1); err != nil {
			t.Fatalf("invalid JSON run 1: %v", err)
		}
		if err := json.Unmarshal(buf2.Bytes(), &p2); err != nil {
			t.Fatalf("invalid JSON run 2: %v", err)
		}
		fp1 := p1.Runs[0].Results[0].PartialFingerprints["primaryLocationLineHash"]
		fp2 := p2.Runs[0].Results[0].PartialFingerprints["primaryLocationLineHash"]
		if fp1 != fp2 {
			t.Errorf("fingerprints not stable across runs: %q vs %q", fp1, fp2)
		}
	})

	t.Run("multi-rule deduplication", func(t *testing.T) {
		var buf bytes.Buffer
		diags := []engine.Diagnostic{
			{File: "/src/a.js", Line: 1, Col: 0, EndLine: 1, EndCol: 3, Rule: "no-var", Message: "Use let or const", Severity: config.SeverityError},
			{File: "/src/a.js", Line: 2, Col: 0, EndLine: 2, EndCol: 3, Rule: "no-console", Message: "No console", Severity: config.SeverityWarn},
			{File: "/src/b.js", Line: 5, Col: 0, EndLine: 5, EndCol: 3, Rule: "no-var", Message: "Use let or const", Severity: config.SeverityError},
		}
		if err := f.Format(&buf, diags); err != nil {
			t.Fatal(err)
		}
		var parsed sarifLog
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		run := parsed.Runs[0]
		if len(run.Tool.Driver.Rules) != 2 {
			t.Fatalf("expected 2 deduplicated rules, got %d", len(run.Tool.Driver.Rules))
		}
		if len(run.Results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(run.Results))
		}
		// Third result should reference rule index 0 (no-var).
		if run.Results[2].RuleIndex != 0 {
			t.Errorf("expected ruleIndex 0 for third result, got %d", run.Results[2].RuleIndex)
		}
	})

	t.Run("with fix", func(t *testing.T) {
		var buf bytes.Buffer
		diags := []engine.Diagnostic{
			{
				File: "/src/index.js", Line: 3, Col: 0, EndLine: 3, EndCol: 3,
				Rule: "no-var", Message: "Use let or const", Severity: config.SeverityError,
				Fix: &engine.Fix{StartByte: 10, EndByte: 13, NewText: "const"},
			},
		}
		if err := f.Format(&buf, diags); err != nil {
			t.Fatal(err)
		}
		var parsed sarifLog
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		r := parsed.Runs[0].Results[0]
		if len(r.Fixes) != 1 {
			t.Fatalf("expected 1 fix, got %d", len(r.Fixes))
		}
		repl := r.Fixes[0].ArtifactChanges[0].Replacements[0]
		if repl.DeletedRegion.ByteOffset != 10 {
			t.Errorf("expected byteOffset 10, got %d", repl.DeletedRegion.ByteOffset)
		}
		if repl.DeletedRegion.ByteLength != 3 {
			t.Errorf("expected byteLength 3, got %d", repl.DeletedRegion.ByteLength)
		}
		if repl.InsertedContent.Text != "const" {
			t.Errorf("expected inserted text 'const', got %q", repl.InsertedContent.Text)
		}
	})

	t.Run("severity levels", func(t *testing.T) {
		var buf bytes.Buffer
		diags := []engine.Diagnostic{
			{File: "/src/a.js", Line: 1, Col: 0, EndLine: 1, EndCol: 1, Rule: "r1", Message: "M1", Severity: config.SeverityError},
			{File: "/src/a.js", Line: 2, Col: 0, EndLine: 2, EndCol: 1, Rule: "r2", Message: "M2", Severity: config.SeverityWarn},
		}
		if err := f.Format(&buf, diags); err != nil {
			t.Fatal(err)
		}
		var parsed sarifLog
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed.Runs[0].Results[0].Level != "error" {
			t.Errorf("expected error, got %q", parsed.Runs[0].Results[0].Level)
		}
		if parsed.Runs[0].Results[1].Level != "warning" {
			t.Errorf("expected warning, got %q", parsed.Runs[0].Results[1].Level)
		}
	})
}

func TestNewFormatterUnknown(t *testing.T) {
	_, err := newFormatter("nope")
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

// --- Benchmarks ---

// benchDiags builds n diagnostics spread across numFiles files with numRules
// distinct rules. Every 3rd diagnostic includes a Fix.
func benchDiags(n, numFiles, numRules int) []engine.Diagnostic {
	diags := make([]engine.Diagnostic, n)
	rules := make([]string, numRules)
	for i := range numRules {
		rules[i] = fmt.Sprintf("rule-%d", i)
	}
	for i := range n {
		d := engine.Diagnostic{
			File:     fmt.Sprintf("/src/file-%d.js", i%numFiles),
			Line:     i + 1,
			Col:      4,
			EndLine:  i + 1,
			EndCol:   10,
			Rule:     rules[i%numRules],
			Message:  "Diagnostic message for benchmarking",
			Severity: config.SeverityError,
		}
		if i%3 == 0 {
			d.Fix = &engine.Fix{StartByte: i * 20, EndByte: i*20 + 3, NewText: "const"}
		}
		if i%2 == 0 {
			d.Severity = config.SeverityWarn
		}
		diags[i] = d
	}
	return diags
}

func BenchmarkFormatters(b *testing.B) {
	diags := benchDiags(1000, 50, 20)

	formatters := []struct {
		name string
		f    Formatter
	}{
		{"stylish", stylishFormat{}},
		{"json", jsonFormat{}},
		{"compact", compactFormat{}},
		{"github", githubFormat{}},
		{"sarif", sarifFormat{}},
	}

	for _, tc := range formatters {
		b.Run(tc.name, func(b *testing.B) {
			var buf bytes.Buffer
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				buf.Reset()
				if err := tc.f.Format(&buf, diags); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSARIFFormatter_Large(b *testing.B) {
	diags := benchDiags(10000, 200, 50)
	var buf bytes.Buffer
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := (sarifFormat{}).Format(&buf, diags); err != nil {
			b.Fatal(err)
		}
	}
}
