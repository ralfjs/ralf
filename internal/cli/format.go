package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/engine"
	"github.com/Hideart/ralf/internal/version"
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
	case "sarif":
		return sarifFormat{}, nil
	default:
		return nil, fmt.Errorf("unknown format %q (available: stylish, json, compact, github, sarif)", name)
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

		switch d.Severity {
		case config.SeverityError:
			errors++
		case config.SeverityWarn:
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

// --- SARIF v2.1.0 formatter ---

type sarifFormat struct{}

// SARIF struct types for JSON marshaling.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name            string      `json:"name"`
	SemanticVersion string      `json:"semanticVersion"`
	InformationURI  string      `json:"informationUri"`
	Rules           []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string   `json:"id"`
	ShortDescription sarifMsg `json:"shortDescription"`
}

type sarifMsg struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             sarifMsg          `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Fixes               []sarifFix        `json:"fixes,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysical `json:"physicalLocation"`
}

type sarifPhysical struct {
	ArtifactLocation sarifArtifact `json:"artifactLocation"`
	Region           sarifRegion   `json:"region"`
}

type sarifArtifact struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
	EndLine     int `json:"endLine"`
	EndColumn   int `json:"endColumn"`
}

type sarifFix struct {
	Description     sarifMsg              `json:"description"`
	ArtifactChanges []sarifArtifactChange `json:"artifactChanges"`
}

type sarifArtifactChange struct {
	ArtifactLocation sarifArtifact      `json:"artifactLocation"`
	Replacements     []sarifReplacement `json:"replacements"`
}

type sarifReplacement struct {
	DeletedRegion   sarifByteRegion `json:"deletedRegion"`
	InsertedContent sarifInserted   `json:"insertedContent,omitempty"`
}

type sarifByteRegion struct {
	ByteOffset int `json:"byteOffset"`
	ByteLength int `json:"byteLength"`
}

type sarifInserted struct {
	Text string `json:"text"`
}

// sarifBufPool reuses buffers across Format calls (helps in LSP hot path).
var sarifBufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

