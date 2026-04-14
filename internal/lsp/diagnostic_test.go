package lsp

import (
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

func TestConvertDiagnostics_Empty(t *testing.T) {
	t.Parallel()

	result := convertDiagnostics(nil, []byte("hello"))
	if result == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(result))
	}
}

func TestConvertDiagnostics_ASCII(t *testing.T) {
	t.Parallel()

	source := []byte("var x = 1;\nconst y = 2;\n")
	diags := []engine.Diagnostic{
		{
			File:     "/tmp/test.js",
			Line:     1,
			Col:      0,
			EndLine:  1,
			EndCol:   3,
			Rule:     "no-var",
			Message:  "Use let or const",
			Severity: config.SeverityError,
		},
	}

	result := convertDiagnostics(diags, source)
	if len(result) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result))
	}

	d := result[0]
	if d.Range.Start.Line != 0 {
		t.Errorf("start line: want 0, got %d", d.Range.Start.Line)
	}
	if d.Range.Start.Character != 0 {
		t.Errorf("start char: want 0, got %d", d.Range.Start.Character)
	}
	if d.Range.End.Line != 0 {
		t.Errorf("end line: want 0, got %d", d.Range.End.Line)
	}
	if d.Range.End.Character != 3 {
		t.Errorf("end char: want 3, got %d", d.Range.End.Character)
	}
	if d.Code != "no-var" {
		t.Errorf("code: want no-var, got %q", d.Code)
	}
	if d.Source != "ralf" {
		t.Errorf("source: want ralf, got %q", d.Source)
	}
	if d.Severity != SeverityError {
		t.Errorf("severity: want %d, got %d", SeverityError, d.Severity)
	}
}

func TestConvertDiagnostics_SecondLine(t *testing.T) {
	t.Parallel()

	source := []byte("const a = 1;\nvar b = 2;\n")
	diags := []engine.Diagnostic{
		{
			File:     "/tmp/test.js",
			Line:     2,
			Col:      0,
			EndLine:  2,
			EndCol:   3,
			Rule:     "no-var",
			Message:  "Use let or const",
			Severity: config.SeverityWarn,
		},
	}

	result := convertDiagnostics(diags, source)
	if len(result) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(result))
	}

	d := result[0]
	if d.Range.Start.Line != 1 {
		t.Errorf("start line: want 1, got %d", d.Range.Start.Line)
	}
	if d.Range.Start.Character != 0 {
		t.Errorf("start char: want 0, got %d", d.Range.Start.Character)
	}
	if d.Severity != SeverityWarning {
		t.Errorf("severity: want %d, got %d", SeverityWarning, d.Severity)
	}
}

func TestMapSeverity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   config.Severity
		want DiagnosticSeverity
	}{
		{config.SeverityError, SeverityError},
		{config.SeverityWarn, SeverityWarning},
		{config.SeverityOff, SeverityWarning},
	}

	for _, tt := range tests {
		got := mapSeverity(tt.in)
		if got != tt.want {
			t.Errorf("mapSeverity(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestByteColToUTF16_ASCII(t *testing.T) {
	t.Parallel()

	source := []byte("hello world\n")
	lineStarts := buildLineIndex(source)

	got := byteColToUTF16(source, lineStarts, 1, 5)
	if got != 5 {
		t.Errorf("ASCII: want 5, got %d", got)
	}
}

func TestByteColToUTF16_TwoByte(t *testing.T) {
	t.Parallel()

	// "café" — é is 2 bytes in UTF-8 but 1 UTF-16 code unit
	source := []byte("caf\xc3\xa9 = 1;\n")
	lineStarts := buildLineIndex(source)

	// byte offset 6 (after "café ") = 5 UTF-16 characters (c,a,f,é,space)
	got := byteColToUTF16(source, lineStarts, 1, 6)
	if got != 5 {
		t.Errorf("2-byte UTF-8: want 5, got %d", got)
	}
}

func TestByteColToUTF16_ThreeByte(t *testing.T) {
	t.Parallel()

	// "→x" — → (U+2192) is 3 bytes in UTF-8, 1 UTF-16 code unit
	source := []byte("\xe2\x86\x92x\n")
	lineStarts := buildLineIndex(source)

	// byte offset 3 (after →) = 1 UTF-16 character
	got := byteColToUTF16(source, lineStarts, 1, 3)
	if got != 1 {
		t.Errorf("3-byte UTF-8: want 1, got %d", got)
	}
}

func TestByteColToUTF16_FourByte(t *testing.T) {
	t.Parallel()

	// "𝕏x" — 𝕏 (U+1D54F) is 4 bytes UTF-8, 2 UTF-16 code units (surrogate pair)
	source := []byte("\xf0\x9d\x95\x8fx\n")
	lineStarts := buildLineIndex(source)

	// byte offset 4 (after 𝕏) = 2 UTF-16 code units
	got := byteColToUTF16(source, lineStarts, 1, 4)
	if got != 2 {
		t.Errorf("4-byte UTF-8 (surrogate pair): want 2, got %d", got)
	}

	// byte offset 5 (after 𝕏x) = 3 UTF-16 code units
	got = byteColToUTF16(source, lineStarts, 1, 5)
	if got != 3 {
		t.Errorf("4-byte + ASCII: want 3, got %d", got)
	}
}

func TestByteColToUTF16_MidRune(t *testing.T) {
	t.Parallel()

	// "café" — é is 2 bytes (\xc3\xa9). A byteCol landing between the two
	// bytes of é must not read past end and should treat the partial rune as
	// a single RuneError (1 UTF-16 unit).
	source := []byte("caf\xc3\xa9\n")
	lineStarts := buildLineIndex(source)

	// byteCol=4 lands on the second byte of é (the \xa9).
	got := byteColToUTF16(source, lineStarts, 1, 4)
	// Bytes 0..3: c(1) a(1) f(1) + 1 byte of é decoded as RuneError → 4 UTF-16 units.
	if got != 4 {
		t.Errorf("mid-rune byteCol: want 4, got %d", got)
	}
}

func TestByteColToUTF16_InvalidLine(t *testing.T) {
	t.Parallel()

	source := []byte("hello\n")
	lineStarts := buildLineIndex(source)

	// Line 99 doesn't exist — should fall back to returning byteCol.
	got := byteColToUTF16(source, lineStarts, 99, 3)
	if got != 3 {
		t.Errorf("invalid line: want 3 (fallback), got %d", got)
	}
}

func TestBuildLineIndex(t *testing.T) {
	t.Parallel()

	source := []byte("abc\ndef\nghi\n")
	starts := buildLineIndex(source)

	// 3 newlines produce 4 line starts: 0 (abc), 4 (def), 8 (ghi), 12 (empty after trailing \n)
	want := []int{0, 4, 8, 12}
	if len(starts) != len(want) {
		t.Fatalf("line count: want %d, got %d", len(want), len(starts))
	}
	for i, w := range want {
		if starts[i] != w {
			t.Errorf("line %d start: want %d, got %d", i+1, w, starts[i])
		}
	}
}
