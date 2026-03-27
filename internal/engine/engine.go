package engine

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"slices"

	"github.com/ralfjs/ralf/internal/config"
	"github.com/ralfjs/ralf/internal/parser"
	"golang.org/x/sync/errgroup"
)

// Engine compiles rules from config and runs them against source files.
type Engine struct {
	regexRules      []compiledRegex
	patternRules    []compiledPattern
	structuralRules []compiledStructural
	importRules     []compiledImport
	builtinRules    []compiledBuiltin
	cfg             *config.Config
}

// New creates an Engine from the given config. It compiles all regex, pattern,
// structural, and import rules (including naming constraints), returning any
// compilation errors.
func New(cfg *config.Config) (*Engine, []error) {
	regexCompiled, regexErrs := compileRegexRules(cfg.Rules)
	patternCompiled, patternErrs := compilePatternRules(cfg.Rules)
	structuralCompiled, structuralErrs := compileStructuralRules(cfg.Rules)
	importCompiled, importErrs := compileImportRules(cfg.Rules)
	builtinCompiled, builtinErrs := compileBuiltinRules(cfg.Rules)

	errs := make([]error, 0, len(regexErrs)+len(patternErrs)+len(structuralErrs)+len(importErrs)+len(builtinErrs))
	errs = append(errs, regexErrs...)
	errs = append(errs, patternErrs...)
	errs = append(errs, structuralErrs...)
	errs = append(errs, importErrs...)
	errs = append(errs, builtinErrs...)
	if len(errs) > 0 {
		return nil, errs
	}

	return &Engine{
		regexRules:      regexCompiled,
		patternRules:    patternCompiled,
		structuralRules: structuralCompiled,
		importRules:     importCompiled,
		builtinRules:    builtinCompiled,
		cfg:             cfg,
	}, nil
}

// resolveRule checks if a rule is active for the given file path after applying
// overrides and Where predicates. Returns the effective severity, message, and
// whether the rule should run.
func resolveRule(effective map[string]config.RuleConfig, name, message, filePath string) (config.Severity, string, bool) {
	rule, exists := effective[name]
	if !exists || rule.Severity == config.SeverityOff {
		return "", "", false
	}
	if !matchesWhere(rule.Where, filePath) {
		return "", "", false
	}
	msg := message
	if rule.Message != "" {
		msg = rule.Message
	}
	return rule.Severity, msg, true
}

// LintFile lints a single file's source and returns diagnostics.
// It applies config overrides for the file path, evaluates Where predicates,
// and runs all matching regex, pattern, structural, and import rules.
func (e *Engine) LintFile(ctx context.Context, filePath string, source []byte) []Diagnostic {
	effective := config.Merge(e.cfg, filePath)
	lineStarts := buildLineIndex(source)

	diags := make([]Diagnostic, 0, len(e.regexRules)+len(e.patternRules)+len(e.structuralRules)+len(e.importRules)+len(e.builtinRules))

	acquireCGo()
	defer releaseCGo()

	// --- Regex rules ---
	for i := range e.regexRules {
		cr := &e.regexRules[i]

		sev, msg, ok := resolveRule(effective, cr.name, cr.message, filePath)
		if !ok {
			continue
		}

		active := *cr
		active.severity = sev
		active.message = msg

		found := matchRegex(active, source, lineStarts, 0)
		setFilePath(found, filePath)
		diags = append(diags, found...)
	}

	// --- AST-based rules (pattern + structural + imports + builtins) ---
	// Resolve active rules before parsing — skip tree-sitter entirely
	// when all AST-based rules are disabled for this file.
	activePatterns := resolvePatternRules(e.patternRules, effective, filePath)
	activeStructural := resolveStructuralRules(e.structuralRules, effective, filePath)
	activeImports := resolveImportRules(e.importRules, effective, filePath)
	activeBuiltins := resolveBuiltinRules(e.builtinRules, effective, filePath)

	if len(activePatterns) > 0 || len(activeStructural) > 0 || len(activeImports) > 0 || len(activeBuiltins) > 0 {
		lang, ok := parser.LangFromPath(filePath)
		if ok {
			p := parser.NewParser(lang)
			tree, err := p.Parse(ctx, source, nil)
			p.Close()

			if err != nil {
				slog.Debug("AST rules: parse failed, skipping",
					"file", filePath, "error", err)
			} else {
				if len(activePatterns) > 0 {
					found := matchPatterns(ctx, activePatterns, tree, source, lineStarts)
					for j := range found {
						found[j].File = filePath
					}
					diags = append(diags, found...)
				}

				if len(activeStructural) > 0 {
					found := matchStructural(ctx, activeStructural, tree, source, lineStarts)
					for j := range found {
						found[j].File = filePath
					}
					diags = append(diags, found...)
				}

				if len(activeImports) > 0 {
					found := matchImports(ctx, activeImports, tree, source, lineStarts)
					setFilePath(found, filePath)
					diags = append(diags, found...)
				}

				if len(activeBuiltins) > 0 {
					found := matchBuiltins(activeBuiltins, tree, source, lineStarts)
					for j := range found {
						found[j].File = filePath
					}
					diags = append(diags, found...)
				}

				tree.Close()
			}
		}
	}

	// Filter diagnostics suppressed by inline comments.
	sup := parseSuppressComments(source)
	diags = filterSuppressed(diags, sup)

	// Sort within file by line, col, rule so Lint only needs to merge by filename.
	slices.SortFunc(diags, compareDiagsWithinFile)

	return diags
}

