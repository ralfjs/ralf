# BEPRO

Fast, project-aware JS/TS linter and formatter with declarative custom rules and incremental cross-file analysis.

Written in Go. Regex engine powered by Rust's `regex` crate via [rure-go](https://github.com/BurntSushi/rure-go). AST parsing via [tree-sitter](https://tree-sitter.github.io/tree-sitter/).

> **Status: Early development.** Not ready for production use.

## Why

| | ESLint | Biome | Prettier | BEPRO |
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

Go + rure-go is **3.3x faster** than Rust parallel and **6.1x faster** than Rust single-thread on this workload. Details in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md#benchmark-results-proven).

## Features

### Linter

| Feature | Status | Description |
|---|---|---|
| Regex rules (rure-go) | Planned | Pattern-based lint rules via Rust regex engine |
| AST pattern matching | Planned | ast-grep-style `$VAR` / `$$$ARGS` syntax |
| Structural queries | Planned | `ast: { kind, parent, ancestor, capture }` |
| Naming conventions | Planned | `naming: { match }` on AST captures |
| Import ordering | Planned | `imports: { groups, alphabetize }` |
| Complexity checks | Planned | Cyclomatic complexity threshold |
| Inline suppression | Planned | `// lint-disable-next-line`, block disables |
| Built-in rules (50+) | Planned | ESLint recommended + React plugin equivalents |

### Custom Rules (Declarative)

| Feature | Status | Description |
|---|---|---|
| Regex rules in config | Planned | `regex: "pattern"` — compiled to rure-go |
| AST patterns in config | Planned | `pattern: "console.log($$$)"` — native matching |
| Structural queries in config | Planned | `ast: { kind, ancestor, enclosingFunction }` |
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
| SQLite project cache | Planned | Per-file cache with content hashing |
| Module graph | Planned | Import/export dependency tracking |
| Cross-file rules | Planned | Unused exports, circular deps, layer violations, dead modules |
| File watcher | Planned | fsnotify + cascade invalidation |
| Incremental re-analysis | Planned | Only changed files + their dependents |

### LSP + Editor Integration

| Feature | Status | Description |
|---|---|---|
| LSP server | Planned | JSON-RPC over stdio |
| Push diagnostics | Planned | Real-time lint errors in editor |
| Quick fixes | Planned | Code actions for auto-fixable rules |
| Format on save | Planned | Integrated formatter |
| Go to definition | Planned | Import → export via module graph |
| Find references | Planned | All importers of a symbol |
| VS Code extension | Planned | Language client + config intellisense |

### Auto-Fix

| Feature | Status | Description |
|---|---|---|
| Template fixes | Planned | `fix: { replace: "const $NAME = $VALUE" }` |
| Safe / unsafe categories | Planned | `--fix` (safe), `--fix-unsafe` (all), `--fix-dry-run` |
| Conflict resolution | Planned | Overlapping fixes resolved by severity + specificity |

### CLI

| Feature | Status | Description |
|---|---|---|
| `bepro lint` | Planned | Lint files with configurable rules |
| `bepro format` | Planned | Format files |
| `bepro check` | Planned | Lint + format check (for CI) |
| `bepro init` | Planned | Generate config, migrate from ESLint/Biome |
| `bepro lsp` | Planned | Start LSP server |
| `bepro debug` | Planned | Inspect rules, AST, module graph |
| Output formats | Planned | Stylish, JSON, SARIF, GitHub Actions annotations |

### Config

| Feature | Status | Description |
|---|---|---|
| JSON config | Planned | `.lintrc.json` |
| YAML config | Planned | `.lintrc.yaml` |
| TOML config | Planned | `.lintrc.toml` |
| JS config | Planned | `.lintrc.js` via goja (eval once) |
| `extends` | Planned | Inherit from shared config packages |
| `overrides` | Planned | Glob-scoped rule overrides |
| Monorepo workspaces | Planned | Per-workspace config with shared base |
| ESLint migration | Planned | `bepro init --from-eslint` |
| Biome migration | Planned | `bepro init --from-biome` |

## Roadmap

| Milestone | Target | Key Deliverable |
|---|---|---|
| **v0.1** | Month 5 | Linter MVP — regex + AST patterns, CLI, 50 rules |
| **v0.2** | Month 8 | Project-aware — cache, module graph, LSP, VS Code |
| **v0.3** | Month 11 | Formatter — dprint WASM, auto-fix, import sorting |
| **v0.4** | Month 13 | WASM plugins — Go/Rust/AS SDKs |
| **v1.0** | Month 16 | Type-aware rules via typescript-go, production-ready |

## Project Structure

```
cmd/bepro/              # CLI entry point (thin)
internal/
  engine/               # Rule execution (regex, AST, structural, naming)
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
| Language | Go 1.22+ | Goroutine parallelism, fast compilation, single binary |
| Regex engine | rure-go (Rust regex via CGo) | 3.3x faster than Rust rayon in benchmarks |
| Parser | tree-sitter (Phase 1), typescript-go (Phase 4) | Error-tolerant, incremental, full TS support |
| Formatter | dprint WASM (Phase 3), native printer (later) | Prettier-compatible without writing a printer |
| Cache | SQLite (modernc.org/sqlite, pure Go) | Concurrent reads, single file, indexed |
| WASM runtime | Wazero (pure Go) | Plugins + dprint, no CGo dependency |
| Config eval | goja (pure Go) | Evaluate .lintrc.js once at startup |
| File watching | fsnotify | Cross-platform, standard Go library |
| Hashing | xxhash | Fast content-based cache invalidation |

## Documentation

- [Architecture & Design](docs/ARCHITECTURE.md) — full technical spec: benchmarks, architecture, declarative API, cross-file analysis, implementation plan, Go conventions

## License

TBD
