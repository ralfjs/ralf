# JS/TS Linter + Formatter — Architecture & Research

## Executive Summary

A fast, project-aware JS/TS linter and formatter written in Go, with declarative custom rules and incremental cross-file analysis. Positioned against ESLint (slow, JS-based), Biome (no custom rules, no cross-file analysis), and Prettier (formatter only).

**Core differentiators:**
- Project-wide incremental analysis with persistent module graph
- Declarative custom rules executed at native speed (no JS runtime)
- Cross-file rules as first-class primitives (unused exports, circular deps, layer violations)
- Watch mode + LSP with cache-backed instant diagnostics

---

## Competitive Landscape

| | ESLint | Biome | Prettier | This Tool |
|---|---|---|---|---|
| Language | JS | Rust | JS | Go |
| Speed | Slow | Fast | Slow | Fast (Go + rure-go) |
| Cross-file analysis | Plugins, re-parse all | None (single file only) | N/A | First-class, incremental graph |
| Custom rules | JS visitors (slow) | None yet | N/A | Declarative config (native speed) |
| Watch + LSP | Third-party | Partial | N/A | Built-in, cache-backed |
| Formatter | N/A | Built-in | Built-in | Built-in |
| Plugin ecosystem | Massive | Growing | Limited | Declarative + WASM escape hatch |

---

## Benchmark Results (Proven)

Benchmarks run on Apple Silicon (14 cores), 390K lines of JS, 30 lint rules, 100 iterations averaged.

| Approach | Avg per run | vs Rust parallel |
|---|---|---|
| Go `regexp` stdlib | ~400ms+ | Baseline Go (slow) |
| Rust single-thread (`regex` crate) | 135ms | 1.9x slower than Rust parallel |
| Rust parallel (rayon, 14 cores) | 73ms | Baseline |
| **Go + rure-go guarded (14 workers)** | **22ms** | **3.3x faster** |
| Go + rure-go unguarded | 13ms | 5.6x faster |

### Key Findings

1. **Go's `regexp` stdlib is the bottleneck, not Go itself.** `strings.Contains`/`strings.Index` (which use platform-optimized assembly, Rabin-Karp + SIMD) matched Rust regex speed on simple patterns.

2. **rure-go (`github.com/BurntSushi/rure-go`)** — Rust's regex engine via CGo. `FindAllBytes` does iteration in C (single CGo call per pattern), eliminating per-line CGo overhead. Batching 2.5M CGo calls down to 8 = 10x speedup.

3. **Goroutine-per-rule architecture beats Rust's rayon** because each goroutine gets its own regex engine instance with dedicated DFA cache (no contention), while Rust's rayon threads fight over shared lazy-DFA thread-local caches.

4. **Rust rayon parallel was slower than expected** due to DFA cache contention. Per-thread regex compilation helped but never achieved good linear speedup.

5. **All alternative Go regex engines tested and benchmarked:**
   - `wasilibs/go-re2` — RE2 via Wasm, 1.6x slower than stdlib
   - `coregex` — "3-3000x faster" claims are cherry-picked for grep-style patterns with extractable literal anchors. 1.2x slower than stdlib on complex patterns.
   - `grafana/regexp` — Same speed as stdlib
   - None beat stdlib on general patterns

6. **WASM compilation penalty:** Go WASM 26x slower, Rust WASM 4x slower. Native binaries only.

### Production Guardrails (Implemented in Benchmarks)

- **Semaphore** — `chan struct{}` sized to `runtime.NumCPU()` limits concurrent CGo threads
- **Iter-based scanning** — `rure.Iter` instead of `FindAllBytes`, avoids bulk C malloc
- **Match cap** — `maxMatchesPerRule = 10000`, breaks early per rule
- **Per-line dedup** — `seen` map ensures one diagnostic per rule per line
- **Line/col resolution** — Binary search on pre-built `lineStarts` index

Cost of guardrails: ~9ms (13ms → 22ms). Acceptable for production safety.

---

## Technology Choices

### Parser

| Option | TS Support | Status | Viability |
|---|---|---|---|
| **tree-sitter** (Phase 1) | Full | Stable, Go bindings exist | Battle-tested, error-tolerant, incremental, gives CST |
| **`microsoft/typescript-go`** (Phase 3) | Full | Active, Microsoft, shipping as TS 7 | Full type info, canonical parser. Wait for API stability. |
| esbuild (fork) | Full | Active | Production-grade but AST deliberately internal. Fork required. |
| go-fAST | JS only | Unclear | No TypeScript support |

**Decision:** tree-sitter now (proven Go bindings, incremental reparsing, error-tolerant). Migrate to `typescript-go` when TypeScript 7 stabilizes — unlocks type-aware rules.

### Regex Engine

**rure-go** — Rust's regex engine via CGo. Proven 3.3x faster than Rust's own rayon parallel in our benchmarks. Requires `librure` C library (built from `github.com/rust-lang/regex` capi).

Build: `cargo build --release` in `regex-capi/` → `librure.{a,dylib}`
Go link: `CGO_LDFLAGS="-L<path> -lrure"`

### Formatter

**Phase 1:** Embed dprint's TypeScript WASM plugin via Wazero (Go WASM runtime). Get Prettier-compatible formatting without writing a printer. dprint plugins are standard `.wasm` files with a defined host protocol.

**Phase 2:** Write native CST printer in Go for full control. This is the hardest component (~6-12 months).

### Cache Storage

**SQLite** via `modernc.org/sqlite` (pure Go, no CGo dependency for cache layer).
- Concurrent read-safe (LSP reads while watcher writes)
- Single file: `.yourlinter/cache.db` at project root
- ~50-100MB for a 10K file project
- Indexed by path + content hash

### LSP

Go has established LSP patterns from `gopls`. Use `golang.org/x/tools/gopls/internal/protocol` or `gorilla/jsonrpc2`.

### File Watching

`fsnotify` — cross-platform Go library for filesystem events.

---

## Architecture

### High-Level

```
┌──────────────────────────────────────────────┐
│              JS/JSON config                   │  ← user writes declarative rules
│              (.lintrc.js)                     │
└──────────────────┬───────────────────────────┘
                   │ parsed once at startup (goja)
┌──────────────────▼───────────────────────────┐
│              Go Core Engine                   │
│                                               │
│  ┌─────────┐ ┌──────────┐ ┌───────────────┐  │
│  │ Parser  │ │  Linter  │ │  Formatter    │  │
│  │(tree-   │ │          │ │               │  │
│  │ sitter) │ │ regex    │ │ dprint WASM   │  │
│  │         │ │ (rure-go)│ │ → own printer │  │
│  │         │ │          │ │               │  │
│  │         │ │ AST pat. │ │               │  │
│  │         │ │ matching │ │               │  │
│  └─────────┘ └──────────┘ └───────────────┘  │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │          Project Layer                   │ │
│  │  Module Graph + SQLite Cache + Watcher   │ │
│  └──────────────────────────────────────────┘ │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │          LSP Server                      │ │
│  │  JSON-RPC over stdio                     │ │
│  └──────────────────────────────────────────┘ │
└───────────────────────────────────────────────┘
```

### Project Layer

```
First run:
  scan all files → parse → analyze → build module graph → cache to disk

Watch mode:
  fs event (file changed)
    → content hash check (skip if unchanged)
    → reparse that file
    → re-analyze
    → update cache
    → exports changed? → cascade re-lint dependents
    → push diagnostics via LSP

LSP:
  editor opens → read from cache → instant diagnostics
  user types → dirty file re-analyzed → push diagnostics
  user saves → cache updated → cross-file rules re-evaluated
```

### Per-File Cache Structure

```go
type FileCache struct {
    Path         string
    ContentHash  uint64          // xxhash — skip if unchanged
    ModTime      int64
    Tree         []byte          // serialized tree-sitter CST
    Diagnostics  []Diagnostic    // pre-computed lint results
    Exports      []Symbol        // what this file exports
    Imports      []ImportRef     // what this file imports
    Symbols      []Symbol        // top-level declarations
}
```

### Module Graph

```go
type ProjectGraph struct {
    mu         sync.RWMutex
    files      map[string]*FileCache       // path → cache
    importedBy map[string][]string          // path → files that import it
    exportMap  map[string]map[string]Symbol  // path → exported symbols
}
```

### Cascade Invalidation

```
User edits utils.ts, removes exported function `formatDate`
  │
  ├─ Re-parse utils.ts → exportMap no longer has `formatDate`
  │
  ├─ graph.ImportedBy["utils.ts:formatDate"] → ["app.tsx", "header.tsx"]
  │
  ├─ Re-lint app.tsx → "import { formatDate }" → error: missing export
  ├─ Re-lint header.tsx → same
  │
  └─ LSP pushes diagnostics to both files within milliseconds
```

### Performance Targets

| Operation | Target | When |
|---|---|---|
| Cold start (10K files) | 5-15s | First run only |
| Warm start (cache hit) | <500ms | Cache exists, validate hashes |
| Single file re-lint | 1-5ms | On save / on type |
| Cascade re-check | 10-50ms | Changed file's importers |
| LSP diagnostic push | <1ms | After analysis completes |

---

## Custom Rules — Declarative API

### Design Principle

Users write declarative config describing **what to match**. Go executes everything natively. No JS runtime at execution time. Config file is JS/JSON for convenience (evaluated once at startup via `goja`), then compiled to native Go matchers.

### Rule Types

| Type | Config Field | Engine | Speed |
|---|---|---|---|
| Regex pattern | `regex: "..."` | rure-go | Fastest |
| AST pattern | `pattern: "console.log($$$)"` | tree-sitter query | Fast |
| Structural query | `ast: { kind, parent, children }` | tree-sitter traversal | Fast |
| Import ordering | `imports: { groups, ... }` | Custom Go analyzer | Fast |
| Naming convention | `naming: { match }` | Regex + AST | Fast |
| Complexity | `complexity: { max: 10 }` | AST visitor | Fast |
| Cross-file | `scope: "cross-file"` | Module graph query | Fast |

### Example Config