// resolvePatternRules filters pattern rules that are active for the given file.
func resolvePatternRules(rules []compiledPattern, effective map[string]config.RuleConfig, filePath string) []compiledPattern {
	active := make([]compiledPattern, 0, len(rules))
	for i := range rules {
		cp := &rules[i]
		sev, msg, ok := resolveRule(effective, cp.name, cp.message, filePath)
		if !ok {
			continue
		}
		resolved := *cp
		resolved.severity = sev
		resolved.message = msg
		active = append(active, resolved)
	}
	return active
}

// resolveStructuralRules filters structural rules that are active for the given file.
func resolveStructuralRules(rules []compiledStructural, effective map[string]config.RuleConfig, filePath string) []compiledStructural {
	active := make([]compiledStructural, 0, len(rules))
	for i := range rules {
		cs := &rules[i]
		sev, msg, ok := resolveRule(effective, cs.name, cs.message, filePath)
		if !ok {
			continue
		}
		resolved := *cs
		resolved.severity = sev
		resolved.message = msg
		active = append(active, resolved)
	}
	return active
}

func compareDiagsWithinFile(a, b Diagnostic) int { //nolint:gocritic // slices.SortFunc requires value receiver signature
	if c := cmp.Compare(a.Line, b.Line); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Col, b.Col); c != 0 {
		return c
	}
	return cmp.Compare(a.Rule, b.Rule)
}

// Lint processes multiple files in parallel and returns aggregated results.
// threads controls concurrency; 0 means runtime.NumCPU().
func (e *Engine) Lint(ctx context.Context, files []string, threads int) *Result {
	if len(files) == 0 {
		return &Result{}
	}
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if threads > len(files) {
		threads = len(files)
	}

	type workerResult struct {
		diags  []Diagnostic
		errors []FileError
	}
	results := make([]workerResult, threads)

	g, ctx := errgroup.WithContext(ctx)

	batchSize := (len(files) + threads - 1) / threads
	for w := range threads {
		start := w * batchSize
		if start >= len(files) {
			break
		}
		end := start + batchSize
		if end > len(files) {
			end = len(files)
		}
		batch := files[start:end]
		wr := &results[w]

		g.Go(func() error {
			wr.diags = make([]Diagnostic, 0, len(batch)*4)
			for _, filePath := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				source, err := os.ReadFile(filePath) //nolint:gosec // caller-controlled paths from discoverFiles
				if err != nil {
					wr.errors = append(wr.errors, FileError{File: filePath, Err: err})
					continue
				}
				diags := e.LintFile(ctx, filePath, source)
				wr.diags = append(wr.diags, diags...)
			}
			return nil
		})
	}

	var result Result
	if err := g.Wait(); err != nil {
		result.Errors = append(result.Errors, FileError{
			File: "",
			Err:  fmt.Errorf("lint cancelled: %w", err),
		})
	}

	totalDiags := 0
	totalErrs := 0
	for i := range results {
		totalDiags += len(results[i].diags)
		totalErrs += len(results[i].errors)
	}
	result.Diagnostics = make([]Diagnostic, 0, totalDiags)
	result.Errors = append(make([]FileError, 0, totalErrs), result.Errors...)
	for i := range results {
		result.Diagnostics = append(result.Diagnostics, results[i].diags...)
		result.Errors = append(result.Errors, results[i].errors...)
	}

	SortDiagChunksByFile(result.Diagnostics)
	return &result
}

