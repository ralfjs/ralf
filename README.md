# RALF

**R**eliable **A**dvanced **L**inter and **F**ormatter

[![CI](https://github.com/Hideart/ralf/actions/workflows/ci.yml/badge.svg)](https://github.com/Hideart/ralf/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/ralf-lint)](https://www.npmjs.com/package/ralf-lint)
[![Go Report Card](https://goreportcard.com/badge/github.com/Hideart/ralf)](https://goreportcard.com/report/github.com/Hideart/ralf)

Fast, project-aware JS/TS linter with 61 built-in rules. ESLint/Biome compatible. Zero config required.

Written in Go. Regex engine powered by Rust's `regex` crate via [rure-go](https://github.com/BurntSushi/rure-go). AST parsing via [tree-sitter](https://tree-sitter.github.io/tree-sitter/).

---

## Table of Contents

- [Why RALF](#why-ralf)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Rules](#rules)
- [Configuration](#configuration)
- [Output Formats](#output-formats)
- [Roadmap](#roadmap)
- [Documentation](#documentation)
- [License](#license)

---

## Why RALF

| | ESLint | Biome | RALF |
|---|---|---|---|
| Language | JS | Rust | Go |
| Speed | Slow | Fast | Fast (Go + Rust regex via CGo) |
| Custom rules | JS visitors (slow) | None yet | Declarative (native speed) |
| Config migration | N/A | N/A | `--from-eslint`, `--from-biome` |
| Output formats | Stylish, JSON | JSON | Stylish, JSON, SARIF, GitHub Actions, compact |
| Auto-fix | Yes | Yes | Yes (`--fix` / `--fix-dry-run`) |

## Installation

**npm** (recommended):
```bash
npm install -D ralf-lint
npx ralf lint
```

**Binary download** (macOS, Linux):
```bash
# Download from GitHub Releases
curl -fsSL https://github.com/Hideart/ralf/releases/latest/download/ralf_$(uname -s | tr A-Z a-z)_$(uname -m).tar.gz | tar xz
sudo mv ralf /usr/local/bin/
```

**Go** (requires CGo + Rust toolchain for librure):
```bash
go install github.com/Hideart/ralf/cmd/ralf@latest
```

## Quick Start

```bash
# Generate config with all 61 rules
ralf init

# Lint your project
ralf lint

# Migrate from ESLint
ralf init --from-eslint

# Migrate from Biome
ralf init --from-biome

# Auto-fix
ralf lint --fix

# SARIF output for GitHub Code Scanning
ralf lint --format sarif > results.sarif
```

## Rules

61 built-in rules covering ESLint recommended and Biome stable equivalents:

**Error prevention:** `no-dupe-keys`, `no-dupe-args`, `no-dupe-class-members`, `no-duplicate-case`, `no-self-assign`, `no-self-compare`, `valid-typeof`, `use-isnan`, `for-direction`, `getter-return`, `no-setter-return`, `no-unsafe-finally`, `no-unsafe-negation`, `no-unsafe-optional-chaining`, `no-constant-condition`, `no-loss-of-precision`, `no-fallthrough`, `no-inner-declarations`, `no-constructor-return`, `no-empty-character-class`, `no-sparse-arrays`, `no-cond-assign`, `no-compare-neg-zero`

**Best practices:** `eqeqeq`, `no-var`, `no-eval`, `no-implied-eval`, `no-new-func`, `no-caller`, `no-void`, `no-with`, `no-labels`, `no-extend-native`, `no-proto`, `no-iterator`, `no-new-wrappers`, `no-return-await`, `no-case-declarations`, `no-delete-var`, `no-octal`, `no-octal-escape`, `no-nonoctal-decimal-escape`, `no-multi-str`, `no-script-url`, `no-inner-html`

**Code quality:** `no-empty`, `no-empty-pattern`, `no-empty-static-block`, `no-useless-catch`, `no-extra-boolean-cast`, `no-shadow-restricted-names`, `no-prototype-builtins`, `require-yield`, `no-async-promise-executor`, `no-new-native-nonconstructor`, `no-obj-calls`, `no-regex-spaces`, `no-control-regex`

**Style:** `no-console`, `no-debugger`, `no-alert`

Full rule gap analysis vs ESLint/Biome: [#24](https://github.com/Hideart/ralf/issues/24)

## Configuration

Zero config works out of the box — all 61 rules enabled with sensible defaults.

To customize, run `ralf init` and edit the generated `.ralfrc.json`. Supports JSON, YAML, TOML, and JS config formats, with `extends`, glob-scoped `overrides`, and inline suppression comments.

See the **[Configuration Guide](docs/CONFIGURATION.md)** for full syntax reference: rule types (regex, AST pattern, structural query, builtin), auto-fix, where predicates, naming conventions, import ordering, and migration from ESLint/Biome.

## Output Formats

| Format | Flag | Use Case |
|---|---|---|
| Stylish | `--format stylish` (default) | Human-readable, grouped by file |
| JSON | `--format json` | Machine-readable |
| SARIF | `--format sarif` | GitHub Code Scanning |
| GitHub | `--format github` | GitHub Actions annotations |
| Compact | `--format compact` | Grep-friendly, one line per diagnostic |

## Roadmap

| Milestone | Key Deliverable |
|---|---|
| **v0.1** (current) | Linter MVP — 61 rules, CLI, config, SARIF, migration |
| **v0.2** | Project-aware — SQLite cache, module graph, LSP, VS Code |
| **v0.3** | Formatter — dprint WASM, import sorting |
| **v0.4** | WASM plugins — Go/Rust/AS SDKs |
| **v1.0** | Type-aware rules via typescript-go, scope analysis, CFG |

## Documentation

- [Configuration Guide](docs/CONFIGURATION.md) — full config syntax reference
- [Architecture & Design](docs/ARCHITECTURE.md) — technical spec, benchmarks, implementation plan
- [Branching & Releases](docs/BRANCHING.md) — Git Flow, versioning
- [Development Status](docs/DEVELOPMENT_STATUS.md) — detailed feature matrix
- [Contributing](CONTRIBUTING.md) — dev setup, code style, testing

## License

[MIT](LICENSE)
