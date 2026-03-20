package engine

import (
	"strconv"
	"strings"

	"github.com/Hideart/ralf/internal/parser"
)

// checkNoLossOfPrecision flags numeric literals that lose precision
// when represented as a float64.
func checkNoLossOfPrecision(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	text := node.Text(source)
	if text == "" {
		return
	}

	// Skip non-decimal prefixes: 0x, 0o, 0b are exact for their integer range.
	if len(text) >= 2 && text[0] == '0' {
		c := text[1] | 0x20 // lowercase
		if c == 'x' || c == 'o' || c == 'b' {
			return
		}
	}

	// Strip trailing 'n' for BigInt literals — these never lose precision.
	if strings.HasSuffix(text, "n") {
		return
	}

	// Strip numeric separators for parsing.
	cleaned := strings.ReplaceAll(text, "_", "")

	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return
	}

	// Infinity means the literal overflows float64.
	if val != val || val == val && (val > 1.7976931348623157e+308 || val < -1.7976931348623157e+308) {
		*diags = append(*diags, builtinDiag(node, lineStarts))
		return
	}

	// Round-trip: format the parsed float64 back and compare to the cleaned literal.
	reconstructed := strconv.FormatFloat(val, 'f', -1, 64)
	if isScientific(cleaned) {
		reconstructed = strconv.FormatFloat(val, 'e', -1, 64)
	}

	if normalizeNumericStr(cleaned) != normalizeNumericStr(reconstructed) {
		*diags = append(*diags, builtinDiag(node, lineStarts))
	}
}

func isScientific(s string) bool {
	return strings.ContainsAny(s, "eE")
}

// normalizeNumericStr strips leading/trailing zeros and signs for comparison.
func normalizeNumericStr(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimLeft(s, "+")

	// Split on 'e' for scientific notation.
	parts := strings.SplitN(s, "e", 2)
	mantissa := parts[0]

	// Normalize mantissa: strip trailing zeros after decimal point.
	if strings.Contains(mantissa, ".") {
		mantissa = strings.TrimRight(mantissa, "0")
		mantissa = strings.TrimRight(mantissa, ".")
	}

	if len(parts) == 2 {
		exp := parts[1]
		exp = strings.TrimLeft(exp, "+")
		if exp == "0" || exp == "-0" || exp == "" {
			return mantissa
		}
		return mantissa + "e" + exp
	}
	return mantissa
}
