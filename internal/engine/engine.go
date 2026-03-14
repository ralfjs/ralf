package engine

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"

	"github.com/Hideart/bepro/internal/config"
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

	var diags []Diagnostic

	for i := range e.regexRules {
		cr := &e.regexRules[i]

		// Check if the rule is still active after overrides.
		rule, exists := effective[cr.name]
		if !exists || rule.Severity == config.SeverityOff {
			continue
		}

		// Evaluate Where predicate.
		if !matchesWhere(cr.where, filePath) {
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

	return diags
}

// Lint processes multiple files in parallel and returns aggregated results.
// threads controls concurrency; 0 means runtime.NumCPU().
func (e *Engine) Lint(ctx context.Context, files []string, threads int) *Result {
	if threads <= 0 {
		threads = runtime.NumCPU()
	}

	var (
		mu     sync.Mutex
		result Result
	)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(threads)

	for _, f := range files {
		filePath := f
		g.Go(func() error {
			// Check context before reading.
			if err := ctx.Err(); err != nil {
				return err
			}

			source, err := os.ReadFile(filePath) //nolint:gosec // caller-controlled paths from discoverFiles
			if err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, FileError{File: filePath, Err: err})
				mu.Unlock()
				return nil // don't abort other files
			}

			diags := e.LintFile(ctx, filePath, source)

			if len(diags) > 0 {
				mu.Lock()
				result.Diagnostics = append(result.Diagnostics, diags...)
				mu.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		// Context cancelled — add as a general error.
		result.Errors = append(result.Errors, FileError{
			File: "",
			Err:  fmt.Errorf("lint cancelled: %w", err),
		})
	}

	// Sort diagnostics for deterministic output: file, line, col, rule.
	sort.Slice(result.Diagnostics, func(i, j int) bool {
		a, b := result.Diagnostics[i], result.Diagnostics[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.Rule < b.Rule
	})

	return &result
}
