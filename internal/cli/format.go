package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/engine"
)

// Formatter writes diagnostics to a writer in a specific format.
type Formatter interface {
	Format(w io.Writer, diagnostics []engine.Diagnostic) error
}

// newFormatter returns the formatter for the given name.
func newFormatter(name string) (Formatter, error) {
	switch name {
	case "stylish":
		return stylishFormat{}, nil
	case "json":
		return jsonFormat{}, nil
	case "compact":
		return compactFormat{}, nil
	case "github":
		return githubFormat{}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (available: stylish, json, compact, github)", name)
	}
}

// --- Stylish formatter (ESLint-style, default) ---

type stylishFormat struct{}

// Format writes ESLint-style grouped output with a summary line.
func (stylishFormat) Format(w io.Writer, diagnostics []engine.Diagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}

	var errors, warnings int
	currentFile := ""

	for _, d := range diagnostics {
		if d.File != currentFile {
			if currentFile != "" {
				if _, err := fmt.Fprintln(w); err != nil {
					return err
				}
			}
			currentFile = d.File
			if _, err := fmt.Fprintln(w, formatPath(d.File)); err != nil {
				return err
			}
		}

		sev := severityLabel(d.Severity)
		// Display col as 1-based.
		if _, err := fmt.Fprintf(w, "  %d:%d  %s  %s  %s\n", d.Line, d.Col+1, sev, d.Message, d.Rule); err != nil {
			return err
		}

		if d.Severity == config.SeverityError {
			errors++
		} else {
			warnings++
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	total := errors + warnings
	if _, err := fmt.Fprintf(w, "\u2716 %d %s (%d %s, %d %s)\n",
		total, pluralize("problem", total),
		errors, pluralize("error", errors),
		warnings, pluralize("warning", warnings),
	); err != nil {
		return err
	}

	return nil
}

// --- JSON formatter ---

type jsonFormat struct{}

type jsonDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Col      int    `json:"col"`
	EndLine  int    `json:"endLine"`
	EndCol   int    `json:"endCol"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// Format writes a JSON array of diagnostic objects.
func (jsonFormat) Format(w io.Writer, diagnostics []engine.Diagnostic) error {
	items := make([]jsonDiagnostic, len(diagnostics))
	for i, d := range diagnostics {
		items[i] = jsonDiagnostic{
			File:     d.File,
			Line:     d.Line,
			Col:      d.Col + 1, // 1-based for output
			EndLine:  d.EndLine,
			EndCol:   d.EndCol + 1,
			Rule:     d.Rule,
			Message:  d.Message,
			Severity: string(d.Severity),
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(items)
}

// --- Compact formatter (one line per diagnostic, grep-friendly) ---

type compactFormat struct{}

// Format writes one line per diagnostic in grep-friendly format.
func (compactFormat) Format(w io.Writer, diagnostics []engine.Diagnostic) error {
	for _, d := range diagnostics {
		sev := severityLabel(d.Severity)
		if _, err := fmt.Fprintf(w, "%s:%d:%d: %s [%s] %s\n",
			formatPath(d.File), d.Line, d.Col+1, sev, d.Rule, d.Message); err != nil {
			return err
		}
	}
	return nil
}

// --- GitHub Actions formatter ---

type githubFormat struct{}

// Format writes GitHub Actions workflow command annotations.
func (githubFormat) Format(w io.Writer, diagnostics []engine.Diagnostic) error {
	for _, d := range diagnostics {
		level := "error"
		if d.Severity == config.SeverityWarn {
			level = "warning"
		}
		if _, err := fmt.Fprintf(w, "::%s file=%s,line=%d,col=%d::%s (%s)\n",
			level, formatPath(d.File), d.Line, d.Col+1, d.Message, d.Rule); err != nil {
			return err
		}
	}
	return nil
}

// --- Helpers ---

func severityLabel(s config.Severity) string {
	if s == config.SeverityError {
		return "error"
	}
	return "warn"
}

func pluralize(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// cachedCwd is resolved once and reused across formatPath calls to avoid
// a syscall per diagnostic.
var cachedCwd string

func init() {
	cachedCwd, _ = filepath.Abs(".")
}

// formatPath attempts to show a relative path for readability.
func formatPath(absPath string) string {
	if cachedCwd == "" {
		return absPath
	}
	rel, err := filepath.Rel(cachedCwd, absPath)
	if err != nil {
		return absPath
	}
	return rel
}
