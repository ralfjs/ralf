# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/),
and this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0](https://github.com/ralfjs/ralf/compare/v0.1.0...v0.1.0) (2026-03-24)


### Bug Fixes

* **release:** add Release environment to npm-publish job ([#39](https://github.com/ralfjs/ralf/issues/39)) ([9e4581b](https://github.com/ralfjs/ralf/commit/9e4581b5f7ed4b6279af6b68c52c55b091225ac8))


### Build

* **deps:** bump actions/labeler from 5 to 6 ([#46](https://github.com/ralfjs/ralf/issues/46)) ([49cba4e](https://github.com/ralfjs/ralf/commit/49cba4e7f399eaf4f95778cd46f57a3e5706604d))
* **deps:** bump actions/setup-go from 5 to 6 ([#45](https://github.com/ralfjs/ralf/issues/45)) ([221ef21](https://github.com/ralfjs/ralf/commit/221ef21f69a0799237b2eaa58b077f2b9aea7eb6))
* **deps:** bump actions/upload-artifact from 4 to 7 ([#44](https://github.com/ralfjs/ralf/issues/44)) ([1c1ba3f](https://github.com/ralfjs/ralf/commit/1c1ba3f7e8bf6a99cab8fac97c85031d3c02972f))
* **deps:** bump amannn/action-semantic-pull-request from 5 to 6 ([#43](https://github.com/ralfjs/ralf/issues/43)) ([7bee09c](https://github.com/ralfjs/ralf/commit/7bee09c528734ca91d22add9efe94a74df3d1319))

## 0.1.0 (2026-03-24)


### Features

* add branching strategy, CI workflow, and validation rules ([ccfefe1](https://github.com/ralfjs/ralf/commit/ccfefe188b9ffa6b11efd1bf14d38e598556eb22))
* **ci:** add release-please for auto versioning and changelog ([2e65a70](https://github.com/ralfjs/ralf/commit/2e65a70b2e3e296d5868fe027f2835a97a4c0e0f))
* **cli:** add ralf init with ESLint/Biome migration ([#26](https://github.com/ralfjs/ralf/issues/26)) ([11d4a70](https://github.com/ralfjs/ralf/commit/11d4a70ef5b81cdf0f55beaada276d1cd93ec485))
* **cli:** add SARIF v2.1.0 output format ([#23](https://github.com/ralfjs/ralf/issues/23)) ([0d337c0](https://github.com/ralfjs/ralf/commit/0d337c03ec5a31a04aa42eea5d04372e2cf60228))
* **config:** add JS config loader and extends resolution ([#22](https://github.com/ralfjs/ralf/issues/22)) ([b415220](https://github.com/ralfjs/ralf/commit/b4152201f92028f62ba83fd46b9c200c48ce94ce))
* **config:** config loader with JSON, YAML, TOML support ([#2](https://github.com/ralfjs/ralf/issues/2)) ([3084872](https://github.com/ralfjs/ralf/commit/3084872ee52d26da6ce2c2c3fd20d7430b924fb7))
* **engine,cli:** lint engine core + CLI lint command + rename to ralf ([#6](https://github.com/ralfjs/ralf/issues/6)) ([1ffc429](https://github.com/ralfjs/ralf/commit/1ffc429d083604e9f3cca199e6a38d6b6aedfe70))
* **engine:** add 29 built-in rules with custom Go checker system ([#19](https://github.com/ralfjs/ralf/issues/19)) ([f8dbd1f](https://github.com/ralfjs/ralf/commit/f8dbd1f7daed33aa655640cce39e9d49ae4910b5))
* **engine:** add auto-fix engine with --fix and --fix-dry-run CLI flags ([#13](https://github.com/ralfjs/ralf/issues/13)) ([129ad8f](https://github.com/ralfjs/ralf/commit/129ad8fd7f0873e8e982c2da7966dbbaedee528d))
* **engine:** add inline suppression comments ([#20](https://github.com/ralfjs/ralf/issues/20)) ([0dfeae2](https://github.com/ralfjs/ralf/commit/0dfeae27fa8dcd218cf211fc09b76996ededfba2))
* **engine:** AST pattern matching with tree-sitter integration ([#9](https://github.com/ralfjs/ralf/issues/9)) ([a2d972d](https://github.com/ralfjs/ralf/commit/a2d972d331a835a4ad4ae1df9225bae9032bb7c9))
* **engine:** import ordering engine with group, alphabetize, and newline checks ([#18](https://github.com/ralfjs/ralf/issues/18)) ([cba9515](https://github.com/ralfjs/ralf/commit/cba9515b3986dc7590de55efa099c1a07afb405c))
* **engine:** naming convention engine with AST integration ([#16](https://github.com/ralfjs/ralf/issues/16)) ([5bf31c6](https://github.com/ralfjs/ralf/commit/5bf31c619f02ebed4fcdb8fa033786a025a9cf3b))
* **engine:** rure-go integration + performance optimizations ([#7](https://github.com/ralfjs/ralf/issues/7)) ([8fcee16](https://github.com/ralfjs/ralf/commit/8fcee1612b2411a51ad52056d39631754e797fd3))
* **engine:** structural AST queries with tree-sitter integration ([#14](https://github.com/ralfjs/ralf/issues/14)) ([fc112a2](https://github.com/ralfjs/ralf/commit/fc112a25c17e3d1cdf8cdf0170a46ccc3c2d0dff))
* **parser:** tree-sitter integration for JS/TS parsing ([#1](https://github.com/ralfjs/ralf/issues/1)) ([d043e0e](https://github.com/ralfjs/ralf/commit/d043e0e2b21ca751be092190d1b98afe83e45a99))
* **rules:** add 12 ESLint/Biome-equivalent builtin rules ([#25](https://github.com/ralfjs/ralf/issues/25)) ([ae2a4db](https://github.com/ralfjs/ralf/commit/ae2a4db2c13aa69d32af4abbc2ef9516764b51d2))
* **rules:** add 20 built-in regex rules with zero-config fallback ([#11](https://github.com/ralfjs/ralf/issues/11)) ([d7f3b64](https://github.com/ralfjs/ralf/commit/d7f3b645001277996427973f271c94ed9500e7b2))


### Bug Fixes

* address CLAUDE.md review findings ([28aa92c](https://github.com/ralfjs/ralf/commit/28aa92c3346ca5e57d6f76afb59c089e644ee51f))
* **npm:** add files fields, pack verification, release docs ([#29](https://github.com/ralfjs/ralf/issues/29)) ([a6af23f](https://github.com/ralfjs/ralf/commit/a6af23f7c2dbcfb6572fdab7a53787ca24f4f5b4))


### Performance

* **engine:** SIMD-accelerated buildLineIndex via bytes.IndexByte ([#8](https://github.com/ralfjs/ralf/issues/8)) ([593a118](https://github.com/ralfjs/ralf/commit/593a118a0759729598892a6cebf47a848f92c71f))


### Documentation

* add conventional commits rules with scopes and examples ([b810615](https://github.com/ralfjs/ralf/commit/b810615dd9a66c9f3680e3c94375d278565ee563))
* update ARCHITECTURE.md and CLAUDE.md for structural AST queries ([#15](https://github.com/ralfjs/ralf/issues/15)) ([ad7eb6b](https://github.com/ralfjs/ralf/commit/ad7eb6b1c37e61fc269cc84d42b87c1c14d4bb9f)), closes [#14](https://github.com/ralfjs/ralf/issues/14)
* update README and ARCHITECTURE.md status markers ([#17](https://github.com/ralfjs/ralf/issues/17)) ([b637e81](https://github.com/ralfjs/ralf/commit/b637e8181b898a6d4a08c9214299a90aae005bc3))


### Build

* v0.1.0 release preparation ([#27](https://github.com/ralfjs/ralf/issues/27)) ([79d0130](https://github.com/ralfjs/ralf/commit/79d0130199bb839727360bbded30c0899e95729c))


### Miscellaneous

* add golangci-lint config, changelog, contributing guide, and license ([1f5d5c9](https://github.com/ralfjs/ralf/commit/1f5d5c948c07c802845a14288214f8c5328c48c9))
* **config:** remove dead struct fields and fix doc markers ([#21](https://github.com/ralfjs/ralf/issues/21)) ([e9820a1](https://github.com/ralfjs/ralf/commit/e9820a102b7bb0edbc8ed480c69b37a6eb3d2990))
* **release:** set release-as 0.1.0 ([8c1930d](https://github.com/ralfjs/ralf/commit/8c1930da47c2f912729a34ad3f820a4825df56d4))
* **release:** set release-as 0.1.0 for initial release ([2bcf54a](https://github.com/ralfjs/ralf/commit/2bcf54ac767c894d225ee9f1a85aa57e74055757))
* **release:** v0.1.0 ([5926528](https://github.com/ralfjs/ralf/commit/5926528ec8108c0a1f3fe8cab7ea31e9a0665acb))

## [Unreleased]

No changes yet.

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
- **Inline suppression:** `lint-disable-next-line`, `lint-disable`/`lint-enable` blocks, `lint-disable-file`, same-line disable
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