```js
// .lintrc.js
export default {
  rules: {
    // Regex mode — runs via rure-go
    "no-magic-timeouts": {
      regex: "setTimeout\\([^,]+,\\s*\\d{4,}\\)",
      message: "Extract timeout to named constant",
      severity: "warn"
    },

    // AST pattern — like ast-grep syntax
    // $NAME matches single node, $$$NAME matches variadic
    "no-console-in-prod": {
      pattern: "console.log($$$ARGS)",
      where: {
        file: "src/**",
        not: { file: "**/*.test.*" }
      },
      message: "No console.log in production code",
      severity: "error",
      fix: "// removed"
    },

    // Structural AST query
    "require-error-boundary": {
      ast: {
        kind: "jsx_element",
        name: /^[A-Z]/,
        parent: { not: { kind: "jsx_element", name: "ErrorBoundary" } }
      },
      where: { file: "src/pages/**" },
      message: "Page components must be wrapped in ErrorBoundary"
    },

    // Naming conventions with AST captures
    "react-boolean-prop-naming": {
      ast: {
        kind: "jsx_attribute",
        name: { type: "boolean" }
      },
      naming: {
        match: /^(is|has|should|can|will|did)/,
        message: "Boolean props must start with is/has/should/can/will/did"
      }
    },

    "react-callback-prop-naming": {
      ast: {
        kind: "jsx_attribute",
        name: { type: "function" }
      },
      naming: {
        match: /^on[A-Z]/,
        message: "Callback props must start with 'on'"
      }
    },

    // Cross-reference with captures and assertions
    "component-props-interface-naming": {
      ast: {
        kind: "function_declaration",
        returns: "jsx_element",
        capture: { name: "$COMP_NAME" },
        params: [{
          kind: "type_annotation",
          capture: { name: "$PROPS_TYPE" }
        }]
      },
      assert: {
        "$PROPS_TYPE": { equals: { concat: ["$COMP_NAME", "Props"] } }
      },
      message: "Props type for $COMP_NAME must be named $COMP_NAMEProps"
    },

    // Import ordering
    "consistent-imports": {
      imports: {
        groups: ["builtin", "external", "internal", "relative"],
        alphabetize: true,
        newlineBetween: true
      }
    },

    // Cross-file: layer violation detection
    "no-feature-cross-imports": {
      scope: "cross-file",
      ast: {
        kind: "import_declaration",
        source: { match: /^\.\.\/.*/ }
      },
      where: {
        file: "src/features/**",
        importCrosses: "src/features/*"
      },
      message: "Features must not import from each other. Use shared/."
    }
  }
}
```

### Query Primitives (Engine Built-ins)

| Primitive | Purpose | Example |
|---|---|---|
| `capture: { name }` | Bind AST node to variable | `$COMP_NAME` |
| `equals / match` | String comparison on captures | `$X == "foo"` |
| `concat / replace` | String transforms | `$NAME + "Props"` |
| `parent / children / siblings` | Direct parent/children/siblings | parent is not ErrorBoundary |
| `ancestor` | Any parent up the tree (not just direct) | hook inside any `if_statement` ancestor |
| `enclosingFunction` | Nearest function ancestor | function that returns JSX = component |
| `has / not / every / some` | Quantifiers over children | all props match pattern |
| `count` | Cardinality checks | max 5 props per component |
| `type` | TS type constraint | `boolean`, `function`, `string` |
| `scope` | Variable scope lookup | is this variable used? |
| `precedes / follows` | Ordering within same scope | import before usage |
| `precededBy / followedBy` | Statement ordering across blocks | hook after early return |
| `sameBlock` | Restrict match to same block scope | same `{}` block vs any ancestor |

### Advanced Declarative Examples

**React Rules of Hooks — conditional call detection:**

```js
"rules-of-hooks-conditional": {
  ast: {
    kind: "call_expression",
    name: { match: /^use[A-Z]/ },
    ancestor: {
      kind: ["if_statement", "switch_statement", "for_statement",
             "while_statement", "conditional_expression", "logical_expression"]
    }
  },
  where: {
    enclosingFunction: { returns: "jsx_element" }
  },
  message: "Hook called inside conditional. Hooks must be called in the same order every render."
}
```

**React Rules of Hooks — call after early return:**

```js
"rules-of-hooks-after-return": {
  ast: {
    kind: "call_expression",
    name: { match: /^use[A-Z]/ },
    precededBy: {
      kind: "return_statement",
      sameBlock: false
    }
  },
  where: {
    enclosingFunction: { returns: "jsx_element" }
  },
  message: "Hook called after early return. Hooks must be called before any conditional returns."
}
```

**Exhaustive switch on enum-like union:**

```js
"exhaustive-switch": {
  ast: {
    kind: "switch_statement",
    discriminant: { capture: { name: "$DISC" } },
    children: {
      kind: "switch_case",
      capture: { name: "$$$CASES" }
    }
  },
  assert: {
    "$$$CASES": {
      coversAll: { typeUnionOf: "$DISC" }
    }
  },
  message: "Switch on $DISC is not exhaustive. Missing cases."
}
```

These are all declarative — no WASM needed. The engine resolves `ancestor`, `enclosingFunction`, `precededBy`, and `coversAll` natively.

---

## Cross-File Analysis

Cross-file rules are graph queries on pre-computed data. No re-parsing.

| Rule | Graph Query | Cost |
|---|---|---|
| Unused exports | exportMap − importedBy | O(exports) |
| Circular dependencies | DFS cycle detection | O(edges) |
| Missing imports | import ref → exportMap lookup miss | O(1) per import |
| Duplicate exports | same symbol in exportMap from 2+ files | O(exports) |
| Barrel file bloat | count re-exports per index.ts | O(1) per file |
| Layer violations | import crosses boundary | path check on edge |
| Dead modules | file with 0 incoming edges | O(files) |
| Inconsistent naming | same symbol imported under different aliases | group-by on importedBy |

Without project cache: "is X unused?" → parse every file → extract imports → check → seconds.
With project cache: "is X unused?" → `graph.ImportedBy["utils.ts:X"].length == 0` → microseconds.

---

## Rule Taxonomy

### The Rule Pyramid

```
         ╱╲
        ╱5%╲          WASM plugins — imperative escape hatch
       ╱────╲         (cross-language, external data, multi-file AST)
      ╱  15% ╲        Structural queries + captures + assertions
     ╱────────╲       (hooks ordering, prop naming, component patterns)
    ╱   80%    ╲      Regex + AST patterns
   ╱────────────╲     (no-var, no-console, prefer-template)
```

Most users never leave the bottom two layers. The declarative engine with `ancestor`, `enclosingFunction`, `precededBy` primitives handles even complex rules like React Rules of Hooks (see Query Primitives section). The WASM escape hatch exists for the true edge cases that require external data or cross-language analysis.

### Layer Breakdown

```
Custom (declarative config, native execution — 97% of rules):
  ├─ Regex patterns           → rure-go
  ├─ AST patterns             → "console.log($$$)" syntax
  ├─ Structural queries       → ast: { kind, ancestor, enclosingFunction, precededBy }
  ├─ Naming conventions       → naming: { match } on captures
  ├─ Import ordering          → imports: { groups }
  └─ Cross-file constraints   → scope: "cross-file" + graph queries

WASM plugins (imperative escape hatch — 3% of rules):
  ├─ External data correlation → OpenAPI spec ↔ code, i18n keys ↔ usage
  ├─ Cross-language analysis   → GraphQL schema ↔ TS types, CSS classes ↔ JSX
  ├─ Multi-file AST traversal  → "component A rendered in route B without error boundary"
  └─ Non-JS file analysis      → lint .sql for injection, .css for unused classes

Built-in (Go code, engine-level):
  ├─ Unused variables         → scope analysis
  ├─ Unreachable code         → control flow graph
  ├─ Type-aware rules         → typescript-go integration (Phase 4)
  └─ Cross-file analysis      → module graph (always available)
```

### What Declarative Handles (Previously Thought to Need WASM)

Many complex rules that seem to require imperative code are actually expressible with the right declarative primitives:

| Rule | Why It Seems Hard | Declarative Solution |
|---|---|---|
| React hooks in conditional | Control flow analysis | `ancestor: { kind: "if_statement" }` + `enclosingFunction: { returns: "jsx_element" }` |
| Hooks after early return | Statement ordering | `precededBy: { kind: "return_statement", sameBlock: false }` |
| Boolean prop naming | Type inference | `ast: { kind: "jsx_attribute", name: { type: "boolean" } }` + `naming: { match }` |
| Exhaustive switch | Type union coverage | `assert: { coversAll: { typeUnionOf: "$DISC" } }` |
| No nested callbacks >3 deep | Depth counting | `ancestor: { kind: "arrow_function", minDepth: 3 }` |
| Require error boundary | Parent component check | `ast: { kind: "jsx_element", parent: { not: { name: "ErrorBoundary" } } }` |

### WASM Plugin Escape Hatch

The declarative config covers ~97% of custom rules. WASM plugins handle the remaining 3% — cases that fundamentally require reading external (non-JS) data or traversing multiple file ASTs simultaneously.