// FileSource pairs a file path with its already-read contents.
// Used by LintSources to avoid double-reading files (once for hashing, once for linting).
type FileSource struct {
	Path   string
	Source []byte
}

// LintSources is like Lint but accepts pre-read file contents.
// This avoids re-reading files when the caller has already read them (e.g., for cache hashing).
func (e *Engine) LintSources(ctx context.Context, files []FileSource, threads int) *Result {
	if len(files) == 0 {
		return &Result{}
	}
	if threads <= 0 {
		threads = runtime.NumCPU()
	}
	if threads > len(files) {
		threads = len(files)
	}

	type workerResult struct {
		diags []Diagnostic
	}
	results := make([]workerResult, threads)

	g, ctx := errgroup.WithContext(ctx)

	batchSize := (len(files) + threads - 1) / threads
	for w := range threads {
		start := w * batchSize
		if start >= len(files) {
			break
		}
		end := start + batchSize
		if end > len(files) {
			end = len(files)
		}
		// batch and wr are per-iteration variables, so capturing them in the goroutine is safe.
		batch := files[start:end]
		wr := &results[w]

		g.Go(func() error {
			wr.diags = make([]Diagnostic, 0, len(batch)*4)
			for _, fs := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				diags := e.LintFile(ctx, fs.Path, fs.Source)
				wr.diags = append(wr.diags, diags...)
			}
			return nil
		})
	}

	var result Result
	if err := g.Wait(); err != nil {
		result.Errors = append(result.Errors, FileError{
			File: "",
			Err:  fmt.Errorf("lint cancelled: %w", err),
		})
	}

	totalDiags := 0
	for i := range results {
		totalDiags += len(results[i].diags)
	}
	result.Diagnostics = make([]Diagnostic, 0, totalDiags)
	for i := range results {
		result.Diagnostics = append(result.Diagnostics, results[i].diags...)
	}

	SortDiagChunksByFile(result.Diagnostics)
	return &result
}

// fileChunk identifies a contiguous run of diagnostics from the same file.
type fileChunk struct {
	file       string
	start, end int
}

// SortDiagChunksByFile reorders diagnostics that are already grouped into
// contiguous per-file chunks so that those chunks are ordered by filename.
//
// Precondition: all diagnostics for a given file must appear in a single
// contiguous segment, and diagnostics within each chunk must already be
// sorted (e.g., by position). This function only reorders chunks by filename;
// it does not fully sort an arbitrary diagnostics slice.
//
// Exported for use by the CLI layer when merging cached and fresh diagnostics.
func SortDiagChunksByFile(diags []Diagnostic) {
	if len(diags) == 0 {
		return
	}

	// Identify contiguous chunks.
	chunks := make([]fileChunk, 0, 64)
	start := 0
	for i := 1; i < len(diags); i++ {
		if diags[i].File != diags[start].File {
			chunks = append(chunks, fileChunk{file: diags[start].File, start: start, end: i})
			start = i
		}
	}
	chunks = append(chunks, fileChunk{file: diags[start].File, start: start, end: len(diags)})

	// Sort chunk metadata by filename.
	slices.SortFunc(chunks, func(a, b fileChunk) int {
		return cmp.Compare(a.file, b.file)
	})

	// Reassemble in sorted order.
	sorted := make([]Diagnostic, 0, len(diags))
	for _, c := range chunks {
		sorted = append(sorted, diags[c.start:c.end]...)
	}
	copy(diags, sorted)
}
