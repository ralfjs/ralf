# CLAUDE.md

BEPRO — fast, project-aware JS/TS linter + formatter written in Go.

## Quick Reference

**Language:** Go 1.22+
**Module:** `github.com/Hideart/bepro`
**CGo deps:** rure-go (Rust regex), go-tree-sitter
**Pure Go deps:** modernc.org/sqlite, fsnotify, goja, wazero, xxhash

### Commands

```bash
make build              # Build binary
make test               # Run tests
make test-race          # Run tests with race detector
make lint               # golangci-lint
make fmt                # gofumpt
make bench              # Run benchmarks
make verify             # CI check (lint + format + mod tidy)
```

---

## Architecture

Full technical spec: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

### Project Layout

```
cmd/bepro/main.go           # Entry point — MUST stay thin (~30 lines)
                             # Parse flags, wire deps, call internal packages. No logic here.

internal/
  engine/                    # Rule execution — the core of the linter
    engine.go                # Orchestrator: takes config + files, returns diagnostics
    regex.go                 # rure-go pattern matching (parallel, semaphore-limited)
    ast_pattern.go           # AST pattern matching ("console.log($$$)" syntax)
    structural.go            # Structural AST queries (kind, parent, ancestor, capture)
    naming.go                # Naming convention checks on AST captures
    imports.go               # Import ordering / grouping analysis
    complexity.go            # Cyclomatic complexity
    crossfile.go             # Cross-file rule evaluation (uses project.Graph)
    fix.go                   # Auto-fix: replacement, deletion, insertion, templates
    lineindex.go             # Line/col resolution (binary search on line starts)

  parser/                    # tree-sitter wrapper
    treesitter.go            # Parse file → AST, walk nodes, incremental reparse
    queries.go               # Pre-built tree-sitter queries for JS/TS/JSX/TSX

  formatter/                 # Code formatting
    printer.go               # CST → formatted output (Phase 3+)
    dprint_wasm.go           # dprint WASM plugin bridge via Wazero

  project/                   # Project-level analysis
    graph.go                 # Module graph: imports, exports, ImportedBy, ExportedBy
    cache.go                 # SQLite cache: per-file hash, diagnostics, symbols
    watcher.go               # fsnotify + cascade invalidation
    scanner.go               # Initial project scan (parallel file processing)
    hasher.go                # xxhash content hashing

  lsp/                       # Language Server Protocol
    server.go                # JSON-RPC handler over stdio
    diagnostics.go           # Push diagnostics to editor
    formatting.go            # Format on save / on type
    codelens.go              # Inline code actions

  config/                    # Configuration
    loader.go                # Load .lintrc.{json,yaml,toml,js} → *Config struct
    compiler.go              # Compile declarative rules → engine representation
    schema.go                # Config validation
    defaults.go              # Built-in recommended ruleset

  cli/                       # CLI commands
    lint.go                  # bepro lint
    format.go                # bepro format
    check.go                 # bepro check (lint + format)
    init.go                  # bepro init, --from-eslint, --from-biome
    debug.go                 # bepro debug (rules, parse, graph)

  plugin/                    # WASM plugin system
    host.go                  # Load + run .wasm plugins via Wazero
    sdk.go                   # Plugin SDK interface definitions
    sandbox.go               # Sandboxed file access for plugins

testdata/                    # Test fixtures (Go tooling ignores during builds)
  rules/<rule-name>/         # Per-rule: valid.js, invalid.js with expect-error comments
  format/<case-name>/        # Per-case: input.ts, output.ts
```

### Dependency Direction

```
cmd/bepro → internal/cli → internal/engine, internal/config, internal/project
internal/engine → internal/parser
internal/project → internal/parser, internal/engine
internal/lsp → internal/engine, internal/project, internal/formatter
internal/formatter → internal/parser
internal/plugin → internal/engine (plugin SDK types)
internal/config → standalone (no internal deps)
```

**Rules:**
- `cmd/bepro/main.go` imports only `internal/cli`. Never import engine/parser/etc directly from main.
- `internal/config` must have zero dependencies on other internal packages.
- `internal/parser` must not depend on `internal/engine` (parser is a low-level primitive).
- `internal/engine` must not depend on `internal/project` — the project layer calls engine, not the reverse.

---

## Code Style

### Naming

- `MixedCaps` / `mixedCaps` only. No underscores (except test functions: `TestScanRule_EmptyInput`).
- Package names: lowercase, single word. `engine`, `parser`, `config` — not `rule_engine`.
- Receiver names: 1-2 letter abbreviation. `func (r Rule) Match(...)`, not `func (rule Rule)`.
- Acronyms all-caps: `AST`, `CST`, `LSP`, `HTTP`, `ID` — not `Ast`, `Lsp`.
- Getters drop `Get`: `rule.Name()`, not `rule.GetName()`.
- Interfaces: method + `er` suffix for single-method interfaces: `Matcher`, `Scanner`, `Formatter`.

### Error Handling

- Always check errors. Never `_` an error without a comment explaining why.
- Wrap with context: `fmt.Errorf("compile rule %q: %w", name, err)`.
- Use `errors.Is()` / `errors.As()` — never `==` or type assertions on errors.
- **Never panic for recoverable errors.** Use `Must*` prefix only for package-level init.
- Define sentinel errors for public APIs: `var ErrRuleCompile = errors.New("rule compile failed")`.