// Format writes SARIF v2.1.0 JSON output for GitHub Code Scanning integration.
// It writes JSON directly to avoid encoding/json reflection overhead on the
// deeply nested SARIF struct tree.
func (sarifFormat) Format(w io.Writer, diagnostics []engine.Diagnostic) error {
	// Build deduplicated rules index.
	type ruleEntry struct{ id, desc string }
	ruleIndex := make(map[string]int, len(diagnostics)/2+1)
	rules := make([]ruleEntry, 0, 32)
	for _, d := range diagnostics {
		if _, exists := ruleIndex[d.Rule]; !exists {
			ruleIndex[d.Rule] = len(rules)
			rules = append(rules, ruleEntry{d.Rule, d.Message})
		}
	}

	buf := sarifBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer sarifBufPool.Put(buf)

	var numBuf [20]byte
	var fp sarifFP

	// Envelope + tool header.
	buf.WriteString(`{"$schema":"https://json.schemastore.org/sarif-2.1.0.json","version":"2.1.0","runs":[{"tool":{"driver":{"name":"ralf","semanticVersion":`)
	appendJSONStr(buf, version.Version)
	buf.WriteString(`,"informationUri":"https://github.com/Hideart/ralf","rules":[`)

	// Rules array.
	for i, r := range rules {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.WriteString(`{"id":`)
		appendJSONStr(buf, r.id)
		buf.WriteString(`,"shortDescription":{"text":`)
		appendJSONStr(buf, r.desc)
		buf.WriteString(`}}`)
	}
	buf.WriteString(`]}},"results":[`)

	// Results array.
	for i, d := range diagnostics {
		if i > 0 {
			buf.WriteByte(',')
		}
		level := `"error"`
		if d.Severity == config.SeverityWarn {
			level = `"warning"`
		}
		uri := formatPath(d.File)

		buf.WriteString(`{"ruleId":`)
		appendJSONStr(buf, d.Rule)
		buf.WriteString(`,"ruleIndex":`)
		buf.Write(strconv.AppendInt(numBuf[:0], int64(ruleIndex[d.Rule]), 10))
		buf.WriteString(`,"level":`)
		buf.WriteString(level)
		buf.WriteString(`,"message":{"text":`)
		appendJSONStr(buf, d.Message)
		buf.WriteString(`},"locations":[{"physicalLocation":{"artifactLocation":{"uri":`)
		appendJSONStr(buf, uri)
		buf.WriteString(`,"uriBaseId":"%SRCROOT%"},"region":{"startLine":`)
		buf.Write(strconv.AppendInt(numBuf[:0], int64(d.Line), 10))
		buf.WriteString(`,"startColumn":`)
		buf.Write(strconv.AppendInt(numBuf[:0], int64(d.Col+1), 10))
		buf.WriteString(`,"endLine":`)
		buf.Write(strconv.AppendInt(numBuf[:0], int64(d.EndLine), 10))
		buf.WriteString(`,"endColumn":`)
		buf.Write(strconv.AppendInt(numBuf[:0], int64(d.EndCol+1), 10))
		buf.WriteString(`}}}],"partialFingerprints":{"primaryLocationLineHash":`)
		appendJSONStr(buf, fp.hash(d.Rule, uri, d.Line))
		buf.WriteByte('}')

		if d.Fix != nil {
			buf.WriteString(`,"fixes":[{"description":{"text":`)
			appendJSONStr(buf, "Apply fix for "+d.Rule)
			buf.WriteString(`},"artifactChanges":[{"artifactLocation":{"uri":`)
			appendJSONStr(buf, uri)
			buf.WriteString(`,"uriBaseId":"%SRCROOT%"},"replacements":[{"deletedRegion":{"byteOffset":`)
			buf.Write(strconv.AppendInt(numBuf[:0], int64(d.Fix.StartByte), 10))
			buf.WriteString(`,"byteLength":`)
			buf.Write(strconv.AppendInt(numBuf[:0], int64(d.Fix.EndByte-d.Fix.StartByte), 10))
			buf.WriteString(`},"insertedContent":{"text":`)
			appendJSONStr(buf, d.Fix.NewText)
			buf.WriteString(`}}]}]}]`)
		}

		buf.WriteByte('}')
	}

	buf.WriteString(`]}]}`)
	buf.WriteByte('\n')

	_, err := w.Write(buf.Bytes())
	return err
}

// sarifFP computes stable fingerprints for GitHub Code Scanning deduplication.
// GitHub uses "primaryLocationLineHash" to track alerts across runs.
// Reuses a key buffer across calls; sha256.Sum256 is stack-friendly.
type sarifFP struct {
	keyBuf bytes.Buffer
	hexBuf [sha256.Size * 2]byte
}

func (fp *sarifFP) hash(rule, uri string, line int) string {
	fp.keyBuf.Reset()
	fp.keyBuf.WriteString(rule)
	fp.keyBuf.WriteByte(0)
	fp.keyBuf.WriteString(uri)
	fp.keyBuf.WriteByte(0)
	var numBuf [20]byte
	fp.keyBuf.Write(strconv.AppendInt(numBuf[:0], int64(line), 10))
	sum := sha256.Sum256(fp.keyBuf.Bytes())
	hex.Encode(fp.hexBuf[:], sum[:])
	return string(fp.hexBuf[:])
}

// appendJSONStr writes a JSON-encoded string (with quotes) to buf.
// Fast path: scans for common case (no escaping needed) in a tight loop.
func appendJSONStr(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' {
			continue
		}
		buf.WriteString(s[start:i])
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			buf.WriteString(`\u00`)
			buf.WriteByte("0123456789abcdef"[c>>4])
			buf.WriteByte("0123456789abcdef"[c&0xf])
		}
		start = i + 1
	}
	buf.WriteString(s[start:])
	buf.WriteByte('"')
}
