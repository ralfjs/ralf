package engine

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"slices"

	"github.com/Hideart/ralf/internal/config"
	"github.com/Hideart/ralf/internal/parser"
	"golang.org/x/sync/errgroup"
)

// Engine compiles rules from config and runs them against source files.
type Engine struct {
	regexRules   []compiledRegex
	patternRules []compiledPattern
	cfg          *config.Config
}

// New creates an Engine from the given config. It compiles all regex and
// pattern rules, returning any compilation errors. Unsupported matcher types
// (AST, imports, naming) are silently skipped.
func New(cfg *config.Config) (*Engine, []error) {
	regexCompiled, regexErrs := compileRegexRules(cfg.Rules)
	patternCompiled, patternErrs := compilePatternRules(cfg.Rules)

	errs := make([]error, 0, len(regexErrs)+len(patternErrs))
	errs = append(errs, regexErrs...)
	errs = append(errs, patternErrs...)
	if len(errs) > 0 {
		return nil, errs
	}

	return &Engine{
		regexRules:   regexCompiled,
		patternRules: patternCompiled,
		cfg:          cfg,
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
// and runs all matching regex and pattern rules.
func (e *Engine) LintFile(ctx context.Context, filePath string, source []byte) []Diagnostic {
	effective := config.Merge(e.cfg, filePath)
	lineStarts := buildLineIndex(source)

	diags := make([]Diagnostic, 0, len(e.regexRules)+len(e.patternRules))

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
		for j := range found {
			found[j].File = filePath
		}
		diags = append(diags, found...)
	}

	// --- Pattern rules ---
	if len(e.patternRules) > 0 {
		// Resolve active patterns before parsing — skip tree-sitter entirely
		// when all pattern rules are disabled for this file.
		active := make([]compiledPattern, 0, len(e.patternRules))
		for i := range e.patternRules {
			cp := &e.patternRules[i]
			sev, msg, ok := resolveRule(effective, cp.name, cp.message, filePath)
			if !ok {
				continue
			}
			resolved := *cp
			resolved.severity = sev
			resolved.message = msg
			active = append(active, resolved)
		}

		if len(active) > 0 {
			lang, ok := parser.LangFromPath(filePath)
			if ok {
				p := parser.NewParser(lang)
				tree, err := p.Parse(ctx, source, nil)
				p.Close()

				if err != nil {
					slog.Debug("pattern rules: parse failed, skipping",
						"file", filePath, "error", err)
				} else {
					found := matchPatterns(ctx, active, tree, source, lineStarts)
					for j := range found {
						found[j].File = filePath
					}
					diags = append(diags, found...)
					tree.Close()
				}
			}
		}
	}

	// Sort within file by line, col, rule so Lint only needs to merge by filename.
	slices.SortFunc(diags, compareDiagsWithinFile)

	return diags
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

	// Partition files into batches — one per worker. Each worker processes
	// its batch sequentially, collecting results locally with no mutex.
	type workerResult struct {
		diags []Diagnostic
		errs  []FileError
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
			wr.diags = make([]Diagnostic, 0, len(batch)*(len(e.regexRules)+len(e.patternRules)))

			for _, filePath := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}

				source, err := os.ReadFile(filePath) //nolint:gosec // caller-controlled paths from discoverFiles
				if err != nil {
					wr.errs = append(wr.errs, FileError{File: filePath, Err: err})
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

	// Merge worker results — no sorting needed within workers since files
	// are processed in order and LintFile sorts within each file.
	totalDiags := 0
	totalErrs := 0
	for i := range results {
		totalDiags += len(results[i].diags)
		totalErrs += len(results[i].errs)
	}
	result.Diagnostics = make([]Diagnostic, 0, totalDiags)
	result.Errors = make([]FileError, 0, totalErrs)
	for i := range results {
		result.Diagnostics = append(result.Diagnostics, results[i].diags...)
		result.Errors = append(result.Errors, results[i].errs...)
	}

	// Diagnostics arrive in contiguous per-file chunks (already sorted by
	// line/col/rule within each chunk by LintFile). Sort the chunks by
	// filename for deterministic output.
	sortDiagChunksByFile(result.Diagnostics)

	return &result
}

// fileChunk identifies a contiguous run of diagnostics from the same file.
type fileChunk struct {
	file       string
	start, end int
}

// sortDiagChunksByFile sorts diagnostics that arrive in contiguous per-file
// chunks. Within each chunk, diagnostics are already sorted by line/col/rule.
// This is O(N + C·log C) where C is the number of file chunks (typically
// equal to the number of files), vs O(N·log N) for a full sort.
func sortDiagChunksByFile(diags []Diagnostic) {
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
