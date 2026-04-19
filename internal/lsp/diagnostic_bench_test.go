package lsp

import (
	"testing"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/engine"
)

func BenchmarkConvertDiagnostics_10(b *testing.B) {
	benchConvert(b, 10)
}

func BenchmarkConvertDiagnostics_100(b *testing.B) {
	benchConvert(b, 100)
}

func benchConvert(b *testing.B, n int) {
	b.Helper()

	// ~80-char lines, 200 lines of ASCII JS — realistic file.
	source := make([]byte, 0, 200*80)
	for range 200 {
		source = append(source, []byte("const x = someFunction(arg1, arg2, arg3); // comment padding\n")...)
	}

	diags := make([]engine.Diagnostic, n)
	for i := range n {
		line := (i*7)%200 + 1 // spread across lines
		diags[i] = engine.Diagnostic{
			File:     "/tmp/bench.js",
			Line:     line,
			Col:      6,
			EndLine:  line,
			EndCol:   20,
			Rule:     "no-var",
			Message:  "Use let or const instead of var",
			Severity: config.SeverityError,
		}
	}

	b.ResetTimer()
	for range b.N {
		convertDiagnostics(diags, source)
	}
}

func BenchmarkBuildLineIndex(b *testing.B) {
	source := make([]byte, 0, 200*80)
	for range 200 {
		source = append(source, []byte("const x = someFunction(arg1, arg2, arg3); // comment padding\n")...)
	}

	b.ResetTimer()
	b.SetBytes(int64(len(source)))

	for range b.N {
		buildLineIndex(source)
	}
}

func BenchmarkByteColToUTF16_ASCII(b *testing.B) {
	source := []byte("const fooBarBaz = calculateValue(alpha, beta, gamma);\n")
	lineStarts := buildLineIndex(source)

	b.ResetTimer()
	for range b.N {
		byteColToUTF16(source, lineStarts, 1, 30)
	}
}

func BenchmarkByteColToUTF16_Multibyte(b *testing.B) {
	// Line with mixed ASCII and multibyte: "const café_naïve = 'über';\n"
	source := []byte("const caf\xc3\xa9_na\xc3\xafve = '\xc3\xbcber';\n")
	lineStarts := buildLineIndex(source)

	b.ResetTimer()
	for range b.N {
		byteColToUTF16(source, lineStarts, 1, 25)
	}
}

func BenchmarkPositionToByteOffset_ASCII(b *testing.B) {
	source := []byte("const fooBarBaz = calculateValue(alpha, beta, gamma);\n")
	lineStarts := buildLineIndex(source)
	pos := Position{Line: 0, Character: 30}

	b.ResetTimer()
	for range b.N {
		positionToByteOffset(source, lineStarts, pos)
	}
}

func BenchmarkPositionToByteOffset_Multibyte(b *testing.B) {
	// Mirror BenchmarkByteColToUTF16_Multibyte: "const café_naïve = 'über';\n"
	source := []byte("const caf\xc3\xa9_na\xc3\xafve = '\xc3\xbcber';\n")
	lineStarts := buildLineIndex(source)
	pos := Position{Line: 0, Character: 22}

	b.ResetTimer()
	for range b.N {
		positionToByteOffset(source, lineStarts, pos)
	}
}

func BenchmarkPositionToByteOffset_MultiLineASCII(b *testing.B) {
	source := make([]byte, 0, 200*80)
	for range 200 {
		source = append(source, []byte("const x = someFunction(arg1, arg2, arg3); // comment padding\n")...)
	}
	lineStarts := buildLineIndex(source)
	pos := Position{Line: 150, Character: 20}

	b.ResetTimer()
	for range b.N {
		positionToByteOffset(source, lineStarts, pos)
	}
}
