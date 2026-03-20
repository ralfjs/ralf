# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] — Linter MVP

First public release. Fast, project-aware JS/TS linter with 61 built-in rules.

### Highlights

- **3.3x faster than Rust parallel** — Go + rure-go regex engine with semaphore-limited CGo workers
- **61 built-in rules** — ESLint recommended + Biome stable equivalents
- **Zero config** — works out of the box with sensible defaults
- **ESLint/Biome migration** — `ralf init --from-eslint` and `--from-biome`

### Features

- **Rule engine:** regex (rure-go), AST pattern matching (`$VAR`/`$$$`), structural queries (`ast: { kind, name, parent, not }`), naming conventions, import ordering, 33 custom Go builtin checkers with kind-indexed single-walk dispatch
- **CLI:** `ralf lint` with `--fix`/`--fix-dry-run`, `ralf init` with `--from-eslint`/`--from-biome`/`--format json|yaml|toml`
- **Output formats:** stylish (default), JSON, compact, GitHub Actions annotations, SARIF v2.1.0 (with `partialFingerprints` for GitHub Code Scanning)
- **Config:** `.ralfrc.json`, `.ralfrc.yaml`, `.ralfrc.yml`, `.ralfrc.toml`, `.ralfrc.js` (via goja), `extends`, glob-scoped `overrides`
- **Auto-fix:** template fixes with capture substitution, conflict resolution, atomic file writes
- **Inline suppression:** `ralf-disable-next-line`, `ralf-disable`/`ralf-enable` blocks, `ralf-disable-file`, same-line disable
- **Parser:** tree-sitter with incremental reparsing, JS/TS/JSX/TSX support

### Rules (61)

**Error prevention (23):** no-dupe-keys, no-dupe-args, no-dupe-class-members, no-duplicate-case, no-self-assign, no-self-compare, valid-typeof, use-isnan, for-direction, getter-return, no-setter-return, no-unsafe-finally, no-unsafe-negation, no-unsafe-optional-chaining, no-constant-condition, no-loss-of-precision, no-fallthrough, no-inner-declarations, no-constructor-return, no-empty-character-class, no-sparse-arrays, no-cond-assign, no-compare-neg-zero

**Best practices (22):** eqeqeq, no-var, no-eval, no-implied-eval, no-new-func, no-caller, no-void, no-with, no-labels, no-extend-native, no-proto, no-iterator, no-new-wrappers, no-return-await, no-case-declarations, no-delete-var, no-octal, no-octal-escape, no-nonoctal-decimal-escape, no-multi-str, no-script-url, no-inner-html

**Code quality (13):** no-empty, no-empty-pattern, no-empty-static-block, no-useless-catch, no-extra-boolean-cast, no-shadow-restricted-names, no-prototype-builtins, require-yield, no-async-promise-executor, no-new-native-nonconstructor, no-obj-calls, no-regex-spaces, no-control-regex

**Style (3):** no-console, no-debugger, no-alert

### Performance

- 61 rules on 50 files x 600 lines: ~197ms (~3.9ms/file)
- 33 builtin checkers via single-walk dispatch: 6.7ms
- SARIF formatter: 395k ns/op for 1,000 diagnostics
