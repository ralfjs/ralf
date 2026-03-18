package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadJS_ModuleExports(t *testing.T) {
	t.Parallel()
	cfg, err := LoadFile(filepath.Join(testdataDir, "valid.js"))
	if err != nil {
		t.Fatalf("LoadFile JS: %v", err)
	}
	assertValidConfig(t, cfg)
}

func TestLoadJS_ExportDefault(t *testing.T) {
	t.Parallel()
	cfg, err := LoadFile(filepath.Join(testdataDir, "export-default.js"))
	if err != nil {
		t.Fatalf("LoadFile JS export default: %v", err)
	}
	assertValidConfig(t, cfg)
}

func TestLoadJS_ComputedValues(t *testing.T) {
	t.Parallel()
	cfg, err := loadJS("computed.js", []byte(`
		var isStrict = true;
		module.exports = {
			rules: {
				"no-eval": {
					regex: "\\beval\\b",
					severity: isStrict ? "error" : "warn"
				}
			}
		};
	`))
	if err != nil {
		t.Fatalf("loadJS computed: %v", err)
	}
	rule, ok := cfg.Rules["no-eval"]
	if !ok {
		t.Fatal("expected rule 'no-eval'")
	}
	if rule.Severity != SeverityError {
		t.Errorf("severity = %q, want %q", rule.Severity, SeverityError)
	}
}

func TestLoadJS_SyntaxError(t *testing.T) {
	t.Parallel()
	_, err := loadJS("bad.js", []byte(`module.exports = {`))
	if err == nil {
		t.Fatal("expected error for syntax error")
	}
	if !strings.Contains(err.Error(), "eval JS") {
		t.Errorf("expected eval error, got: %v", err)
	}
}

func TestLoadJS_RuntimeError(t *testing.T) {
	t.Parallel()
	_, err := loadJS("throw.js", []byte(`throw new Error("boom");`))
	if err == nil {
		t.Fatal("expected error for runtime throw")
	}
	if !strings.Contains(err.Error(), "eval JS") {
		t.Errorf("expected eval error, got: %v", err)
	}
}

func TestLoadJS_NonObjectExport(t *testing.T) {
	t.Parallel()
	_, err := loadJS("number.js", []byte(`module.exports = 42;`))
	if err == nil {
		t.Fatal("expected error for non-object export")
	}
	if !strings.Contains(err.Error(), "must be an object") {
		t.Errorf("expected object error, got: %v", err)
	}
}

func TestLoadJS_NullExport(t *testing.T) {
	t.Parallel()
	_, err := loadJS("null.js", []byte(`module.exports = null;`))
	if err == nil {
		t.Fatal("expected error for null export")
	}
}

func TestLoadJS_UnsetExport(t *testing.T) {
	t.Parallel()
	// Script never sets module.exports — the initial empty object remains.
	cfg, err := loadJS("noop.js", []byte(`var x = 1;`))
	if err != nil {
		t.Fatalf("loadJS unset: %v", err)
	}
	if len(cfg.Rules) != 0 {
		t.Errorf("expected 0 rules from empty export, got %d", len(cfg.Rules))
	}
}

func TestLoadJS_InfiniteLoop(t *testing.T) {
	t.Parallel()
	_, err := loadJS("loop.js", []byte(`while(true){} module.exports = {};`))
	if err == nil {
		t.Fatal("expected error for infinite loop")
	}
	if !strings.Contains(err.Error(), "eval JS") {
		t.Errorf("expected eval/timeout error, got: %v", err)
	}
}

func TestShimExportDefault(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "has export default",
			input:  "export default { rules: {} };",
			expect: "module.exports = { rules: {} };",
		},
		{
			name:   "no export default",
			input:  "module.exports = { rules: {} };",
			expect: "module.exports = { rules: {} };",
		},
		{
			name:   "only first occurrence",
			input:  "export default { rules: {} };\nexport default { rules: {} };",
			expect: "module.exports = { rules: {} };\nexport default { rules: {} };",
		},
		{
			name:   "inside comment not matched",
			input:  "// Don't use export default in libraries\nexport default { rules: {} };",
			expect: "// Don't use export default in libraries\nmodule.exports = { rules: {} };",
		},
		{
			name:   "preserves indentation",
			input:  "  export default { rules: {} };",
			expect: "  module.exports = { rules: {} };",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shimExportDefault(tt.input)
			if got != tt.expect {
				t.Errorf("shimExportDefault(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
