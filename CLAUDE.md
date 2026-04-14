# CLAUDE.md

RALF — fast, project-aware JS/TS linter + formatter written in Go.

## Quick Reference

**Language:** Go 1.25+ | **Module:** `github.com/ralfjs/ralf`
**CGo deps:** rure-go (Rust regex), go-tree-sitter
**Pure Go deps:** modernc.org/sqlite, fsnotify, goja, wazero, xxhash

```bash
make build       # Build binary
make test        # Run tests
make test-race   # Tests with race detector
make lint        # golangci-lint (includes gocritic)
make fmt         # gofumpt
make bench       # Benchmarks
make verify      # CI check (lint + format + mod tidy)
```

## Architecture

Full spec: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)

### Dependency Direction

```
cmd/ralf → internal/cli → internal/engine, internal/config, internal/project
internal/engine → internal/parser
internal/project → internal/parser, internal/engine
internal/lsp → internal/engine, internal/config, internal/project, internal/crossfile, internal/parser (will add: internal/formatter)
internal/formatter → internal/parser
internal/plugin → internal/engine (plugin SDK types)
internal/config → standalone (no internal deps)
```

- `cmd/ralf/main.go` imports only `internal/cli`. Must stay thin (~30 lines).
- `internal/config` has zero internal deps. `internal/parser` must not depend on `internal/engine`. `internal/engine` must not depend on `internal/project`.
- No `pkg/` directory — everything in `internal/`.

## Code Style

- **Naming:** `MixedCaps` only (no underscores except test functions). Receivers: 1-2 letter. Acronyms all-caps (`AST`, `ID`). Getters drop `Get`. Packages: lowercase single word.
- **Errors:** Always check. Wrap with context: `fmt.Errorf("compile rule %q: %w", name, err)`. Use `errors.Is()`/`errors.As()`. Return early, happy path left-aligned. Never panic for recoverable errors.
- **Interfaces:** Accept interfaces, return structs. Define at consumer. Keep small (1-3 methods).
- **Context:** `context.Context` as first param. Never store in struct.
- **Concurrency:** `wg.Add(1)` before `go func()`. Semaphore `chan struct{}` sized to `runtime.NumCPU()` for CGo. Never share `map` without sync.
- **Logging:** `log/slog` only (never `log` package). `fmt.Println` for CLI user-facing output only.
- **CGo:** Minimize crossings (batch APIs). Semaphore-limit concurrent calls. Never pass Go pointers retained after call.
- **Formatting:** gofumpt (not gofmt). Run `make fmt` before committing.

## Testing

- Table-driven tests, `t.Run()` subtests, `t.Parallel()` for independent tests.
- **No testify** — stdlib `testing` only.
- Naming: `TestFunctionName_Scenario`.
- Each rule needs `testdata/rules/<rule-name>/valid.js` and `invalid.js` with `// expect-error: <rule>` comments.
- CI: `CGO_ENABLED=1 go test -race -count=1 ./...`

## Commit Messages

[Conventional Commits](https://www.conventionalcommits.org/): `<type>(<scope>): <description>`

**Types:** feat, fix, perf, refactor, test, docs, build, ci, chore
**Scopes:** engine, parser, formatter, project, lsp, config, cli, plugin, rules, deps, crossfile

Subject max 72 chars, lowercase, imperative mood, no period. One logical change per commit. Reference issues: `Refs: #42`.

## Validation (MANDATORY)

**BLOCKING: Run full sequence before committing or creating a PR.**

```bash
make build      # must compile
make test-race  # zero races
make lint       # zero warnings (gocritic catches rangeValCopy, hugeParam that pass locally)
make fmt        # then: git diff — must be clean
```

If any step fails, fix before proceeding. Never skip. If you added a lint rule, must have fixture test in `testdata/rules/<rule-name>/`.

## Branching

Full details: [docs/BRANCHING.md](docs/BRANCHING.md)

- **`main`** — releases only. **`develop`** — integration. Features from `develop`, hotfixes from `main`.
- Branch names: `feat/<desc>`, `fix/<desc>`, `refactor/<desc>`, `release/v<version>`, `hotfix/v<version>-<desc>`. Lowercase, hyphen-separated.
- Never push directly to `main` or `develop`. Feature → develop: squash merge. Release/hotfix → main: merge commit.

## Key Decisions (Do Not Change)

1. **Go** — goroutine-per-rule + rure-go proven 3.3x faster than Rust rayon
2. **rure-go** for regex (not Go regexp, 15x slower)
3. **tree-sitter** for parsing (migrate to typescript-go when TS7 stabilizes)
4. **Declarative custom rules** — no JS runtime at lint time
5. **Flat config** — no ESLint-style cascading
6. **SQLite cache** — modernc.org/sqlite, pure Go
7. **WASM plugins** — Wazero, also for dprint formatter
8. **Git Flow** — main + develop

**Current Phase:** Phase 1 — Linter MVP (v0.1)