**What truly needs WASM (can't be declarative):**

```js
// Validate GraphQL query variables match TypeScript interface fields
// → requires parsing .graphql files (different grammar) AND cross-referencing
//   with TS types in a different file. Two different parsers + correlation logic.

// Ensure all i18n keys used in code exist in translation JSON files
// → requires reading external .json translation files, extracting keys,
//   and checking every t("key") call references a real key.

// Lint SQL template literals for injection patterns
// → requires a SQL parser to analyze tagged template strings like sql`SELECT ...`
//   with embedded expressions. Regex can't parse SQL reliably.

// Verify CSS module class names used in JSX exist in the .module.css file
// → requires parsing .css file, extracting class names, cross-referencing
//   with styles.className usage in JSX. Two file formats.

// "Component rendered in route without ErrorBoundary"
// → requires traversing route config AST AND component AST together,
//   checking parent tree across two different files simultaneously.
```

**The common thread:** all WASM use cases involve either **non-JS file parsing** or **multi-file AST correlation** that goes beyond the module graph's import/export tracking.

**How WASM plugins work:**

Users write a rule in any language that compiles to WASM (Go, Rust, AssemblyScript, C):

```go
// rules/api-error-check/main.go → compiled to api-error-check.wasm
package main

import plugin "github.com/yourorg/yourlinter-plugin-sdk"

func Run(ctx plugin.Context) []plugin.Diagnostic {
    tree := ctx.AST()
    var diags []plugin.Diagnostic

    // Full AST access via plugin SDK
    for _, node := range tree.FindAll("switch_statement") {
        // Read external file (sandboxed — only project files)
        spec := ctx.ReadFile("openapi.yaml")
        expectedCodes := parseErrorCodes(spec)
        actualCases := extractCaseValues(node)

        for _, code := range expectedCodes {
            if !contains(actualCases, code) {
                diags = append(diags, plugin.Diagnostic{
                    Node:    node,
                    Message: fmt.Sprintf("Missing case for error code %q", code),
                })
            }
        }
    }
    return diags
}
```

**Config integration:**

```js
// .lintrc.js
export default {
  rules: {
    "api-error-exhaustive": {
      plugin: "./rules/api-error-check.wasm",
      options: {
        specPath: "./openapi.yaml"
      },
      severity: "error"
    }
  }
}
```

**Engine execution flow:**

```
1. Load .wasm file via Wazero (same runtime used for dprint formatter)
2. Pass serialized AST + options + sandboxed file access API to WASM module
3. WASM rule executes in sandbox (no network, no arbitrary fs access)
4. Returns diagnostics array
5. Engine merges with declarative rule results
```

**Why WASM and not JS/Lua:**

| | WASM | JS (V8/goja) | Lua |
|---|---|---|---|
| Speed | Near-native | 10-100x slower | 5-50x slower |
| Sandbox | Built-in isolation | Requires careful sandboxing | Requires careful sandboxing |
| Languages | Any (Go, Rust, C, AS) | JavaScript only | Lua only |
| Ecosystem | Growing | Mature | Small |
| Already needed | Yes (dprint formatter) | No | No |

WASM is already a dependency (Wazero for dprint). No additional runtime needed.

**Plugin SDK provides:**

| API | Description |
|---|---|
| `ctx.AST()` | Parsed tree-sitter AST for current file |
| `ctx.Source()` | Raw source bytes |
| `ctx.ReadFile(path)` | Read project file (sandboxed to project root) |
| `ctx.Options()` | Rule options from config |
| `ctx.FilePath()` | Current file path |
| `ctx.Imports()` | Resolved imports (from module graph) |
| `ctx.Exports()` | Exports of current file |

**Distribution:** WASM plugins can be published as npm packages (just `.wasm` files + config schema) or shared as files in the repo.

---

## Project Structure

```
cmd/
  linter/                    # CLI: lint, format, check, init
  linter --lsp               # LSP server mode (same binary)

internal/
  parser/
    treesitter.go            # tree-sitter wrapper
    treesitter_queries.go    # pre-built queries for JS/TS/JSX/TSX

  linter/
    engine.go                # rule execution orchestrator
    regex.go                 # rure-go pattern rules
    ast_pattern.go           # AST pattern matching ("console.log($$$)")
    structural.go            # structural AST queries
    naming.go                # naming convention checker
    imports.go               # import ordering / grouping
    complexity.go            # cyclomatic complexity
    crossfile.go             # cross-file rule evaluation

  formatter/
    printer.go               # CST → formatted output
    dprint_wasm.go           # dprint WASM plugin bridge (Phase 1)

  project/
    graph.go                 # module graph + dependency tracking
    cache.go                 # SQLite cache layer
    watcher.go               # fsnotify + invalidation logic
    scanner.go               # initial project scan
    hasher.go                # xxhash content hashing

  lsp/
    server.go                # JSON-RPC handler
    diagnostics.go           # push diagnostics to editor
    formatting.go            # on-save / on-type formatting
    codelens.go              # inline code actions
    completion.go            # rule name completion in config

  config/                      # ✅ Implemented (Sprint 1)
    config.go                # Config/RuleConfig/Severity types, matcher stubs (AST, Imports, Naming, Where)
    loader.go                # Load (dir search) + LoadFile (JSON/YAML/TOML dispatch)
    validate.go              # Structural validation: one matcher, valid severity, override rules
    merge.go                 # Override resolution: file-glob matching, later-wins semantics
    defaults.go              # DefaultConfig (empty)
    compiler.go              # (planned) compile declarative rules → engine representation

  plugin/
    host.go                  # WASM plugin host (Wazero)
    sdk.go                   # Plugin SDK interface definitions
    sandbox.go               # Sandboxed file access for plugins
```

---

## Implementation Plan

### Overview

```
Phase 1 (v0.1) — Linter MVP                      Months 1-5
Phase 2 (v0.2) — Project-Aware + LSP              Months 5-8
Phase 3 (v0.3) — Formatter + Auto-Fix             Months 8-11
Phase 4 (v0.4) — WASM Plugins                     Months 11-13
Phase 5 (v1.0) — Type-Aware + Production          Months 13-16
```

Assumes 2 senior Go engineers full-time. Solo developer: multiply by 1.8-2x.

---

### Phase 1 — Linter MVP (v0.1) — Months 1-5

**Goal:** Usable linter with regex + AST pattern rules, declarative config, CLI output. Enough to replace ESLint for basic use cases.

**Month 1 — Foundation**

| Week | Task | Deliverable |
|---|---|---|
| 1 | Project scaffolding | Repo, `cmd/` + `internal/` layout, go.mod, Makefile, CI (lint + test), .golangci.yml |
| 2 | librure vendoring + build | Static librure.a for macOS arm64/x64 + Linux x64. CGo build working. Smoke test: compile regex, match string. |
| 3 | tree-sitter integration | `internal/parser/`: load JS/TS/JSX/TSX grammars, parse file to AST, walk nodes. Tests: parse valid + invalid files. |
| 4 | ✅ Config loader (JSON/YAML/TOML) | `internal/config/`: Config types, Load/LoadFile (JSON/YAML/TOML), Validate (rules + overrides), Merge with file-scoped overrides. 24 tests, 6 fixtures. Known limitations: no `**` globstar (#4), no JSONC (#5), no field-level override merge (#3). |

**Month 2 — Regex Engine + CLI Shell**

| Week | Task | Deliverable |
|---|---|---|
| 5 | Regex rule engine | `internal/engine/regex.go`: compile rure-go patterns, parallel scan (semaphore, iter-based, dedup, match cap). Port from benchmark prototype. |
| 6 | Line/col resolution | `internal/engine/lineindex.go`: `buildLineIndex`, `offsetToLine`. Diagnostic struct with file, line, col, end col, rule name, message, severity. |
| 7 | CLI `lint` command | `internal/cli/lint.go`: file discovery (glob + ignore), parallel file processing (goroutine per file), output formatters (stylish, JSON). Exit codes. |
| 8 | First 20 regex rules | Built-in rules: no-var, no-console, no-eval, no-debugger, eqeqeq, no-alert, no-inner-html, etc. Fixture tests for each. |

**Month 3 — AST Pattern Matching (Critical Path)**

| Week | Task | Deliverable |
|---|---|---|
| 9 | Pattern parser | Parse `"console.log($$$ARGS)"` into a pattern AST. Handle `$NAME` (single node) and `$$$NAME` (variadic). |
| 10 | Pattern matcher | Match pattern AST against tree-sitter AST. Capture bindings. Handle nested patterns. |
| 11 | Pattern integration | Wire pattern matcher into engine. Config: `pattern: "..."` field compiles to matcher. Tests: 10+ pattern-based rules. |
| 12 | Structural queries | `ast: { kind, parent, children, capture }` syntax. Compile to tree-sitter query or custom walker. |

**Month 4 — Naming Rules + Rule Expansion**

| Week | Task | Deliverable |
|---|---|---|
| 13 | Naming convention engine | `naming: { match }` on captures. Integrate with structural queries. Rules: component naming, prop naming, file naming. |
| 14 | Import analysis | `imports: { groups, alphabetize }` — parse import statements, detect ordering violations. |
| 15 | 30 more built-in rules | Total: 50 rules. Cover ESLint recommended + React plugin essentials. Each with fixture test. |
| 16 | Inline suppression | Parse `// lint-disable-next-line`, `// lint-disable`, `/* lint-disable-file */`. Skip diagnostics for suppressed ranges. |

**Month 5 — Polish + Release**

| Week | Task | Deliverable |
|---|---|---|
| 17 | Config JS loader (goja) | `.lintrc.js` support via goja. Evaluate once, extract static config object. `extends` resolution. |
| 18 | Output formats | SARIF, GitHub Actions annotations, compact format. `--format` flag. |
| 19 | `yourlinter init` | Generate config from scratch. `--from-eslint` migration (rule name mapping table). |
| 20 | Release prep | Cross-compile (macOS arm64/x64, Linux x64/arm64). GoReleaser config. npm wrapper package. README. |

**v0.1 deliverable:** `yourlinter lint`, `yourlinter check`, `yourlinter init`. 50 rules. JSON/YAML/JS config. Stylish + JSON + SARIF output. npm + homebrew + GitHub Releases.

---

### Phase 2 — Project-Aware + LSP (v0.2) — Months 5-8

**Goal:** Persistent project cache, module graph, cross-file rules, LSP server, VS Code extension.

**Month 5-6 — Cache + Module Graph**

| Week | Task | Deliverable |
|---|---|---|
| 21 | SQLite cache layer | `internal/project/cache.go`: per-file cache (content hash, diagnostics, exports, imports). WAL mode. Read/write benchmarks. |
| 22 | Project scanner | `internal/project/scanner.go`: walk project, parse all files, populate cache. Parallel file processing. Progress reporting. |
| 23 | Module graph | `internal/project/graph.go`: build import→export graph from cache. `ImportedBy`, `ExportedBy`, `ExportMap` queries. Cycle detection. |
| 24 | Incremental update | Content hash check → skip unchanged. Re-parse changed files. Update graph edges. |
| 25 | Cross-file rules | `scope: "cross-file"` in config. Built-in: unused exports, circular deps, missing imports, dead modules, layer violations. |
| 26 | File watcher | `internal/project/watcher.go`: fsnotify integration. Cascade invalidation: file changed → re-lint dependents. Debounce rapid saves. |

**Month 7-8 — LSP + VS Code**

| Week | Task | Deliverable |
|---|---|---|
| 27 | LSP server core | `internal/lsp/server.go`: JSON-RPC over stdio. Initialize, shutdown, didOpen, didChange, didSave, didClose. |
| 28 | Push diagnostics | `textDocument/publishDiagnostics`: lint on open, re-lint on change (debounced), push results. Cross-file diagnostics from cache. |
| 29 | Code actions | `textDocument/codeAction`: quick fixes for auto-fixable rules. "Fix all" action. |
| 30 | VS Code extension | TypeScript extension: language client, status bar, config intellisense (JSON schema for `.lintrc.*`). |
| 31 | Workspace diagnostics | `workspace/diagnostic`: project-wide cross-file errors in Problems panel. Efficient pull-based diagnostics. |
| 32 | LSP extras | Hover (rule description on squiggle), go-to-definition (import → export via graph), find references (symbol → all importers). |

**v0.2 deliverable:** `yourlinter lsp`, VS Code extension. Project cache, module graph, cross-file rules. Watch mode with cascade invalidation. All v0.1 features plus project-awareness.

---

### Phase 3 — Formatter + Auto-Fix (v0.3) — Months 8-11

**Goal:** Code formatting via dprint WASM, auto-fix for lint rules, format-on-save in LSP.

**Month 8-9 — dprint WASM Integration**

| Week | Task | Deliverable |
|---|---|---|
| 33 | Wazero runtime setup | Initialize Wazero, load dprint TypeScript WASM plugin, implement dprint host protocol (format request/response). |
| 34 | Formatter engine | `internal/formatter/dprint_wasm.go`: format file via dprint, apply config (printWidth, indent, quotes, etc.). |
| 35 | Format CLI | `yourlinter format`, `yourlinter format --check`, `yourlinter format --diff`. File discovery, parallel formatting. |
| 36 | `yourlinter check` | Combine lint + format check in single command. Exit 1 if any issue. Optimized: single file read, lint then format check. |

**Month 9-10 — Auto-Fix**

| Week | Task | Deliverable |
|---|---|---|
| 37 | Fix infrastructure | `internal/engine/fix.go`: fix types (replacement, deletion, insertion). Range-based. Conflict resolution (priority, overlap detection). |
| 38 | Template fixes | `fix: { replace: "const $NAME = $VALUE" }` — substitution using captured variables from pattern/AST match. |
| 39 | Safe vs unsafe fixes | Categorize all built-in rule fixes. `--fix` (safe only), `--fix-unsafe` (all), `--fix-dry-run` (show diff). |
| 40 | Fix in LSP | Code actions return fixes. "Fix all auto-fixable" command. Format after fix. |

**Month 10-11 — Import Fixer + Polish**

| Week | Task | Deliverable |
|---|---|---|
| 41 | Import auto-sort | Re-order imports to match config groups. Remove unused imports. Add missing imports (from module graph). |
| 42 | Format-on-save LSP | `textDocument/formatting`, `textDocument/onTypeFormatting`. Configurable: format on save, format on paste. |
| 43 | Migration tools | `--from-biome`, `--from-prettier` config converters. Rule mapping tables. Migration report. |
| 44 | Release prep | Benchmark regression suite. dprint WASM bundled in binary (embed). Documentation. |

**v0.3 deliverable:** `yourlinter format`, auto-fix (`--fix`, `--fix-dry-run`), format-on-save in VS Code. Import sorting. Migration from Biome/Prettier.

---

### Phase 4 — WASM Plugins (v0.4) — Months 11-13

**Goal:** WASM plugin escape hatch for imperative custom rules.

**Month 11-12 — Plugin Runtime**

| Week | Task | Deliverable |
|---|---|---|
| 45 | Plugin SDK design | Define Go SDK: `plugin.Context` interface (AST, Source, ReadFile, Options, Imports, Exports). Serialization format for AST (compact, WASM-friendly). |
| 46 | Plugin host | `internal/engine/wasm_plugin.go`: load .wasm, instantiate via Wazero, pass context, receive diagnostics. Sandbox: no network, fs limited to project root. |
| 47 | Plugin lifecycle | Load once at startup, call per-file. Cache plugin instances. Timeout per plugin invocation (prevent infinite loops). |
| 48 | SDK for Go plugins | `github.com/yourorg/yourlinter-plugin-sdk` package. Users: `tinygo build -o rule.wasm -target=wasi`. Example plugin with tests. |

**Month 12-13 — SDK Expansion + Docs**

| Week | Task | Deliverable |
|---|---|---|
| 49 | SDK for Rust plugins | Rust crate for writing plugins. `cargo build --target wasm32-wasi`. Example: GraphQL + TS field matcher. |
| 50 | SDK for AssemblyScript | AS package for TS-like plugin authoring. Lowest barrier for JS/TS developers. |
| 51 | Plugin distribution | npm publishable `.wasm` + config schema. `plugin: "@myorg/graphql-lint"` resolves from node_modules. |
| 52 | Documentation | Plugin authoring guide. SDK API reference. Example plugins repo. |

**v0.4 deliverable:** WASM plugin system. SDKs for Go, Rust, AssemblyScript. Plugin npm distribution. Documentation.

---

### Phase 5 — Type-Aware + Production (v1.0) — Months 13-16

**Goal:** Type-aware rules via `typescript-go`, production hardening, v1.0 release.

**Month 13-14 — typescript-go Integration**

| Week | Task | Deliverable |
|---|---|---|
| 53-54 | typescript-go parser | Replace tree-sitter with `microsoft/typescript-go` parser for type-aware rules. Keep tree-sitter as fallback for error-tolerant parsing during edits. |
| 55-56 | Type-aware rules | Built-in rules: no-unsafe-any, no-explicit-any, strict-boolean-expressions, no-unnecessary-type-assertion, no-floating-promises. |

**Month 14-15 — Scope Analysis + Control Flow**

| Week | Task | Deliverable |
|---|---|---|
| 57-58 | Scope analysis | Variable scope resolution. Built-in: no-unused-vars, no-use-before-define, no-shadow. |
| 59-60 | Control flow graph | Basic CFG for unreachable code detection, no-fallthrough, consistent-return. |

**Month 15-16 — Production Hardening**

| Week | Task | Deliverable |
|---|---|---|
| 61 | Monorepo support | Workspace config, workspace-aware module graph, cross-workspace import rules. |
| 62 | Performance audit | Profile on real-world codebases (10K+ files). Optimize hot paths. Memory audit. |
| 63 | Stability | Edge case fixes. Error recovery hardening. Large file handling. Malicious config protection. |
| 64 | v1.0 release | Documentation site. Blog post. Product Hunt / HN launch. npm, homebrew, GitHub Releases, Docker, VS Code marketplace. |

**v1.0 deliverable:** Full linter + formatter with type-aware rules, cross-file analysis, WASM plugins, LSP, VS Code extension, monorepo support. Production-ready.

---

### Milestone Summary

| Milestone | Month | Key Deliverable | Rules |
|---|---|---|---|
| **v0.1** | 5 | Linter MVP — regex + AST patterns, CLI | 50 built-in |
| **v0.2** | 8 | Project-aware — cache, module graph, LSP, VS Code | 50 + cross-file |
| **v0.3** | 11 | Formatter — dprint WASM, auto-fix, import sorting | 70 + fixes |
| **v0.4** | 13 | WASM plugins — Go/Rust/AS SDKs, npm distribution | 70 + user WASM |
| **v1.0** | 16 | Type-aware — typescript-go, scope, CFG, production | 100+ |

### Critical Path

```
tree-sitter integration (M1)
  → AST pattern matching (M3) ← THIS IS THE RISKIEST COMPONENT
    → structural queries (M3)
      → built-in rules (M4)
        → CLI (M5)
          → v0.1 release

SQLite cache (M5)
  → module graph (M6)
    → cross-file rules (M6)
      → LSP server (M7)
        → v0.2 release

Wazero + dprint (M8)
  → formatter (M9)
    → auto-fix (M10)
      → v0.3 release
```

AST pattern matching is the highest-risk, highest-value component. It's the core differentiator and has no Go library to build on. Budget 4-6 weeks and expect iteration. Reference: ast-grep's Rust source for algorithm design.

---

## Dependencies

| Dependency | Purpose | Type |
|---|---|---|
| `github.com/BurntSushi/rure-go` | Fast regex via Rust engine | CGo (requires librure) |
| `github.com/smacker/go-tree-sitter` | JS/TS parser | CGo |
| `modernc.org/sqlite` | Cache storage | Pure Go |
| `github.com/fsnotify/fsnotify` | File watching | Pure Go |
| `github.com/dop251/goja` | Config file evaluation | Pure Go |
| `github.com/tetratelabs/wazero` | WASM runtime (dprint, future plugins) | Pure Go |
| `github.com/cespare/xxhash/v2` | Content hashing | Pure Go |

### Build Requirements

- Go 1.25+
- Rust toolchain (for building librure from `github.com/rust-lang/regex` capi)
- tree-sitter CLI (for grammar compilation)

---

## Risks & Mitigations

| Risk | Mitigation |
|---|---|
| CGo overhead (rure-go) | Batch calls via `FindAllBytes`/`IterBytes`, semaphore limits concurrent threads |
| tree-sitter Go bindings stability | Mature project, used by GitHub. Fallback: direct C bindings. |
| `typescript-go` API instability | Phase 4 dependency — tree-sitter covers Phases 1-3 |
| Formatter complexity | Start with dprint WASM, defer native printer |
| librure cross-compilation | Static link librure.a, or ship pre-built for targets |
| SQLite cache corruption | WAL mode, write-ahead logging, periodic integrity checks |
| Large monorepo performance | Incremental by design — only changed files re-analyzed |

---

## Config Resolution

### Strategy: Flat with Overrides (not ESLint cascade)

ESLint's cascading config (merging `.eslintrc` from every parent directory) is a known source of confusion and bugs. Biome uses a single config. This tool uses a **flat config with explicit overrides** — one config file at project root, with glob-scoped override blocks.

```js
// .lintrc.js (project root — single source of truth)
export default {
  // Base rules — apply to all files
  rules: {
    "no-var": "error",
    "no-console": "warn",
    "eqeqeq": "error"
  },

  // Overrides — scoped by glob
  overrides: [
    {
      files: ["**/*.test.*", "**/*.spec.*"],
      rules: {
        "no-console": "off",          // allow in tests
        "no-magic-numbers": "off"
      }
    },
    {
      files: ["src/legacy/**"],
      rules: {
        "no-var": "warn",             // downgrade for legacy code
        "eqeqeq": "off"
      }
    },
    {
      files: ["*.config.js", "*.config.ts"],
      rules: {
        "no-default-export": "off"    // configs need default exports
      }
    }
  ],

  // Ignore patterns (also supports .lintignore file)
  ignore: [
    "dist/**",
    "node_modules/**",
    "*.generated.*"
  ]
}
```

### Resolution Order

```
1. Base rules from config
2. Override blocks matched by glob (later blocks win)
3. Inline comments (highest priority)
```

### Inline Suppression Comments

```js
// Disable for next line
// lint-disable-next-line no-console
console.log("debug");

// Disable for current line
console.log("debug"); // lint-disable no-console

// Disable block
// lint-disable no-console, no-var
var x = 1;
console.log(x);
// lint-enable no-console, no-var

// Disable entire file (must be at top)
// lint-disable-file no-console
```

### Config Discovery

```
1. Look for .lintrc.{js,ts,mjs,json,yaml,yml,toml} in CWD
2. Walk up to find project root (nearest package.json or .git)
3. Check "lint" key in package.json as alternative
4. No config found → use built-in recommended preset
```

Priority order: `.lintrc.js` > `.lintrc.ts` > `.lintrc.json` > `.lintrc.yaml` > `.lintrc.toml` > `package.json#lint`

No merging across directories. One config per project. Monorepos use workspace-level overrides (see Monorepo Support section).

### Multi-Format Config Support

The config is declarative data — a tree of rules, patterns, and options. The engine consumes a single `*Config` struct regardless of source format. All formats are first-class.

| Format | Extension | Parser | Comments | Regex | Computed Values |
|---|---|---|---|---|---|
| JavaScript | `.js`, `.ts`, `.mjs` | goja (eval once) | Yes | `/regex/` literals | Yes (`path.resolve`, conditionals) |
| JSON(C) | `.json` | `json.Unmarshal` | JSONC yes | Strings: `"^on[A-Z]"` | No |
| YAML | `.yaml`, `.yml` | `yaml.Unmarshal` | Yes | Strings: `"^on[A-Z]"` | No |
| TOML | `.toml` | `toml.Unmarshal` | Yes | Strings: `"^on[A-Z]"` | No |

**Config loading internals:**

```go
func LoadConfig(path string) (*Config, error) {
    switch filepath.Ext(path) {
    case ".json":
        return loadJSON(path)
    case ".yaml", ".yml":
        return loadYAML(path)
    case ".toml":
        return loadTOML(path)
    case ".js", ".ts", ".mjs":
        return loadJS(path)   // goja — eval once, extract static data
    default:
        return nil, fmt.Errorf("unsupported config format: %s", path)
    }
}
// All return the same *Config struct. Engine doesn't care about source format.
```

**JSON example:**

```jsonc
// .lintrc.json
{
  "extends": ["@myorg/lint-rules-react"],
  "rules": {
    "no-var": "error",
    "no-console": ["warn", { "allow": ["warn", "error"] }],
    "react-boolean-prop-naming": {
      "ast": {
        "kind": "jsx_attribute",
        "name": { "type": "boolean" }
      },
      "naming": {
        "match": "^(is|has|should|can|will|did)",
        "message": "Boolean props must start with is/has/should/can/will/did"
      }
    }
  },
  "overrides": [
    {
      "files": ["**/*.test.*"],
      "rules": { "no-console": "off" }
    }
  ]
}
```

**YAML example:**

```yaml
# .lintrc.yaml
extends:
  - "@myorg/lint-rules-react"

rules:
  no-var: error
  no-console:
    - warn
    - allow: [warn, error]
  react-boolean-prop-naming:
    ast:
      kind: jsx_attribute
      name:
        type: boolean
    naming:
      match: "^(is|has|should|can|will|did)"
      message: "Boolean props must start with is/has/should/can/will/did"

overrides:
  - files: ["**/*.test.*"]
    rules:
      no-console: off
```

**TOML example:**

```toml
# .lintrc.toml
extends = ["@myorg/lint-rules-react"]

[rules]
no-var = "error"
no-console = "warn"

[rules.react-boolean-prop-naming.ast]
kind = "jsx_attribute"

[rules.react-boolean-prop-naming.ast.name]
type = "boolean"

[rules.react-boolean-prop-naming.naming]
match = "^(is|has|should|can|will|did)"
message = "Boolean props must start with is/has/should/can/will/did"

[[overrides]]
files = ["**/*.test.*"]

[overrides.rules]
no-console = "off"
```

**JS-only features** (not available in JSON/YAML/TOML):
- Computed values: `printWidth: process.stdout.columns || 100`
- Conditional config: `rules: { ...(isCI ? strictRules : devRules) }`
- Dynamic imports: `import config from "./shared-config.js"`

These cover <5% of use cases. The recommendation is YAML or JSON for most projects, JS only when dynamic config is needed.

---

## Auto-Fix Architecture

### Fix Types

| Type | Description | Example |
|---|---|---|
| **Replacement** | Replace matched range with new text | `var x` → `const x` |
| **Template** | Pattern-based substitution using captures | `$X == null` → `$X === null` |
| **Deletion** | Remove matched code | Remove `console.log(...)` |
| **Insertion** | Add code before/after match | Add missing `ErrorBoundary` wrapper |
| **Import fix** | Add/remove/reorder imports | Auto-sort, remove unused |

### Fix Definition in Config

```js
"no-var": {
  pattern: "var $NAME = $VALUE",
  fix: {
    // Simple replacement template
    replace: "const $NAME = $VALUE"
  }
},

"no-console-in-prod": {
  pattern: "console.log($$$ARGS)",
  fix: {
    // Delete the entire statement
    action: "delete-statement"
  }
},

"prefer-template": {
  pattern: "$A + $B + $C",
  where: {
    "$A": { kind: "string" },
    "$C": { kind: "string" }
  },
  fix: {
    // Template literal conversion
    replace: "`${$A.stripQuotes}${$B}${$C.stripQuotes}`"
  }
}
```

### Conflict Resolution

When multiple fixes target overlapping ranges:

```
1. Sort fixes by range start position (ascending)
2. If two fixes overlap:
   a. Higher-severity rule wins
   b. Same severity → smaller range (more specific) wins
   c. Same range → first rule in config order wins
3. Losing fix is reported as "unfixable conflict" in output
4. User can re-run to apply remaining fixes iteratively
```

### Safety

- **Dry-run mode** (`--fix-dry-run`): Show diffs without applying. Output unified diff format.
- **Fix categories**: `safe` (always correct, e.g., `==` → `===`) vs `unsafe` (might change semantics, e.g., `var` → `const` in loops). `--fix` applies safe only. `--fix-unsafe` applies both.
- **Atomic writes**: Fixes applied per-file atomically — either all fixes for a file succeed or none are written.
- **Formatter re-run**: After fixes are applied, formatter runs on changed files to ensure consistent style.

---

## Error Recovery

### Tree-sitter Error Tolerance

Tree-sitter produces partial ASTs for files with syntax errors. A file with a missing `}` still parses everything before and after the error. This is critical for LSP (users are always mid-edit).

### Behavior on Syntax Errors

```
File has syntax errors
  │
  ├─ Regex rules: RUN NORMALLY — regex doesn't care about syntax validity
  │
  ├─ AST pattern rules: RUN on valid subtrees, SKIP error nodes
  │     tree-sitter marks error regions with ERROR nodes
  │     patterns only match against non-error subtrees
  │
  ├─ Cross-file rules: USE LAST VALID CACHE
  │     if current parse has errors, don't update export/import maps
  │     stale data is better than wrong data
  │
  └─ Diagnostics: ADD syntax error diagnostic at error location
        "Syntax error: unexpected token '}' at line 42"
        (from tree-sitter's error node position)
```

### LSP Behavior During Edits

```
User is typing (incomplete code):
  1. tree-sitter incrementally re-parses (fast — reuses unchanged subtrees)
  2. Regex rules run on raw text (always valid)
  3. AST rules run on valid portions only
  4. Diagnostics pushed with both lint warnings + syntax errors
  5. Cross-file graph NOT updated until file is saved + parseable
```

This means: a user deleting a function mid-edit won't cause false "missing export" errors in other files. The graph only updates on save with a valid parse.

---

## CLI Design

### Commands

```bash
# Lint
yourlinter lint                     # lint all files (respects config)
yourlinter lint src/                # lint specific directory
yourlinter lint src/app.tsx         # lint specific file
yourlinter lint --fix               # apply safe auto-fixes
yourlinter lint --fix-unsafe        # apply all auto-fixes
yourlinter lint --fix-dry-run       # show fixes without applying

# Format
yourlinter format                   # format all files
yourlinter format src/              # format specific directory
yourlinter format --check           # check formatting (exit 1 if unformatted)
yourlinter format --diff            # show what would change

# Check (lint + format check — for CI)
yourlinter check                    # lint + format check, exit 1 on any issue

# Project
yourlinter init                     # create .lintrc.js with recommended preset
yourlinter init --from-eslint       # migrate from ESLint config
yourlinter init --from-biome        # migrate from Biome config
yourlinter cache clean              # clear project cache
yourlinter cache status             # show cache stats

# LSP
yourlinter lsp                      # start LSP server (stdio)

# Debug
yourlinter debug rules              # show compiled rules + match strategy
yourlinter debug parse <file>       # dump AST for a file
yourlinter debug graph              # show module graph summary
yourlinter debug graph <file>       # show imports/exports for a file
```

### Flags

```
Global:
  --config <path>         # explicit config path
  --no-config             # ignore config, use defaults
  --quiet                 # only errors, no warnings
  --max-warnings <n>      # exit 1 if warnings exceed N (for CI)

Output:
  --format stylish        # default: human-readable with colors
  --format json           # JSON array of diagnostics
  --format sarif           # SARIF for GitHub Code Scanning
  --format compact        # one-line-per-diagnostic
  --format github         # GitHub Actions annotation format

Performance:
  --threads <n>           # limit worker goroutines (default: CPU count)
  --no-cache              # disable project cache
```

### Exit Codes

```
0  — no errors (warnings are OK)
1  — lint errors found or format check failed
2  — config error / invalid arguments
3  — internal error (bug)
```

### CI Integration Examples

```yaml
# GitHub Actions
- run: yourlinter check --format github --max-warnings 0

# GitLab CI
- yourlinter lint --format json > gl-code-quality-report.json

# Pre-commit hook
- yourlinter lint --fix && yourlinter format
```

---

## VS Code Extension

### Architecture

```
┌─────────────┐     stdio      ┌──────────────────────┐
│  VS Code    │◄──────────────►│  yourlinter lsp      │
│  Extension  │   JSON-RPC     │  (same binary)       │
└──────┬──────┘                └──────────────────────┘
       │
       │  Extension provides:
       ├─ Language client (vscode-languageclient)
       ├─ Status bar item (diagnostics count, running state)
       ├─ Config file intellisense (.lintrc.js schema)
       └─ Commands palette integration
```

### Features

| Feature | LSP Method | Description |
|---|---|---|
| Inline diagnostics | `textDocument/publishDiagnostics` | Red/yellow squiggles with rule name |
| Quick fixes | `textDocument/codeAction` | "Fix: change `var` to `const`" |
| Fix all | `textDocument/codeAction` | "Fix all auto-fixable problems" |
| Format on save | `textDocument/formatting` | Triggers formatter |
| Hover info | `textDocument/hover` | Hover on squiggle → rule description + link |
| Go to definition | `textDocument/definition` | Jump from import to export (uses module graph) |
| Find references | `textDocument/references` | Find all importers of a symbol |
| Code lens | `textDocument/codeLens` | "5 usages" above exported functions |
| Config intellisense | JSON Schema | Autocomplete rule names, severity values |
| Workspace diagnostics | `workspace/diagnostic` | Cross-file errors shown in Problems panel |

### Config File Intellisense

The extension ships a JSON Schema for `.lintrc.js` that provides:
- Autocomplete for rule names (all built-in + custom rules from config)
- Severity value suggestions (`"error"`, `"warn"`, `"off"`)
- Validation of rule options
- Hover docs for each rule

---

## Migration Path

### From ESLint

```bash
yourlinter init --from-eslint
```

This reads `.eslintrc.*` / `eslint.config.*` and generates `.lintrc.js`:

1. **Rule mapping** — built-in lookup table of ESLint rule name → equivalent rule:
   ```
   "no-unused-vars" → "no-unused-vars" (built-in)
   "no-console" → "no-console" (built-in)
   "react/jsx-no-target-blank" → pattern-based equivalent
   ```

2. **Plugin rules** — for ESLint plugin rules without a direct equivalent:
   - Attempt to express as declarative pattern/AST query
   - If not possible, add as `// TODO: manual migration needed` comment

3. **Config translation**:
   ```
   ESLint "extends"     → resolved and flattened into rules
   ESLint "overrides"   → overrides blocks with file globs
   ESLint "env"         → global variable declarations
   ESLint "ignorePatterns" → ignore array
   ```

4. **Output**: migration report showing:
   - Rules migrated successfully
   - Rules with approximate equivalents (review recommended)
   - Rules with no equivalent (need manual handling or custom rule)

### From Biome

```bash
yourlinter init --from-biome
```

Reads `biome.json` and translates:
- Biome linter rules → equivalent rules
- Biome formatter config → formatter config
- Biome `files.ignore` → ignore patterns

### From Prettier

Prettier config (`.prettierrc`) is read for formatter settings:
```
printWidth, tabWidth, useTabs, semi, singleQuote,
trailingComma, bracketSpacing, arrowParens
```

Direct mapping — the formatter supports the same options.

---

## Testing Strategy

### Rule Testing (Fixture-Based)

Each rule has a test fixture file with annotated expected diagnostics:

```js
// tests/rules/no-var/valid.js
const x = 1;      // OK
let y = 2;         // OK

// tests/rules/no-var/invalid.js
var x = 1;         // expect-error: no-var
var y = 2;         // expect-error: no-var
const z = 3;       // OK
```

Test runner:
```bash
yourlinter test                           # run all rule tests
yourlinter test rules/no-var              # run specific rule test
yourlinter test --update-snapshots        # update expected outputs
```

### Implementation

```go
// Rule test reads fixture, runs linter, compares diagnostics
// against "expect-error" comments in the fixture file.

func TestRule(t *testing.T) {
    fixtures := glob("tests/rules/*/")
    for _, dir := range fixtures {
        valid := runLinter(dir + "/valid.js")
        assert(len(valid.Diagnostics) == 0)

        invalid := runLinter(dir + "/invalid.js")
        expected := parseExpectComments(dir + "/invalid.js")
        assertDiagnosticsMatch(invalid.Diagnostics, expected)
    }
}
```

### Custom Rule Testing

Users can test their own declarative rules the same way:

```bash
# In user's project
yourlinter test --rule "no-magic-timeouts" --fixture tests/lint/
```

Fixture format is the same: `// expect-error: rule-name` comments.

### Formatter Testing (Snapshot-Based)

```
tests/format/
  cases/
    arrow-functions/
      input.ts           # unformatted
      output.ts          # expected formatted output
    jsx-multiline/
      input.tsx
      output.tsx
```

Test runner formats `input.*`, diffs against `output.*`. `--update-snapshots` overwrites `output.*` files.

### Integration Tests

- **CLI tests**: Run the binary as a subprocess, check exit codes and stdout/stderr.
- **LSP tests**: Mock editor sends JSON-RPC messages, assert responses.
- **Cache tests**: Modify files, run incrementally, verify only changed files re-analyzed.
- **Cross-file tests**: Multi-file fixtures with expected cross-file diagnostics.

### CI Pipeline

```yaml
test:
  - go test ./...                              # unit + rule fixtures
  - yourlinter check --format github .          # dogfood on own codebase
  - ./scripts/benchmark.sh                     # regression guard on perf
```

---

## Monorepo Support

### Workspace Config

```js
// .lintrc.js (monorepo root)
export default {
  workspaces: [
    "apps/*",
    "packages/*"
  ],

  // Shared rules — apply to all workspaces
  rules: {
    "no-var": "error",
    "eqeqeq": "error"
  },

  // Per-workspace overrides
  overrides: [
    {
      workspace: "apps/web",
      rules: {
        "no-document-write": "error"       // web-specific
      }
    },
    {
      workspace: "packages/shared",
      rules: {
        "no-platform-specific": "error"    // must stay platform-agnostic
      }
    }
  ]
}
```

### Workspace-Aware Module Graph

```
monorepo/
  apps/web/         → can import from packages/*
  apps/mobile/      → can import from packages/*
  packages/shared/  → cannot import from apps/*
  packages/ui/      → cannot import from apps/* or other packages/*
```

The module graph respects workspace boundaries. Cross-workspace imports are resolved via `package.json` names (not relative paths). This enables rules like:

```js
"no-cross-workspace-relative-imports": {
  scope: "cross-file",
  ast: { kind: "import_declaration" },
  where: {
    importCrossesWorkspace: true,
    source: { match: /^\./ }      // relative path crossing workspace
  },
  message: "Use package name for cross-workspace imports"
}
```

### Shared Rule Packages

Teams can publish reusable rule configs as npm packages:

```js
// .lintrc.js
import reactRules from "@myorg/lint-rules-react";
import securityRules from "@myorg/lint-rules-security";

export default {
  extends: [reactRules, securityRules],
  rules: {
    // local overrides
  }
}
```

`extends` is resolved at config load time (goja evaluates the JS), flattened into a single rule set. No runtime cost.

---

## Performance Budget

### Hard Limits

| Metric | Limit | Enforcement |
|---|---|---|
| First diagnostic in LSP | <100ms from keystroke | Benchmark in CI |
| Cold scan (10K files) | <15s | Benchmark in CI |
| Warm start | <500ms | Benchmark in CI |
| Single file re-lint | <5ms | Benchmark in CI |
| Memory per file (cached) | <50KB avg | Monitor in tests |
| Binary size | <30MB | Build CI check |
| Config parse time | <100ms | Test on large configs |

### Enforcement

```bash
# scripts/benchmark.sh — runs in CI, fails if limits exceeded
yourlinter bench --cold-scan ./fixtures/10k-files/ --max-time 15s
yourlinter bench --single-file ./fixtures/large-file.ts --max-time 5ms
yourlinter bench --lsp-diagnostic ./fixtures/edit-scenario/ --max-time 100ms
```

Results tracked over time. Regressions block merge.

### Optimization Priorities

```
1. File I/O (parallel reads, mmap for large files)
2. Regex matching (rure-go, batch per-rule)
3. AST traversal (single pass, multiple rules per visitor)
4. Cache hits (avoid redundant work)
5. Memory (pool allocators for diagnostics, reuse tree-sitter parsers)
```

---

## Distribution

### Build Targets

| Platform | Arch | librure | tree-sitter | Notes |
|---|---|---|---|---|
| macOS | arm64 | static .a | static .a | Primary dev target |
| macOS | x86_64 | static .a | static .a | Intel Macs |
| Linux | x86_64 | static .a | static .a | CI / servers |
| Linux | arm64 | static .a | static .a | ARM servers |
| Windows | x86_64 | static .lib | static .lib | MinGW or MSVC |

### librure Bundling

The Rust regex C library (`librure`) is statically linked into the Go binary. Build process:

```bash
# Build librure for target
cd regex-capi && cargo build --release --target <target>

# Link into Go binary
CGO_ENABLED=1 \
CGO_LDFLAGS="-L<path>/librure.a -lrure" \
go build -o yourlinter -ldflags="-s -w" ./cmd/linter/
```

Pre-built librure archives for all targets are checked into the repo or built in CI via cross-compilation.

### Distribution Channels

| Channel | Method |
|---|---|
| **npm** | `npx yourlinter` — npm package wrapping platform-specific binary (like Biome, esbuild) |
| **Homebrew** | `brew install yourlinter` |
| **GitHub Releases** | Pre-built binaries per platform |
| **Docker** | `ghcr.io/org/yourlinter:latest` |
| **Go install** | `go install github.com/org/yourlinter@latest` (requires CGo toolchain) |
| **VS Code Marketplace** | Extension bundles LSP binary for each platform |

### npm Wrapper Pattern (like esbuild)

```
@yourlinter/cli                    # main package, postinstall picks platform binary
@yourlinter/darwin-arm64           # macOS ARM binary
@yourlinter/darwin-x64             # macOS Intel binary
@yourlinter/linux-x64              # Linux x64 binary
@yourlinter/linux-arm64            # Linux ARM binary
@yourlinter/win32-x64              # Windows binary
```

`package.json` uses `optionalDependencies` — npm installs only the matching platform package.

---

## Ignore Patterns

### .lintignore File

```gitignore
# .lintignore (same syntax as .gitignore)
dist/
build/
node_modules/
*.generated.ts
*.min.js
coverage/
__snapshots__/
```

### Config-Level Ignores

```js
export default {
  ignore: [
    "dist/**",
    "**/*.generated.*",
    "vendor/**"
  ]
}
```

### Inline Suppression

```js
// Disable specific rule for next line
// lint-disable-next-line no-console
console.log("debug");

// Disable for current line
doSomething(); // lint-disable no-eval

// Disable block
/* lint-disable no-var, no-console */
var debug = true;
console.log(debug);
/* lint-enable no-var, no-console */

// Disable entire file (must be first comment)
/* lint-disable-file */

// Disable specific rule for entire file
/* lint-disable-file no-console */
```

### Generated File Detection

Auto-detect generated files (skip linting):
- Files starting with `// @generated` or `/* @generated */`
- Files matching common generated patterns: `*.generated.*`, `*.g.ts`, `*.pb.ts`
- Files in directories: `generated/`, `__generated__/`, `.next/`, `.nuxt/`
- Configurable via `generatedPatterns` in config

### Suppression Tracking

Track suppression usage to identify stale disables:

```bash
yourlinter lint --report-unused-disables
```

Reports `// lint-disable` comments that suppress no actual diagnostics — likely leftover from fixed code.

---

## Comparison with ast-grep

ast-grep is the closest existing tool to the declarative rule approach. Key differences:

| | ast-grep | This Tool |
|---|---|---|
| Language | Rust CLI | Go (with rure-go for regex) |
| Parser | tree-sitter | tree-sitter (Phase 1), typescript-go (Phase 4) |
| Pattern syntax | `console.log($$$)` | Same — adopted ast-grep's pattern syntax |
| Regex rules | No — AST patterns only | Yes — rure-go, first-class |
| Cross-file | No | First-class, incremental module graph |
| Project cache | No (re-parses every run) | SQLite cache, incremental |
| Watch mode | No | Built-in with cascade invalidation |
| LSP | Limited (experimental) | Full LSP with project-wide diagnostics |
| Formatter | No | Built-in (dprint WASM → native printer) |
| Config | YAML rule files | JS/JSON config with extends, overrides |
| Auto-fix | Pattern replacement | Pattern replacement + structural fixes |
| Naming rules | No | Built-in (capture + naming constraints) |
| Import rules | No | Built-in (ordering, grouping, cross-file) |
| CI integration | Basic | SARIF, GitHub annotations, JSON output |
| Custom rule testing | No framework | Fixture-based testing with expect-error comments |
| Migration | N/A | From ESLint, Biome, Prettier configs |

**In short:** ast-grep is a search/refactoring tool. This is a complete linter+formatter platform that happens to use the same pattern syntax for one of its rule types.

---

## Go Code Style & Conventions

### Authoritative Sources (Priority Order)

1. [Effective Go](https://go.dev/doc/effective_go) — official, maintained by the Go team
2. [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) — official wiki
3. [Google Go Style Guide](https://google.github.io/styleguide/go/) — detailed decisions & best practices
4. [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md) — production-focused, strong on concurrency and performance

### Naming Rules

- `MixedCaps` / `mixedCaps` only. Never underscores in Go names.
- Exception: test functions like `TestScanRule_EmptyInput`.
- Package names: lowercase, single word, no underscores or hyphens. `engine`, `rules`, `lsp` — not `rule_engine` or `lint-rules`.
- Receiver names: 1-2 letter abbreviation of the type. `func (r Rule) Match(...)`, not `func (rule Rule)` or `func (self Rule)`.
- Acronyms are all-caps: `HTTP`, `URL`, `ID`, `LSP`, `AST`, `CST` — not `Http`, `Url`, `Id`.
- Getters drop `Get` prefix: `rule.Name()`, not `rule.GetName()`.
- Interfaces with one method are named by the method + `er` suffix: `Matcher`, `Scanner`, `Formatter`.

### Error Handling

```go
// WRONG — panics on recoverable error
func compileOpt(pattern string) *rure.Regex {
    re, err := rure.CompileOptions(pattern, 0, opts)
    if err != nil {
        panic(fmt.Sprintf("failed to compile %q: %v", pattern, err))
    }
    return re
}

// CORRECT — returns error
func compileOpt(pattern string) (*rure.Regex, error) {
    re, err := rure.CompileOptions(pattern, 0, opts)
    if err != nil {
        return nil, fmt.Errorf("compile rule %q: %w", pattern, err)
    }
    return re, nil
}

// ACCEPTABLE — Must* prefix for package-level init that truly cannot recover
var reNoVar = MustCompile(`\bvar\s+\w+`)
```

Rules:
- Always check errors. Never `_` an error without a documented reason.
- Wrap with context: `fmt.Errorf("compile rule %q: %w", name, err)` — `%w` preserves error chain.
- Use `errors.Is()` and `errors.As()` for comparison — never `==` or type assertions.
- Define sentinel errors: `var ErrRuleCompile = errors.New("rule compile failed")`.
- **Never panic for recoverable errors.** `Must*` prefix is acceptable only for init-time.

### Interface Rules

- Accept interfaces, return structs.
- Define interfaces at the **consumer**, not the implementer. If `engine` needs a regex matcher, define `type Matcher interface { ... }` in `engine/`, not in `rules/`.
- Keep interfaces small (1-3 methods). `io.Reader` (1 method) is the gold standard.

### Concurrency Rules

- `wg.Add(1)` must be called **before** `go func()`, never inside the goroutine.
- Channel direction in function signatures: use `chan<-` (send-only) and `<-chan` (receive-only).
- Semaphore pattern (`sem <- struct{}{}` / `<-sem`) is idiomatic Go — use it.
- Prefer `sync.WaitGroup` + channels over `sync.Mutex` for goroutine coordination.
- Never share `map` across goroutines without synchronization. Use `sync.Map` or mutex-protected access.

---

## Repository Structure

### Project Layout

```
yourlinter/
├── cmd/
│   └── yourlinter/
│       └── main.go              # Thin: parse flags, wire deps, call engine.Run()
│                                # Target: ~30 lines
├── internal/
│   ├── engine/
│   │   ├── engine.go            # Rule execution orchestrator
│   │   ├── engine_test.go       # Table-driven tests
│   │   ├── regex.go             # rure-go pattern rules
│   │   ├── ast_pattern.go       # AST pattern matching
│   │   ├── structural.go        # Structural AST queries
│   │   ├── naming.go            # Naming convention checker
│   │   ├── imports.go           # Import ordering / grouping
│   │   ├── complexity.go        # Cyclomatic complexity
│   │   └── crossfile.go         # Cross-file rule evaluation
│   │
│   ├── parser/
│   │   ├── treesitter.go        # tree-sitter wrapper
│   │   ├── treesitter_test.go
│   │   └── queries.go           # Pre-built queries for JS/TS/JSX/TSX
│   │
│   ├── formatter/
│   │   ├── printer.go           # CST → formatted output
│   │   └── dprint_wasm.go       # dprint WASM plugin bridge
│   │
│   ├── project/
│   │   ├── graph.go             # Module graph + dependency tracking
│   │   ├── graph_test.go
│   │   ├── cache.go             # SQLite cache layer
│   │   ├── watcher.go           # fsnotify + invalidation logic
│   │   ├── scanner.go           # Initial project scan
│   │   └── hasher.go            # xxhash content hashing
│   │
│   ├── lsp/
│   │   ├── server.go            # JSON-RPC handler
│   │   ├── diagnostics.go       # Push diagnostics to editor
│   │   ├── formatting.go        # On-save / on-type formatting
│   │   └── codelens.go          # Inline code actions
│   │
│   ├── config/
│   │   ├── loader.go            # Parse .lintrc.{js,json,yaml,toml}
│   │   ├── loader_test.go
│   │   ├── compiler.go          # Compile declarative rules → engine repr
│   │   ├── schema.go            # Config validation
│   │   └── defaults.go          # Built-in recommended ruleset
│   │
│   └── cli/
│       ├── lint.go              # `yourlinter lint` command
│       ├── format.go            # `yourlinter format` command
│       ├── check.go             # `yourlinter check` command
│       ├── init.go              # `yourlinter init` command
│       └── debug.go             # `yourlinter debug` commands
│
├── testdata/                    # Test fixtures
│   ├── rules/
│   │   ├── no-var/
│   │   │   ├── valid.js
│   │   │   └── invalid.js
│   │   └── no-console/
│   │       ├── valid.js
│   │       └── invalid.js
│   └── format/
│       ├── arrow-functions/
│       │   ├── input.ts
│       │   └── output.ts
│       └── jsx-multiline/
│           ├── input.tsx
│           └── output.tsx
│
├── .golangci.yml                # Linter config
├── .goreleaser.yml              # Release config
├── Makefile                     # Build, test, lint, fmt targets
├── go.mod
├── go.sum
├── LICENSE
└── README.md
```

**Key rules:**
- **`cmd/`** — One subdirectory per binary. `main.go` must be thin (~30 lines): parse flags, read config, delegate to `internal/`.
- **`internal/`** — Go compiler enforces nothing outside this module can import `internal/` packages. Use for all non-reusable code.
- **No `pkg/` directory.** `pkg/` is effectively deprecated for single-binary projects. The Go team never endorsed it. Use `internal/` for everything.
- **`testdata/`** — Go tooling ignores this directory during builds. Standard location for test fixtures.

### Module Path

```go
// go.mod
module github.com/yourorg/yourlinter

go 1.25

require (
    github.com/BurntSushi/rure-go v0.0.0
    github.com/smacker/go-tree-sitter v0.0.0
    modernc.org/sqlite v1.0.0
    github.com/fsnotify/fsnotify v1.7.0
    github.com/dop251/goja v0.0.0
    github.com/tetratelabs/wazero v1.0.0
    github.com/cespare/xxhash/v2 v2.0.0
)
```

- Module path must match repo path.
- Pin Go version to minimum supported: `go 1.25`.
- Run `go mod tidy` before every commit. CI must verify no diff.

---

## Makefile

```makefile
.PHONY: build test test-race lint fmt clean bench install

BINARY     := yourlinter
MODULE     := github.com/yourorg/yourlinter
VERSION    := $(shell git describe --tags --always --dirty)
LDFLAGS    := -s -w -X main.version=$(VERSION)
CGO_ENABLED := 1

# librure paths (adjust per platform)
RURE_DIR   := ./vendor/librure
CGO_LDFLAGS := -L$(RURE_DIR) -lrure

## Build
build:
	CGO_ENABLED=$(CGO_ENABLED) CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/yourlinter

## Install locally
install:
	CGO_ENABLED=$(CGO_ENABLED) CGO_LDFLAGS="$(CGO_LDFLAGS)" \
		go install -ldflags="$(LDFLAGS)" ./cmd/yourlinter

## Test
test:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

## Benchmark
bench:
	go test -bench=. -benchmem ./internal/engine/

## Lint
lint:
	golangci-lint run ./...

## Format
fmt:
	gofumpt -w .

## Verify (CI: format check + mod tidy check)
verify: lint
	gofumpt -d . | (! grep .)  # fail if any file needs formatting
	go mod tidy
	git diff --exit-code go.mod go.sum  # fail if tidy changed anything

## Clean
clean:
	rm -f $(BINARY)

## Build librure from source (one-time setup)
librure:
	cd vendor/regex-src/regex-capi && cargo build --release
	cp vendor/regex-src/target/release/librure.a $(RURE_DIR)/
```

---

## Linting & Formatting Tools

### golangci-lint v2 Config

```yaml
# .golangci.yml
version: "2"

linters:
  default: standard
  enable:
    # Formatting
    - gofumpt          # Strict superset of gofmt (grouped imports, trailing commas)

    # Bugs & correctness
    - gocritic         # Subtle issues (appendAssign, captLocal, etc.)
    - errcheck         # All errors must be checked
    - errorlint        # Enforce errors.Is/As instead of == / type assertion
    - bodyclose        # HTTP response body must be closed
    - noctx            # HTTP requests must include context
    - gosec            # Security scanner

    # Performance
    - prealloc         # Suggest slice preallocation
    - unconvert        # Remove unnecessary type conversions

    # Maintainability
    - unparam          # Detect unused function parameters
    - misspell         # Catch typos in comments/strings
    - revive           # Extensible linter, superset of golint

linters-settings:
  gofumpt:
    extra-rules: true
  gocritic:
    enabled-tags:
      - diagnostic
      - style
      - performance
  revive:
    rules:
      - name: exported
        arguments:
          - "checkPrivateReceivers"
      - name: unused-parameter

formatters:
  enable:
    - gofumpt
```

### Why gofumpt over gofmt

`gofumpt` is a strict superset of `gofmt` maintained by Daniel Marti (Go team contributor). Additional rules:
- Grouped imports (stdlib, external, internal — separated by blank lines)
- No empty lines at start/end of blocks
- Trailing commas in multi-line composite literals
- Consistent function signature formatting

`gofmt` is the minimum. `gofumpt` is the standard for new projects.

---

## Testing Conventions

### Table-Driven Tests (Mandatory)

```go
func TestOffsetToLine(t *testing.T) {
    lineStarts := []int{0, 10, 25, 40}

    tests := []struct {
        name     string
        offset   int
        wantLine int
        wantCol  int
    }{
        {"first char", 0, 1, 0},
        {"mid first line", 5, 1, 5},
        {"start second line", 10, 2, 0},
        {"end of input", 42, 4, 2},
        {"exact line start", 25, 3, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            line, col := offsetToLine(lineStarts, tt.offset)
            if line != tt.wantLine || col != tt.wantCol {
                t.Errorf("offsetToLine(%d) = (%d, %d), want (%d, %d)",
                    tt.offset, line, col, tt.wantLine, tt.wantCol)
            }
        })
    }
}
```

### Rules

- **Table-driven tests for all pure functions.** No exceptions.
- **`t.Run()` for subtests** — always. Enables `go test -run TestScanRule/empty_input`.
- **`t.Parallel()`** — add to independent subtests. But **not** when tests share CGo resources (rure-go, tree-sitter).
- **Test naming:** `TestFunctionName_Scenario` — e.g., `TestScanRuleIter_MaxMatchesCap`, `TestBuildLineIndex_EmptyFile`.
- **Test file location:** `foo_test.go` next to `foo.go`, same directory.
- **Package:** Use `package engine` (same package) for unit tests. Use `package engine_test` (external package) only for integration tests that test the public API.
- **No testify.** Standard library `testing` is sufficient for this project. Zero dependencies in tests.
- **`testdata/` directory** for fixtures. Go tooling ignores it during builds.

### Benchmarks in Test Files

```go
// internal/engine/engine_test.go

func BenchmarkAnalyzeGuarded(b *testing.B) {
    data := loadFixture(b, "testdata/large.js")
    rules, _ := buildRules()
    lineStarts := buildLineIndex(data)

    b.ResetTimer()
    b.ReportAllocs()

    for i := 0; i < b.N; i++ {
        analyzeGuardedCount(data, lineStarts, rules)
    }
}

func BenchmarkScanRuleIter(b *testing.B) {
    data := loadFixture(b, "testdata/large.js")
    rule, _ := compileRule("no-var", `\bvar\s+\w+`)
    lineStarts := buildLineIndex(data)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        scanRuleIter(rule, data, lineStarts, 10000)
    }
}
```

Run with: `go test -bench=. -benchmem ./internal/engine/`

### CI Test Flags

```bash
go test -race -count=1 -coverprofile=coverage.out ./...
```

- `-race` — mandatory for concurrent code (semaphore pattern, goroutines).
- `-count=1` — disables test caching, catches flaky tests.
- `-coverprofile` — track coverage, block merge if below threshold.

---

## CGo Best Practices

### Build Flags

```makefile
# Development (dynamic linking, fast builds)
CGO_ENABLED=1 go build ./cmd/yourlinter

# Production Linux (static binary)
CGO_ENABLED=1 CC=musl-gcc \
  go build -tags "netgo osusergo" \
  -ldflags="-linkmode external -extldflags '-static' -s -w" \
  -o yourlinter ./cmd/yourlinter

# macOS (dynamic link to libSystem is always required)
CGO_ENABLED=1 go build -ldflags="-s -w" -o yourlinter ./cmd/yourlinter
```

Build tags `netgo` and `osusergo` avoid glibc dependencies in `net` and `os/user` packages.

### Cross-Compilation

CGo is **disabled by default** when cross-compiling. Each target needs:

1. `CGO_ENABLED=1` explicitly set
2. A C cross-compiler: `CC=x86_64-linux-musl-gcc` (Linux amd64), `CC=aarch64-linux-musl-gcc` (Linux arm64)
3. Pre-compiled librure static archive (`.a`) for each target
4. `CGO_LDFLAGS` pointing to the correct archive

```bash
# Example: build for Linux amd64 from macOS
CGO_ENABLED=1 \
GOOS=linux GOARCH=amd64 \
CC=x86_64-linux-musl-gcc \
CGO_LDFLAGS="-L./vendor/librure/linux-amd64 -lrure" \
go build -o yourlinter-linux-amd64 ./cmd/yourlinter
```

### CGo Performance Rules

- **Minimize boundary crossings.** Each Go→C call has ~100ns overhead. Use batch APIs (`FindAllBytes`, `IterBytes`) instead of per-line calls.
- **Semaphore-limit concurrent CGo calls.** CGo pins goroutines to OS threads. Unbounded goroutines calling CGo = unbounded OS threads = OOM. Limit to `runtime.NumCPU()`.
- **Never pass Go pointers to C that will be retained** after the C call returns (Go pointer passing rules).
- **`#cgo` directives in Go files**, not separate C files:
  ```go
  // #cgo LDFLAGS: -lrure
  // #cgo CFLAGS: -I${SRCDIR}/vendor/include
  import "C"
  ```

### Docker Build (Recommended for CI)

```dockerfile
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache gcc musl-dev rust cargo

# Build librure
COPY vendor/regex-src /tmp/regex-src
RUN cd /tmp/regex-src/regex-capi && cargo build --release

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 \
    CGO_LDFLAGS="-L/tmp/regex-src/target/release -lrure" \
    go build -tags "netgo osusergo" \
    -ldflags="-linkmode external -extldflags '-static' -s -w" \
    -o /yourlinter ./cmd/yourlinter

FROM scratch
COPY --from=builder /yourlinter /yourlinter
ENTRYPOINT ["/yourlinter"]
```

---

## Release & Distribution (GoReleaser)

### .goreleaser.yml

```yaml
version: 2

builds:
  - id: yourlinter
    main: ./cmd/yourlinter
    binary: yourlinter
    env:
      - CGO_ENABLED=1
    goos:
      - linux
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.Commit}}
    overrides:
      - goos: linux
        goarch: amd64
        env:
          - CC=x86_64-linux-musl-gcc
          - CGO_LDFLAGS=-L./vendor/librure/linux-amd64 -lrure
        flags:
          - -tags=netgo,osusergo
        ldflags:
          - -linkmode external
          - -extldflags '-static'
      - goos: linux
        goarch: arm64
        env:
          - CC=aarch64-linux-musl-gcc
          - CGO_LDFLAGS=-L./vendor/librure/linux-arm64 -lrure
        flags:
          - -tags=netgo,osusergo
        ldflags:
          - -linkmode external
          - -extldflags '-static'
      - goos: darwin
        goarch: arm64
        env:
          - CGO_LDFLAGS=-L./vendor/librure/darwin-arm64 -lrure
      - goos: darwin
        goarch: amd64
        env:
          - CGO_LDFLAGS=-L./vendor/librure/darwin-amd64 -lrure

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"

checksum:
  name_template: checksums.txt

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
      - "^ci:"
```

### GitHub Actions Release Workflow

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: docker://ghcr.io/goreleaser/goreleaser-cross:v1.25
        with:
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### CI Workflow (Lint + Test)

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - uses: golangci/golangci-lint-action@v6
        with:
          version: v2

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Install librure
        run: |
          cd vendor/regex-src/regex-capi && cargo build --release
      - name: Test with race detector
        run: |
          CGO_LDFLAGS="-L./vendor/regex-src/target/release -lrure" \
          go test -race -count=1 -coverprofile=coverage.out ./...
      - name: Check coverage
        run: |
          go tool cover -func=coverage.out | grep total | awk '{print $3}'

  verify:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Check formatting
        run: |
          go install mvdan.cc/gofumpt@latest
          gofumpt -d . | (! grep .)
      - name: Check go.mod tidy
        run: |
          go mod tidy
          git diff --exit-code go.mod go.sum

  bench:
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Run benchmarks
        run: |
          CGO_LDFLAGS="-L./vendor/regex-src/target/release -lrure" \
          go test -bench=. -benchmem -count=5 ./internal/engine/ | tee bench.txt
      - name: Compare with main
        uses: bencherdev/bencher@main  # or similar bench comparison tool
```

---

## Why Go (Not Rust)

1. **Proven faster for this workload** — goroutine-per-rule + rure-go = 3.3x faster than Rust rayon on 30-rule benchmark
2. **Parallelism is trivial** — goroutines vs Rust's async complexity or rayon DFA cache contention
3. **Plugin/WASM ecosystem** — Wazero is mature, pure Go, well-documented
4. **LSP precedent** — gopls proves Go is ideal for language servers
5. **Lower contributor barrier** — Go is simpler, larger potential contributor base
6. **Fast compilation** — quick iteration during development
7. **Single binary distribution** — static linking (except librure dylib, which can be embedded)
