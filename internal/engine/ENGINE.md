# Engine Package

Lint pipeline: config → compile rules → discover files → parallel lint → collect diagnostics → format output.

## Architecture

```
Config
  ↓
engine.New()          compile regex rules, skip unsupported matchers
  ↓
Engine.Lint()         partition files into worker batches, fan out via errgroup
  ├─ Worker goroutine    processes batch of files sequentially, no mutex
  │   ├─ os.ReadFile()       read source from disk
  │   └─ Engine.LintFile()   per-file: merge overrides → filter rules → match → sort
  │       ├─ config.Merge()     effective rules for this file path (fast path: no overrides → no alloc)
  │       ├─ acquireCGo()       CGo semaphore (bounds OS threads to NumCPU)
  │       ├─ matchesWhere()     evaluate Where predicate (doublestar globs)
  │       ├─ buildLineIndex()   SIMD-accelerated newline scan (bytes.IndexByte)
  │       ├─ matchRegex()       rure.IterBytes + dedup + max cap
  │       └─ slices.SortFunc()  sort diagnostics within file by line/col/rule
  └─ ... (parallel workers)
  ↓
Merge worker results  concat per-worker slices (no mutex, no shared state)
  ↓
sortDiagChunksByFile  sort contiguous per-file chunks by filename (O(N + C·log C))
  ↓
Result                sorted diagnostics + file errors
```

## Files

| File | Responsibility |
|---|---|
| `diagnostic.go` | `Diagnostic`, `FileError`, `Result` types |
| `lineindex.go` | `buildLineIndex` (O(n) scan), `offsetToLineCol` (O(log n) binary search) |
| `regex.go` | `compiledRegex` type, `compileRegexRules`, `matchRegex` (rure-go) |
| `semaphore.go` | CGo concurrency limiter (`acquireCGo`/`releaseCGo`) |
| `where.go` | `matchesWhere` — evaluates `config.WherePredicate` against file paths |
| `engine.go` | `Engine` orchestrator: `New`, `LintFile`, `Lint`, `sortDiagChunksByFile` |

## Diagnostic Conventions

- `Line`: 1-based (first line = 1)
- `Col`: 0-based byte offset within line (formatters convert to 1-based for display)
- `EndLine`/`EndCol`: same conventions, mark end of matched region
- `File`: absolute path, set by `LintFile`/`Lint`

Matches tree-sitter's coordinate system for consistency when AST-based rules are added.

## Line Index

Standard approach from `go/token`: scan source once for `\n` positions, store `[]int` of line-start byte offsets. First entry is always 0. Pre-allocated based on estimated ~60 bytes/line. Binary search via `sort.SearchInts` per offset lookup.

Uses `bytes.IndexByte` loop instead of byte-by-byte `range` iteration. Go's `bytes.IndexByte` uses platform-optimized assembly (SIMD on supported architectures) to scan multiple bytes per cycle.

Handles `\r\n` (CRLF) — `\n` positions are tracked, `\r` is an extra byte in the column offset.

## Regex Matcher

Uses **rure-go** (Rust regex engine via CGo) for pattern matching. Requires `librure.a` — built from `github.com/rust-lang/regex` capi via `scripts/build-librure.sh`.

- `compileRegexRules`: filters `Regex != ""` and `Severity != Off`, compiles each via `rure.Compile`, collects all errors (no fail-fast)
- `matchRegex`: `rure.IterBytes` → iterate matches in C (single CGo call) → `offsetToLineCol` → dedup by line (one diagnostic per rule per line) → cap at `maxMatches` (default 10,000)
- Pre-compiled `*rure.Regex` is concurrent-safe. `*rure.Iter` is per-call (not shared). No per-rule parallelism — parallelism is at file level.

Rust regex syntax is largely compatible with Go's RE2. Both use the same RE2 semantics (no lookaheads/lookbehinds). Key advantage is performance: rure-go's DFA engine with C-side iteration is ~15x faster than Go stdlib on large inputs.

**Known syntax difference:** `\b` in Rust regex is Unicode-aware by default (matches at boundaries between Unicode word characters and non-word characters), while Go's `regexp` treats `\b` as ASCII-only. This means patterns like `\bcafé\b` work correctly in rure-go but may not match as expected in Go stdlib. Rule authors can rely on full Unicode `\b` behavior.

## CGo Semaphore

`semaphore.go` bounds concurrent CGo calls to `runtime.NumCPU()` via a buffered channel. CGo pins goroutines to OS threads — unbounded concurrent CGo calls cause unbounded OS thread creation and resource exhaustion. The semaphore is acquired once per file in `LintFile` (covers all regex rule calls for that file) and released on return.

## Parallelism

`Engine.Lint` partitions files into equal batches (one per worker, up to `runtime.NumCPU()` workers). Each worker goroutine processes its batch sequentially, collecting diagnostics and errors into a local slice — no mutex during the hot path. After all workers finish, results are concatenated and sorted.

