package engine

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"runtime"
	"slices"

	"github.com/Hideart/ralf/internal/config"
	"golang.org/x/sync/errgroup"
)

// Engine compiles rules from config and runs them against source files.
type Engine struct {
	regexRules []compiledRegex
	cfg        *config.Config
}

// New creates an Engine from the given config. It compiles all regex rules and
// returns any compilation errors. Unsupported matcher types (pattern, AST,
// imports, naming) are silently skipped.
func New(cfg *config.Config) (*Engine, []error) {
	compiled, errs := compileRegexRules(cfg.Rules)
	if len(errs) > 0 {
		return nil, errs
	}

	return &Engine{
		regexRules: compiled,
		cfg:        cfg,
	}, nil
}

// LintFile lints a single file's source and returns diagnostics.
// It applies config overrides for the file path, evaluates Where predicates,
// and runs all matching regex rules.
func (e *Engine) LintFile(_ context.Context, filePath string, source []byte) []Diagnostic {
	effective := config.Merge(e.cfg, filePath)
	lineStarts := buildLineIndex(source)

	diags := make([]Diagnostic, 0, len(e.regexRules))

	acquireCGo()
	defer releaseCGo()

	for i := range e.regexRules {
		cr := &e.regexRules[i]

		// Check if the rule is still active after overrides.
		rule, exists := effective[cr.name]
		if !exists || rule.Severity == config.SeverityOff {
			continue
		}

		// Evaluate Where predicate from the effective (possibly overridden) rule,
		// not from the compiled rule which holds the base config's predicate.
		if !matchesWhere(rule.Where, filePath) {
			continue
		}

		// Use the effective severity (may be overridden).
		active := *cr
		active.severity = rule.Severity
		if rule.Message != "" {
			active.message = rule.Message
		}

		found := matchRegex(active, source, lineStarts, 0)
		for j := range found {
			found[j].File = filePath
		}
		diags = append(diags, found...)
	}

	// Sort within file by line, col, rule so Lint only needs to merge by filename.
	slices.SortFunc(diags, compareDiagsWithinFile)

	return diags
}

func compareDiagsWithinFile(a, b Diagnostic) int {
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
			wr.diags = make([]Diagnostic, 0, len(batch)*len(e.regexRules))

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
