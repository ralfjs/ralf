package lsp

import (
	"testing"
	"unicode/utf8"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

func TestPositionToByteOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source string
		line   int
		char   int
		want   int
	}{
		// ASCII, single line
		{"ascii start", "hello", 0, 0, 0},
		{"ascii mid", "hello", 0, 3, 3},
		{"ascii end", "hello", 0, 5, 5},
		{"ascii char past end clamps", "hello", 0, 99, 5},

		// ASCII, multiple lines — line end stops before '\n'
		{"line 0 start", "hello\nworld", 0, 0, 0},
		{"line 0 end (before newline)", "hello\nworld", 0, 5, 5},
		{"line 0 past end clamps", "hello\nworld", 0, 99, 5},
		{"line 1 start", "hello\nworld", 1, 0, 6},
		{"line 1 end", "hello\nworld", 1, 5, 11},
		{"line 1 past end clamps", "hello\nworld", 1, 99, 11},

		// Out-of-range clamping
		{"line past end", "hello\nworld", 99, 0, 11},
		{"negative line", "hello\nworld", -1, 5, 0},
		{"negative char", "hello\nworld", 1, -1, 6},

		// Empty source / empty line
		{"empty source", "", 0, 0, 0},
		{"empty source, line past end", "", 5, 5, 0},
		{"empty line", "a\n\nb", 1, 0, 2},
		{"empty line char past end", "a\n\nb", 1, 5, 2},

		// Multi-byte UTF-8, single UTF-16 code unit: é = 0xC3 0xA9, 1 UTF-16 unit
		{"utf-8 é: before", "aéb", 0, 0, 0},
		{"utf-8 é: after a", "aéb", 0, 1, 1},
		{"utf-8 é: after é", "aéb", 0, 2, 3},
		{"utf-8 é: after b", "aéb", 0, 3, 4},

		// UTF-16 surrogate pair: 𝐀 (U+1D400) = 4 bytes UTF-8, 2 UTF-16 units
		{"surrogate: before", "a𝐀b", 0, 0, 0},
		{"surrogate: after a", "a𝐀b", 0, 1, 1},
		{"surrogate: inside pair clamps to rune start", "a𝐀b", 0, 2, 1},
		{"surrogate: after pair", "a𝐀b", 0, 3, 5},
		{"surrogate: after b", "a𝐀b", 0, 4, 6},

		// Round-trip spot check against byteOffsetToPosition
		{"after multi-byte and pair", "aé𝐀", 0, 4, 7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := []byte(tc.source)
			starts := buildLineIndex(src)
			got := positionToByteOffset(src, starts, Position{Line: tc.line, Character: tc.char})
			if got != tc.want {
				t.Errorf("source=%q pos={line:%d char:%d}: got %d, want %d",
					tc.source, tc.line, tc.char, got, tc.want)
			}
		})
	}
}

func TestPositionToByteOffset_RoundTrip(t *testing.T) {
	t.Parallel()

	// Each rune-boundary byte offset (including len(source)) should round-trip
	// through byteOffsetToPosition → positionToByteOffset back to itself.
	// Offsets inside a multi-byte UTF-8 sequence are not valid cursor positions
	// and are deliberately skipped.
	sources := []string{
		"hello\nworld",
		"a\nbb\nccc",
		"aéb\n𝐀",
		"",
		"x",
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			b := []byte(src)
			starts := buildLineIndex(b)
			for i := 0; i <= len(b); {
				pos := byteOffsetToPosition(b, starts, i)
				got := positionToByteOffset(b, starts, pos)
				if got != i {
					t.Errorf("offset %d → pos %+v → offset %d", i, pos, got)
				}
				if i == len(b) {
					break
				}
				_, size := utf8.DecodeRune(b[i:])
				if size <= 0 {
					size = 1
				}
				i += size
			}
		})
	}
}

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