`LintFile` sorts diagnostics within a single file by line/col/rule. `Lint` then sorts the contiguous per-file chunks by filename via `sortDiagChunksByFile` — O(N + C·log C) where C is the number of files, vs O(N·log N) for a full sort of all diagnostics.

Context cancellation is checked before each file read. If cancelled, remaining files in the batch are skipped and an error is recorded.

## Sorting Strategy

Diagnostics must be sorted for deterministic output: file, then line, then col, then rule. Instead of one large sort over all diagnostics:

1. **`LintFile`** sorts per-file diagnostics by line/col/rule (small sorts, ~hundreds of items)
2. **`Lint`** merges worker results into contiguous per-file chunks
3. **`sortDiagChunksByFile`** identifies chunk boundaries and sorts them by filename (sorts ~C chunk descriptors, then reassembles)

This turns an O(N·log N) sort of 150K+ diagnostics into many O(K·log K) sorts of ~1.5K items + one O(C·log C) sort of ~100 file chunks.

## Performance Optimizations

Current optimizations (all allocation/scheduling, no logic changes):

- **SIMD newline scanning**: `buildLineIndex` uses `bytes.IndexByte` loop — Go's implementation uses platform-optimized assembly (SIMD on supported architectures) to scan multiple bytes per cycle instead of byte-by-byte iteration.
- **Pre-allocated slices**: `lineStarts`, `seen` map, `diags` in `matchRegex`, `LintFile`, and `Lint`
- **config.Merge fast path**: returns `cfg.Rules` directly when no overrides exist (zero allocation)
- **Worker batching**: files partitioned statically across workers — no per-file goroutine spawn, no mutex during processing
- **Chunk sort**: two-level sort strategy avoids O(N·log N) on the full diagnostic set
- **`slices.SortFunc`**: avoids interface boxing overhead of `sort.Slice`

### Benchmarking

Two benchmarks live in `internal/engine/regex_bench_test.go`:

| Benchmark | What it measures |
|---|---|
| `BenchmarkMatchRegex` | Single rule on 30K lines. Isolates rure-go iterator + dedup + line resolution. No I/O, no concurrency. |
| `BenchmarkLintE2E` | Full `Engine.Lint` pipeline: 100 files × 300 lines × 5 rules. Includes disk I/O, worker batching, CGo semaphore, config merging, per-file sort, chunk sort. |

Run benchmarks:

```bash
make bench                          # both benchmarks, single iteration
make bench | grep Benchmark         # just the numbers

# For stable comparisons, use -count:
go test -bench=. -benchmem -count=5 ./internal/engine/
```

When evaluating performance changes:

- **Always compare with `-count=5`** (or more). Single runs have high variance due to OS scheduling and thermal throttling.
- **`MatchRegex`** is the low-noise benchmark — use it for regex engine changes.
- **`LintE2E`** has higher variance (~±15%) due to disk I/O and goroutine scheduling. Look at median, not best/worst.
- **Watch allocs, not just wall time.** Allocation reductions pay off under sustained load (LSP mode, watch mode) even when wall time doesn't visibly improve in isolated benchmarks.
- **Use `go test -cpuprofile` and `go tool pprof`** to identify bottlenecks before optimizing. The current breakdown (~27ms): regex matching ~10ms, file I/O ~8ms, sort+merge ~5ms, lineindex ~4ms.

### Future Optimizations

These require more invasive changes and are deferred to a later sprint:

- **mmap file reads**: benchmarked — adds syscall overhead (open→stat→mmap→munmap) on small warm-cache files that outweighs the eliminated userspace copy. With project cache (Phase 2), cold scans happen once at startup; steady-state is single-file re-lints where mmap loses. Revisit only if cold-scan performance becomes a concern before caching is implemented.
- **Buffer pooling**: `sync.Pool` for `lineStarts []int` and `[]Diagnostic` slices to reduce GC pressure across sequential lint runs. Becomes load-bearing in LSP/watch mode (Phase 2) where `LintFile` is called thousands of times in a long-running process. Deferred because it changes `buildLineIndex` signature and touches test/bench files.
- **Parallel file readahead**: separate I/O goroutines pre-read files into a bounded buffer pool while worker goroutines process already-loaded files, overlapping I/O and compute.

## Where Predicate

Evaluated per rule per file in `LintFile`. Rules whose predicate doesn't match are skipped.

- `nil` → matches all files
- `File: "src/**/*.js"` → matched via `doublestar.Match` (supports `**`)
- `Not: { File: "test/**" }` → inverts inner predicate
- `ImportCrosses` → not yet implemented, matches all (doesn't block linting)

## Future Matchers

Planned but not implemented in this sprint:
- **Pattern**: AST pattern matching (`console.log($$$)`) — month 3
- **AST**: structural queries (`kind: "function_declaration"`) — month 3
- **Imports**: import ordering rules — month 4
- **Naming**: naming convention enforcement — month 4

Rules with these matcher types are silently skipped by `engine.New`. Config validation accepts them.
