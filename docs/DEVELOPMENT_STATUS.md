# RALF — Development Status (pre-v0.1)

> This is a snapshot of the development-phase README, preserved for reference.
> The main README was rewritten for the v0.1 public release.

[![CI](https://github.com/ralfjs/ralf/actions/workflows/ci.yml/badge.svg)](https://github.com/ralfjs/ralf/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/ralfjs/ralf/branch/main/graph/badge.svg)](https://codecov.io/gh/ralfjs/ralf)
[![Go Report Card](https://goreportcard.com/badge/github.com/ralfjs/ralf)](https://goreportcard.com/report/github.com/ralfjs/ralf)

Fast, project-aware JS/TS linter and formatter with declarative custom rules and incremental cross-file analysis.

Written in Go. Regex engine powered by Rust's `regex` crate via [rure-go](https://github.com/BurntSushi/rure-go). AST parsing via [tree-sitter](https://tree-sitter.github.io/tree-sitter/).

> **Status: Early development.** Not ready for production use.

## Why

| | ESLint | Biome | Prettier | RALF |
|---|---|---|---|---|
| Language | JS | Rust | JS | Go |
| Speed | Slow | Fast | Slow | Fast |
| Cross-file analysis | Plugins, re-parse all | None (single file) | N/A | First-class, incremental |
| Custom rules | JS visitors (slow) | None yet | N/A | Declarative (native speed) |
| Watch + LSP | Third-party | Partial | N/A | Built-in, cache-backed |
| Formatter | N/A | Built-in | Built-in | Built-in |
| Plugin escape hatch | N/A | N/A | N/A | WASM |

## Benchmarks

Tested on Apple Silicon (14 cores), 390K lines of JS, 30 lint rules, 100 iterations averaged:

| Approach | Avg per run |
|---|---|
| Go `regexp` stdlib | ~400ms+ |
| Rust single-thread (`regex` crate) | 135ms |
| Rust parallel (rayon, 14 cores) | 73ms |
| **Go + rure-go guarded (14 workers)** | **22ms** |

Go + rure-go is **3.3x faster** than Rust parallel and **6.1x faster** than Rust single-thread on this workload. Details in [ARCHITECTURE.md](ARCHITECTURE.md#benchmark-results-proven).

## Features

### Linter

| Feature | Status | Description |
|---|---|---|
| Regex rules (rure-go) | ✅ Implemented | Pattern-based lint rules via Rust regex engine |
| AST pattern matching | ✅ Implemented | ast-grep-style `$VAR` / `$$$ARGS` syntax |
| Structural queries | ✅ Implemented | `ast: { kind, name, parent, not }` with symbol ID optimization |
| Naming conventions | ✅ Implemented | `naming: { match }` as modifier on `ast` rules |
| Auto-fix | ✅ Implemented | `--fix` / `--fix-dry-run`, conflict resolution |
| Built-in rules (49) | ✅ Implemented | ESLint recommended equivalents with zero-config fallback |
| Import ordering | ✅ Implemented | `imports: { groups, alphabetize, newlineBetween }` |
| Custom Go built-in rules | ✅ Implemented | Kind-indexed single-walk dispatch for complex AST checks |
| Complexity checks | Planned | Cyclomatic complexity threshold |
| Inline suppression | ✅ Implemented | `lint-disable-next-line`, `lint-disable`/`lint-enable` blocks, `lint-disable-file`, same-line disable |

### Custom Rules (Declarative)

| Feature | Status | Description |
|---|---|---|
| Regex rules in config | ✅ Implemented | `regex: "pattern"` — compiled to rure-go |
| AST patterns in config | ✅ Implemented | `pattern: "console.log($$$)"` — native matching |
| Structural queries in config | ✅ Implemented | `ast: { kind, name, parent, not }` + `naming: { match }` |
| Capture + assertions | Planned | `capture: { name: "$X" }`, `assert: { "$X": ... }` |
| Cross-file rules | Planned | `scope: "cross-file"` — module graph queries |
| WASM plugin escape hatch | Planned | Imperative rules in Go/Rust/AS compiled to WASM |

### Formatter

| Feature | Status | Description |
|---|---|---|
| dprint WASM integration | Planned | Prettier-compatible formatting via Wazero |
| Import auto-sorting | Planned | Group, alphabetize, remove unused |
| Native CST printer | Planned | Full control over output (long-term replacement for dprint) |

### Project-Aware Analysis

| Feature | Status | Description |
|---|---|---|
| SQLite project cache | ✅ Implemented | Per-file cache with xxhash content hashing, WAL mode |
| Module graph | ✅ Implemented | Import/export extraction, specifier resolution, incremental updates |
| Cross-file rules | ✅ Implemented | no-unused-exports, no-circular-deps, no-dead-modules |
| File watcher | ✅ Implemented | fsnotify + debounced cascade invalidation, `ralf lint --watch` |
| Incremental re-analysis | ✅ Implemented | Content hash dedup, export-change cascade to dependents |

### LSP + Editor Integration

| Feature | Status | Description |
|---|---|---|
| LSP server skeleton | ✅ Implemented | JSON-RPC over stdio, initialize/shutdown/exit lifecycle |
| Push diagnostics | ✅ Implemented | Lint on open/change (debounced)/save, `textDocument/publishDiagnostics`, cross-file diagnostics via graph |
| Quick fixes | Planned | Code actions for auto-fixable rules |
| Format on save | Planned | Integrated formatter |
| Go to definition | Planned | Import → export via module graph |
| Find references | Planned | All importers of a symbol |
| VS Code extension | Planned | Language client + config intellisense |
| Zed extension | Planned | Rust extension via zed_extension_api |
| WebStorm plugin | Planned | LSP client plugin, run configuration |

### Auto-Fix

| Feature | Status | Description |
|---|---|---|
| Template fixes | ✅ Implemented | `fix: "replacement"` with capture substitution |
| Safe / unsafe categories | ✅ Implemented | `--fix` (apply), `--fix-dry-run` (preview) |
| Conflict resolution | ✅ Implemented | Overlapping fixes resolved, non-conflicting applied |

### CLI

| Feature | Status | Description |
|---|---|---|
| `ralf lint` | ✅ Implemented | Lint files with configurable rules |
| `ralf format` | Planned | Format files |
| `ralf check` | Planned | Lint + format check (for CI) |
| `ralf init` | ✅ Implemented | Generate config, migrate from ESLint/Biome |
| `ralf lsp` | ✅ Implemented | Start LSP server (stdio) |
| `ralf debug` | Planned | Inspect rules, AST, module graph |
| Output formats | ✅ Implemented | Stylish, JSON, compact, GitHub Actions, SARIF |

### Config

| Feature | Status | Description |
|---|---|---|
| JSON config | ✅ Implemented | `.ralfrc.json` |
| YAML config | ✅ Implemented | `.ralfrc.yaml` |
| TOML config | ✅ Implemented | `.ralfrc.toml` |
| JS config | ✅ Implemented | `.ralfrc.js` via goja (eval once) |
| `extends` | ✅ Implemented | Inherit from shared config files |
| `overrides` | ✅ Implemented | Glob-scoped rule overrides |
| Monorepo workspaces | Planned | Per-workspace config with shared base |
| ESLint migration | Planned | `ralf init --from-eslint` |
| Biome migration | Planned | `ralf init --from-biome` |

## Roadmap

| Milestone | Target | Key Deliverable |
|---|---|---|
| **v0.1** | Month 5 | Linter MVP — regex + AST patterns + builtin checkers, CLI, 61 rules |
| **v0.2** | Month 8 | Project-aware — cache, module graph, LSP, VS Code |
| **v0.3** | Month 11 | Formatter — dprint WASM, auto-fix, import sorting |
| **v0.4** | Month 13 | WASM plugins — Go/Rust/AS SDKs |
| **v1.0** | Month 16 | Type-aware rules via typescript-go, production-ready |

## Project Structure

```
cmd/ralf/              # CLI entry point (thin)
internal/
  engine/               # Rule execution (regex, AST, structural, naming, imports, builtin)
  parser/               # tree-sitter wrapper
  formatter/            # dprint WASM bridge → native printer
  project/              # Module graph, SQLite cache, file watcher
  lsp/                  # LSP server
  config/               # Config loader (JSON/YAML/TOML/JS)
  cli/                  # CLI commands
  plugin/               # WASM plugin host (Wazero)
testdata/               # Test fixtures
docs/                   # Architecture & design docs
```

## Tech Stack

| Component | Technology | Why |
|---|---|---|
| Language | Go 1.25+ | Goroutine parallelism, fast compilation, single binary |
| Regex engine | rure-go (Rust regex via CGo) | 3.3x faster than Rust rayon in benchmarks |
| Parser | tree-sitter (Phase 1), typescript-go (Phase 4) | Error-tolerant, incremental, full TS support |
| Formatter | dprint WASM (Phase 3), native printer (later) | Prettier-compatible without writing a printer |
| Cache | SQLite (modernc.org/sqlite, pure Go) | Concurrent reads, single file, indexed |
| WASM runtime | Wazero (pure Go) | Plugins + dprint, no CGo dependency |
| Config eval | goja (pure Go) | Evaluate .ralfrc.js once at startup |
| File watching | fsnotify | Cross-platform, standard Go library |
| Hashing | xxhash | Fast content-based cache invalidation |

## Documentation

- [Architecture & Design](ARCHITECTURE.md) — full technical spec: benchmarks, architecture, declarative API, cross-file analysis, implementation plan, Go conventions
- [Branching & Releases](BRANCHING.md) — Git Flow, branch naming, release process, versioning, merge strategy
- [Contributing](CONTRIBUTING.md) — dev setup, workflow, code style, testing

## License

[MIT](LICENSE)
