# internal/parser — Tree-Sitter Integration

Tree-sitter wrapper providing parsed syntax trees for JS/TS source files. This is the foundation for all AST-based lint rules, structural queries, and pattern matching.

## Why a Wrapper

The package wraps `github.com/tree-sitter/go-tree-sitter` (official C bindings) behind Go-native types. This serves two purposes:

1. **Isolation** — The rest of the codebase never touches tree-sitter types directly. When `microsoft/typescript-go` stabilizes (TypeScript 7), migration means changing this package only.
2. **Resource safety** — Every C object (`Parser`, `Tree`, `Query`, `TreeCursor`) must be explicitly freed. The wrapper enforces `Close()` contracts and uses `t.Cleanup()` in tests.

## Architecture

```
                  LangFromPath("app.tsx")
                         │
                         ▼
              ┌──────────────────┐
              │   Lang (uint8)   │  Extension → grammar mapping
              │  JS/JSX/TS/TSX   │
              └────────┬─────────┘
                       │ .Language()
                       ▼
              ┌──────────────────┐
              │     Parser       │  Wraps *tree_sitter.Parser
              │  NewParser(lang) │  One parser per language
              └────────┬─────────┘
                       │ .Parse(ctx, source, oldTree)
                       ▼
              ┌──────────────────┐
              │      Tree        │  Wraps *tree_sitter.Tree
              │  .RootNode()     │  Entry point for traversal
              │  .Edit()         │  For incremental reparse
              └────────┬─────────┘
                       │
            ┌──────────┼──────────┐
            ▼          ▼          ▼
         Walk()    WalkNamed()  Query.Exec()
      All nodes   Named only   S-expression match
```

## Files

| File | Responsibility |
|---|---|
| `lang.go` | `Lang` type, `LangFromPath`, extension→grammar mapping |
| `parser.go` | `Parser` struct, `NewParser`, `Parse` with context cancellation + incremental reparse |
| `tree.go` | `Tree`, `Node`, `Point` value types wrapping tree-sitter internals |
| `walk.go` | `Walk`, `WalkNamed` — depth-first traversal via `TreeCursor` (zero allocations) |
| `query.go` | `Query`, `Match`, `Capture` — compiled S-expression pattern execution |

## Language Detection

`LangFromPath` maps file extensions to `Lang` values:

| Extensions | Lang |
|---|---|
| `.js`, `.mjs`, `.cjs` | `LangJS` |
| `.jsx` | `LangJSX` |
| `.ts`, `.mts`, `.cts` | `LangTS` |
| `.tsx` | `LangTSX` |

JS and JSX share the JavaScript grammar (JSX is part of it). TS and TSX use separate grammars from the `tree-sitter-typescript` package (`LanguageTypescript()` and `LanguageTSX()`).

## Parsing

`Parser.Parse` accepts a `context.Context` for cancellation. Cancellation is cooperative — tree-sitter checks a progress callback periodically during parsing. On cancellation, any partial tree is freed and `ctx.Err()` is returned.

For incremental reparsing (watch mode / LSP), call `Tree.Edit()` to describe the edit, then pass the old tree to `Parse`. Tree-sitter reuses unchanged subtrees.

## Tree Walking

Two traversal functions, both using `TreeCursor` internally (no per-node heap allocations):

- **`Walk`** — visits all nodes including anonymous ones (keywords, punctuation)
- **`WalkNamed`** — skips anonymous nodes, visits only named grammar nodes

`WalkFunc` returns `false` to skip a node's children (prune subtree).

## Queries

`Query` compiles tree-sitter S-expression patterns and executes them against a node. Used as the foundation for AST pattern matching rules (month 3).

S-expression patterns use tree-sitter's query syntax. Captures are named with `@name`:

```
(function_declaration name: (identifier) @fn-name) @fn
```

`Query.Exec` returns `[]Match`, each containing `[]Capture` with the capture name and matched `Node`.

## Resource Management

Every type wrapping a C resource has a `Close()` method. In production code, use `defer`. In tests, use `t.Cleanup()`:

```go
p := parser.NewParser(parser.LangTS)
t.Cleanup(p.Close)

tree, err := p.Parse(ctx, source, nil)
t.Cleanup(tree.Close)

q, err := parser.NewQuery(parser.LangTS, pattern)
t.Cleanup(q.Close)
```

## Dependencies

- `github.com/tree-sitter/go-tree-sitter` v0.25.0 — official Go bindings (CGo)
- `github.com/tree-sitter/tree-sitter-javascript` v0.25.0 — JS/JSX grammar
- `github.com/tree-sitter/tree-sitter-typescript` v0.23.2 — TS and TSX grammars (separate functions)

## CGo

This package requires `CGO_ENABLED=1`. The tree-sitter grammars include large C parser files (~1-5MB each). First build is slow; subsequent builds use the Go build cache.
