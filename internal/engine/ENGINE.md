# Engine Package

Lint pipeline: config → compile rules → discover files → parallel lint → collect diagnostics → format output.

## Architecture

```
Config
  ↓
engine.New()          compile regex rules, skip unsupported matchers
  ↓
Engine.Lint()         fan out files via errgroup with bounded concurrency
  ├─ Engine.LintFile()   per-file: merge overrides → filter rules → match → collect
  │   ├─ config.Merge()     effective rules for this file path
  │   ├─ matchesWhere()     evaluate Where predicate (doublestar globs)
  │   ├─ buildLineIndex()   byte offset → line:col via binary search
  │   └─ matchRegex()       FindAllIndex + dedup + max cap
  └─ ... (parallel)
  ↓
Result                sorted diagnostics + file errors
```

## Files

| File | Responsibility |
|---|---|
| `diagnostic.go` | `Diagnostic`, `FileError`, `Result` types |
| `lineindex.go` | `buildLineIndex` (O(n) scan), `offsetToLineCol` (O(log n) binary search) |
| `regex.go` | `compiledRegex` type, `compileRegexRules`, `matchRegex` |
| `where.go` | `matchesWhere` — evaluates `config.WherePredicate` against file paths |
| `engine.go` | `Engine` orchestrator: `New`, `LintFile`, `Lint` |

## Diagnostic Conventions

- `Line`: 1-based (first line = 1)
- `Col`: 0-based byte offset within line (formatters convert to 1-based for display)
- `EndLine`/`EndCol`: same conventions, mark end of matched region
- `File`: absolute path, set by `LintFile`/`Lint`

Matches tree-sitter's coordinate system for consistency when AST-based rules are added.

## Line Index

Standard approach from `go/token`: scan source once for `\n` positions, store `[]int` of line-start byte offsets. First entry is always 0. Binary search via `sort.SearchInts` per offset lookup.

Handles `\r\n` (CRLF) — `\n` positions are tracked, `\r` is an extra byte in the column offset.

## Regex Matcher

- `compileRegexRules`: filters `Regex != ""` and `Severity != Off`, compiles each via `regexp.Compile`, collects all errors (no fail-fast)
- `matchRegex`: `re.FindAllIndex` → `offsetToLineCol` → dedup by line (one diagnostic per rule per line) → cap at `maxMatches` (default 10,000)
- Pre-compiled `*regexp.Regexp` is concurrent-safe (Go 1.12+). No per-rule parallelism — parallelism is at file level.

**Placeholder engine**: uses Go stdlib `regexp`. The API shape matches rure-go's — swapping to `rure.MustCompile` + `rure.Iter` is a single-function change when librure is vendored.

## Where Predicate

Evaluated per rule per file in `LintFile`. Rules whose predicate doesn't match are skipped.

- `nil` → matches all files
- `File: "src/**/*.js"` → matched via `doublestar.Match` (supports `**`)
- `Not: { File: "test/**" }` → inverts inner predicate
- `ImportCrosses` → not yet implemented, matches all (doesn't block linting)

## Parallelism

`Engine.Lint` uses `errgroup.SetLimit(threads)` (default `runtime.NumCPU()`). Each goroutine reads one file and calls `LintFile`. Results are collected under a mutex, then sorted by file/line/col/rule for deterministic output.

Context cancellation is checked before each file read. If cancelled, remaining files are skipped and an error is recorded.

## Future Matchers

Planned but not implemented in this sprint:
- **Pattern**: AST pattern matching (`console.log($$$)`) — month 3
- **AST**: structural queries (`kind: "function_declaration"`) — month 3
- **Imports**: import ordering rules — month 4
- **Naming**: naming convention enforcement — month 4

Rules with these matcher types are silently skipped by `engine.New`. Config validation accepts them.
