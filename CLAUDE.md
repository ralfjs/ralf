# CLAUDE.md

RALF — fast, project-aware JS/TS linter + formatter written in Go.

## Quick Reference

**Language:** Go 1.25+
**Module:** `github.com/Hideart/ralf`
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
cmd/ralf/main.go           # Entry point — MUST stay thin (~30 lines)
                             # Parse flags, wire deps, call internal packages. No logic here.

internal/
  engine/                    # Rule execution — the core of the linter
    engine.go                # Orchestrator: takes config + files, returns diagnostics
    regex.go                 # rure-go pattern matching (parallel, semaphore-limited)
    ast_pattern.go           # AST pattern matching ("console.log($$$)" syntax)
    structural.go            # Structural AST queries (kind, name, parent, not). Symbol ID optimization
    naming.go                # Naming convention checks on AST captures
    imports.go               # Import ordering / grouping analysis
    complexity.go            # Cyclomatic complexity
    crossfile.go             # Cross-file rule evaluation (uses project.Graph)
    fix.go                   # Auto-fix: Fix/Conflict types, ApplyFixes (single-pass), expandToStatement
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
    loader.go                # Load .ralfrc.{json,yaml,toml,js} → *Config struct
    compiler.go              # Compile declarative rules → engine representation
    schema.go                # Config validation
    defaults.go              # Built-in recommended ruleset

  cli/                       # CLI commands
    lint.go                  # ralf lint
    format.go                # ralf format
    check.go                 # ralf check (lint + format)
    init.go                  # ralf init, --from-eslint, --from-biome
    debug.go                 # ralf debug (rules, parse, graph)

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
cmd/ralf → internal/cli → internal/engine, internal/config, internal/project
internal/engine → internal/parser
internal/project → internal/parser, internal/engine
internal/lsp → internal/engine, internal/project, internal/formatter
internal/formatter → internal/parser
internal/plugin → internal/engine (plugin SDK types)
internal/config → standalone (no internal deps)
```

**Rules:**
- `cmd/ralf/main.go` imports only `internal/cli`. Never import engine/parser/etc directly from main.
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
- Return early on error — keep the happy path left-aligned:
  ```go
  result, err := doSomething()
  if err != nil {
      return fmt.Errorf("do something: %w", err)
  }
  // happy path continues here, not nested
  ```
- No naked returns in functions longer than a few lines. Named return values are fine for documentation but always return explicitly.

### Interfaces

- Accept interfaces, return structs.
- Define interfaces at the **consumer**, not the implementer.
- Keep interfaces small (1-3 methods).

### Context

- Pass `context.Context` as the first parameter to all functions that do I/O, CGo calls, or may need cancellation.
- CLI commands and LSP handlers create root contexts.
- Engine and parser functions accept context for cancellation support.
- Never store `context.Context` in a struct — pass it as a function parameter.

### Concurrency

- `wg.Add(1)` before `go func()`, never inside the goroutine.
- Semaphore pattern (`chan struct{}` sized to `runtime.NumCPU()`) for bounding CGo calls.
- Channel direction in signatures: `chan<-` (send), `<-chan` (receive).
- Never share `map` across goroutines without sync.

### Logging

- Use `log/slog` (structured logging) for all internal diagnostics and debug output.
- `fmt.Println` / `fmt.Fprintf(os.Stdout, ...)` is for **CLI user-facing output only** (lint results, formatted code).
- `slog.Debug` for verbose tracing (enabled with `--verbose`).
- `slog.Info` for operational messages (cache status, file count).
- `slog.Error` for errors that don't terminate the process.
- Never use the `log` package directly — always `log/slog`.

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
CGO_ENABLED=1 go test -race -count=1 -coverprofile=coverage.out ./...
```

- `CGO_ENABLED=1` required — project depends on CGo (rure-go, tree-sitter). Defaults to 1 on macOS but may be 0 on Linux CI.
- `-race` mandatory (concurrent code with CGo).
- `-count=1` disables cache (catches flaky tests).
- `-coverprofile` for coverage tracking in CI.