### Interfaces

- Accept interfaces, return structs.
- Define interfaces at the **consumer**, not the implementer.
- Keep interfaces small (1-3 methods).

### Concurrency

- `wg.Add(1)` before `go func()`, never inside the goroutine.
- Semaphore pattern (`chan struct{}` sized to `runtime.NumCPU()`) for bounding CGo calls.
- Channel direction in signatures: `chan<-` (send), `<-chan` (receive).
- Never share `map` across goroutines without sync.

### CGo (rure-go, tree-sitter)

- Minimize CGo boundary crossings. Use batch APIs (`IterBytes`, `FindAllBytes`).
- Semaphore-limit concurrent CGo calls to `runtime.NumCPU()`.
- Never pass Go pointers to C that will be retained after the call returns.

---

## Testing

### Rules

- **Table-driven tests for all pure functions.** No exceptions.
- **`t.Run()` for subtests** — always.
- **No testify.** Standard library `testing` only.
- **Test naming:** `TestFunctionName_Scenario` — `TestScanRuleIter_MaxMatchesCap`.
- **`t.Parallel()`** for independent subtests. NOT when sharing CGo resources.

### Rule Fixtures

Each rule has `testdata/rules/<rule-name>/valid.js` and `invalid.js`:

```js
// testdata/rules/no-var/invalid.js
var x = 1;         // expect-error: no-var
var y = 2;         // expect-error: no-var
const z = 3;       // OK
```

Test runner compares diagnostics against `expect-error` comments.

### Benchmarks

Benchmarks go in `*_test.go` using `testing.B`, not hand-rolled timing in main:

```go
func BenchmarkAnalyze(b *testing.B) {
    data := loadFixture(b, "testdata/large.js")
    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        analyze(data, rules)
    }
}
```

### CI Flags

```bash
go test -race -count=1 -coverprofile=coverage.out ./...
```

- `-race` mandatory (concurrent code with CGo).
- `-count=1` disables cache (catches flaky tests).

---

## Anti-Patterns (MUST NOT do)

### Architecture
- **Never** put logic in `cmd/bepro/main.go` — it must be a thin wrapper.
- **Never** import `internal/engine` from `internal/parser` — parser is lower-level.
- **Never** import `internal/project` from `internal/engine` — project calls engine, not reverse.
- **Never** use `pkg/` directory — everything goes in `internal/`.

### Code
- **Never** panic for recoverable errors — return errors.
- **Never** use `interface{}` / `any` when a concrete type is known.
- **Never** mutate shared state without synchronization.
- **Never** use `sync.Mutex` when a channel or `sync.WaitGroup` is clearer.
- **Never** ignore `golangci-lint` warnings — fix them or explicitly disable with a comment.

### CGo
- **Never** call CGo in a tight inner loop without batching.
- **Never** spawn unbounded goroutines that call CGo (pins OS threads → OOM).
- **Never** pass Go-managed memory to C that outlives the call.

### Testing
- **Never** use testify or other test frameworks — stdlib `testing` only.
- **Never** skip `-race` in CI.
- **Never** put benchmark timing in main.go — use `testing.B`.
- **Never** write a rule without a fixture test.

### Config
- **Never** evaluate user config at runtime — parse once at startup, compile to Go structs.
- **Never** support cascading configs (ESLint-style) — flat config with overrides only.

---

## Formatting

- **gofumpt** (not gofmt). Enforces grouped imports, trailing commas, no empty lines at block boundaries.
- Run `make fmt` before committing.
- CI verifies via `make verify`.

---

## Commit Messages

```
<type>: <short description>

<optional body — what and why, not how>
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `ci`, `build`, `perf`, `chore`.

Examples:
```
feat: add regex rule engine with rure-go parallel scanning
fix: prevent duplicate diagnostics on same line
refactor: extract line index to separate package
test: add fixture tests for no-var rule
perf: batch CGo calls in regex scanner
```

---

## Key Decisions (Do Not Change Without Discussion)

1. **Go, not Rust** — goroutine-per-rule parallelism + rure-go proven 3.3x faster than Rust rayon.
2. **rure-go for regex** — Rust regex via CGo. Not Go stdlib regexp (15x slower).
3. **tree-sitter for parsing** — error-tolerant, incremental, CST. Will migrate to typescript-go when TS7 stabilizes.
4. **Declarative custom rules** — no JS runtime at lint time. Config parsed once, compiled to native matchers.
5. **Flat config with overrides** — no ESLint-style cascading. One config per project.
6. **SQLite for cache** — pure Go (modernc.org/sqlite), concurrent reads, single file.
7. **WASM for plugins** — Wazero runtime, sandboxed, language-agnostic. Also used for dprint formatter.
8. **No `pkg/` directory** — everything in `internal/`. This is a CLI tool, not a library.

---

## Current Phase

**Phase 1 — Linter MVP (v0.1)**

See [docs/ARCHITECTURE.md → Implementation Plan](docs/ARCHITECTURE.md) for week-by-week breakdown.
