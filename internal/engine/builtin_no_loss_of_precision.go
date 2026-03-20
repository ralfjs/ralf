package engine

import (
	"math"
	"strconv"
	"strings"

	"github.com/Hideart/ralf/internal/parser"
)

// checkNoLossOfPrecision flags numeric literals that lose precision
// when represented as a float64. Handles decimal, hex, octal, and binary.
func checkNoLossOfPrecision(node parser.Node, source []byte, lineStarts []int, diags *[]Diagnostic) {
	text := node.Text(source)
	if text == "" {
		return
	}

	// BigInt literals never lose precision.
	if strings.HasSuffix(text, "n") {
		return
	}

	// Strip numeric separators for parsing.
	cleaned := strings.ReplaceAll(text, "_", "")

	val, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		// Try parsing as integer for large hex/octal/binary that ParseFloat rejects.
		intVal, intErr := strconv.ParseInt(cleaned, 0, 64)
		if intErr != nil {
			// Also try unsigned for very large values.
			uintVal, uintErr := strconv.ParseUint(cleaned, 0, 64)
			if uintErr != nil {
				return
			}
			val = float64(uintVal)
			if uint64(val) != uintVal {
				*diags = append(*diags, builtinDiag(node, lineStarts))
			}
			return
		}
		val = float64(intVal)
		if int64(val) != intVal {
			*diags = append(*diags, builtinDiag(node, lineStarts))
		}
		return
	}

	// Infinity/NaN means the literal overflows float64.
	if math.IsInf(val, 0) || math.IsNaN(val) {
		*diags = append(*diags, builtinDiag(node, lineStarts))
		return
	}

	// For hex/octal/binary integers, do an integer round-trip check.
	lower := strings.ToLower(cleaned)
	if len(lower) >= 2 && lower[0] == '0' && (lower[1] == 'x' || lower[1] == 'o' || lower[1] == 'b') {
		intVal, err := strconv.ParseInt(cleaned, 0, 64)
		if err == nil {
			if int64(val) != intVal {
				*diags = append(*diags, builtinDiag(node, lineStarts))
			}
			return
		}
		uintVal, err := strconv.ParseUint(cleaned, 0, 64)
		if err == nil {
			if uint64(val) != uintVal {
				*diags = append(*diags, builtinDiag(node, lineStarts))
			}
		}
		return
	}

	// Decimal: round-trip float64 → string and compare.
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