---

## Anti-Patterns (MUST NOT do)

### Architecture
- **Never** put logic in `cmd/ralf/main.go` — it must be a thin wrapper.
- **Never** import `internal/engine` from `internal/parser` — parser is lower-level.
- **Never** import `internal/project` from `internal/engine` — project calls engine, not reverse.
- **Never** use `pkg/` directory — everything goes in `internal/`.

### Code
- **Never** panic for recoverable errors — return errors.
- **Never** use `interface{}` / `any` when a concrete type is known.
- **Never** mutate shared state without synchronization.
- **Never** use `sync.Mutex` when a channel or `sync.WaitGroup` is clearer.
- **Never** ignore `golangci-lint` warnings — fix them or explicitly disable with a comment.
- **Never** use `init()` for complex initialization — use explicit setup functions called from main. `init()` is acceptable only for registering codecs or similar static setup.
- **Never** use naked returns in functions longer than a few lines.
- **Never** store `context.Context` in a struct — always pass as first parameter.

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

## Commit Messages (Conventional Commits)

Follows [Conventional Commits v1.0.0](https://www.conventionalcommits.org/).

### Format

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

- **Subject line:** max 72 characters, lowercase, no period at end.
- **Body:** wrap at 80 characters. Explain **what** and **why**, not how.
- **Footer:** `BREAKING CHANGE:` for breaking changes, `Refs:` for issue references.

### Types

| Type | When to use |
|---|---|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `perf` | Performance improvement (no behavior change) |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `test` | Adding or updating tests |
| `docs` | Documentation only |
| `build` | Build system, dependencies, Makefile, GoReleaser |
| `ci` | CI/CD pipeline changes (GitHub Actions, workflows) |
| `chore` | Maintenance (gitignore, tooling config, no production code) |

### Scopes

Scope maps to internal package or component:

| Scope | Maps to |
|---|---|
| `engine` | `internal/engine/` — rule execution, regex, AST matching |
| `parser` | `internal/parser/` — tree-sitter integration |
| `formatter` | `internal/formatter/` — dprint WASM, printer |
| `project` | `internal/project/` — cache, graph, watcher, scanner |
| `lsp` | `internal/lsp/` — LSP server |
| `config` | `internal/config/` — config loading, compilation |
| `cli` | `internal/cli/` — CLI commands |
| `plugin` | `internal/plugin/` — WASM plugin host |
| `rules` | Built-in rule definitions |
| `deps` | Dependency updates (go.mod, go.sum) |

Scope is optional but recommended. Omit only for cross-cutting changes.

### Examples

```
feat(engine): add regex rule engine with rure-go parallel scanning

Implements semaphore-limited goroutine-per-rule architecture.
Uses rure.Iter for bounded scanning with per-line dedup.

Refs: #12
```

```
fix(engine): prevent duplicate diagnostics on same line

The seen map was keyed by offset instead of line number,
allowing multiple diagnostics per rule on the same line.
```

```
perf(engine): batch CGo calls in regex scanner

Replaces per-line IsMatchBytes with IterBytes (single CGo call
per pattern). Reduces 2.5M CGo crossings to 30.
```

```
refactor(parser): extract line index to lineindex.go
```

```
test(rules): add fixture tests for no-var and no-console
```

```
feat(config): support YAML and TOML config formats
```

```
build(deps): update rure-go to v0.3.0
```

```
feat(cli)!: rename --format flag to --output

BREAKING CHANGE: --format is now used for the format command.
Use --output to specify output format (stylish, json, sarif).
```

### Rules

- **One logical change per commit.** Don't mix a feature with a refactor.
- **`feat` and `fix` appear in the changelog.** Other types don't. Choose accordingly.
- **Breaking changes** use `!` after type/scope AND a `BREAKING CHANGE:` footer.
- **Never use generic messages** like "update code", "fix stuff", "wip".
- **Reference issues** in the footer when applicable: `Refs: #42` or `Closes: #42`.
- **Imperative mood** in description: "add", "fix", "remove" — not "added", "fixes", "removed".

---

## Validation (MANDATORY)

**Every code change must be validated before considering it complete.**

### After Writing Code

1. **Run tests:** `make test` — all tests must pass.
2. **Run tests with race detector:** `make test-race` — no data races allowed.
3. **Run linter:** `make lint` — zero warnings.
4. **Check formatting:** `make fmt` then verify no diff.
5. **If you added a lint rule:** there must be a corresponding fixture test in `testdata/rules/<rule-name>/`.
6. **If you changed engine/parser/project:** run `make bench` and note any regressions.

### Before Committing or Creating a PR

**BLOCKING: You MUST run the full validation sequence and confirm all steps pass before committing code or creating a PR.** Do not skip any step. CI runs `make lint` with gocritic, which catches issues like `rangeValCopy` (large struct copies in range loops) and `hugeParam` (large value parameters) that compile and test fine locally but fail CI.

- Confirm all tests pass. If tests fail, fix the code — don't skip or disable tests.
- If a new test is needed for the change, write it first or alongside the implementation.
- Never commit code that doesn't compile.

### Validation Sequence

```bash
make build      # must compile
make test-race  # must pass with zero races
make lint       # MUST pass with zero warnings — CI will reject the PR otherwise
make fmt        # then: git diff — must be clean
```

If any step fails, fix the issue before proceeding. Do not ask whether to skip.

---

## Branching & Git Workflow

Full details: [docs/BRANCHING.md](docs/BRANCHING.md)

### Branch Model

- **`main`** — stable releases only. Every commit is a tagged release or hotfix.
- **`develop`** — integration branch. Features merge here first.
- **Feature branches** — `feat/<description>` from `develop`, PR back to `develop`.
- **Release branches** — `release/v<version>` from `develop`, PR to `main`.
- **Hotfix branches** — `hotfix/v<version>-<description>` from `main`, PR to `main`.

### Branch Naming

| Pattern | Example |
|---|---|
| `feat/<ticket>-<description>` | `feat/BP-42-ast-pattern-matching` |
| `fix/<ticket>-<description>` | `fix/BP-55-duplicate-diagnostics` |
| `refactor/<description>` | `refactor/extract-line-index` |
| `test/<description>` | `test/add-pattern-fixtures` |
| `docs/<description>` | `docs/update-architecture` |
| `chore/<description>` | `chore/update-deps` |
| `perf/<description>` | `perf/batch-cgo-calls` |
| `release/v<version>` | `release/v0.1.0` |
| `hotfix/v<version>-<description>` | `hotfix/v0.1.1-fix-crash` |

Rules: lowercase, hyphen-separated, no nested slashes beyond prefix.

### Workflow Rules

- **Never push directly to `main` or `develop`.** Always use PRs.
- **Feature → `develop`:** squash merge. One commit per feature.
- **Release → `main`:** merge commit. Preserve release history.
- **Hotfix → `main`:** merge commit. Then back-merge `main` into `develop`.
- **Always run `make test-race` before pushing.** CI will catch it, but catch it locally first.
- **Delete branches after merge.**
- **Create branch from the correct base:** features from `develop`, hotfixes from `main`.

### Release Process

1. Create `release/v<version>` from `develop`
2. Version bump, changelog update, final fixes only — NO new features
3. PR to `main`, merge, tag with `v<version>`
4. GoReleaser builds on tag push
5. Back-merge `main` into `develop`

### Versioning

Semantic Versioning: `v<MAJOR>.<MINOR>.<PATCH>`
- Pre-1.0: `MINOR` bumps may include breaking changes
- Tags only on `main`, annotated (`git tag -a`)

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
9. **Git Flow branching** — `main` (releases) + `develop` (integration). See [docs/BRANCHING.md](docs/BRANCHING.md).

---

## Current Phase

**Phase 1 — Linter MVP (v0.1)**

See [docs/ARCHITECTURE.md → Implementation Plan](docs/ARCHITECTURE.md) for week-by-week breakdown.
