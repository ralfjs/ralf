# Contributing to RALF

## Prerequisites

- **Go 1.25+** — [install](https://go.dev/dl/)
- **Rust toolchain** — [install](https://rustup.rs/) (for building librure)
- **golangci-lint v2** — [install](https://golangci-lint.run/welcome/install/)
- **gofumpt** — `go install mvdan.cc/gofumpt@latest`

## Development Setup

### 1. Clone the repo

```bash
git clone git@github.com:Hideart/ralf.git
cd ralf
git checkout develop
```

### 2. Build librure (Rust regex C library)

RALF uses [rure-go](https://github.com/BurntSushi/rure-go) which requires the Rust regex C API library.

```bash
# Clone the Rust regex repo
git clone https://github.com/rust-lang/regex.git vendor/regex-src

# Build the C library
cd vendor/regex-src/regex-capi
cargo build --release

# Copy the static library
mkdir -p ../../librure
cp ../target/release/librure.a ../../librure/
cd ../../..
```

### 3. Verify the build

```bash
make build
make test
```

If `make build` fails with linker errors, ensure `CGO_LDFLAGS` points to the correct librure path. The Makefile expects it at `./vendor/librure/librure.a`.

### 4. Install dev tools

```bash
# Linter
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

# Formatter
go install mvdan.cc/gofumpt@latest
```

## Development Workflow

### Start a feature

```bash
git checkout develop
git pull origin develop
git checkout -b feat/BP-42-my-feature
```

### Make changes

1. Write code in `internal/` packages
2. Write tests alongside (table-driven, `*_test.go`)
3. If adding a lint rule, create fixture in `testdata/rules/<rule-name>/`

### Validate

Run the full validation sequence before pushing:

```bash
make build       # must compile
make test-race   # must pass with zero races
make lint        # must pass with zero warnings
make fmt         # then: git diff — must be clean
```

### Commit

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```bash
git commit -m "feat(engine): add AST pattern parser"
```

See [CLAUDE.md — Commit Messages](CLAUDE.md#commit-messages-conventional-commits) for the full spec.

### Submit PR

```bash
git push -u origin feat/BP-42-my-feature
gh pr create --base develop
```

PRs target `develop`, not `main`. See [docs/BRANCHING.md](docs/BRANCHING.md) for the full workflow.

## Project Structure

```
cmd/ralf/           # CLI entry point (keep thin)
internal/
  engine/            # Rule execution core
  parser/            # tree-sitter wrapper
  formatter/         # Code formatting
  project/           # Cache, module graph, watcher
  lsp/               # LSP server
  config/            # Config loading
  cli/               # CLI commands
  plugin/            # WASM plugin host
testdata/            # Test fixtures
docs/                # Architecture, branching docs
```

See [CLAUDE.md — Architecture](CLAUDE.md#architecture) for dependency direction rules.

## Code Style

- **gofumpt** for formatting (not gofmt)
- **Table-driven tests** for all pure functions
- **No testify** — stdlib `testing` only
- **`log/slog`** for structured logging, `fmt.Println` for CLI output only
- **`context.Context`** as first param for I/O and CGo functions
- **Early return on error** — happy path left-aligned

Full rules: [CLAUDE.md — Code Style](CLAUDE.md#code-style)

## Testing

### Run tests

```bash
make test          # fast, no race detector
make test-race     # with race detector (CI uses this)
make coverage      # with coverage report
make bench         # benchmarks for engine
```

### Benchmarking

The engine has two benchmarks in `internal/engine/regex_bench_test.go`:

- `BenchmarkMatchRegex` — single rule, 30K lines, no I/O (isolates regex engine)
- `BenchmarkLintE2E` — 100 files × 300 lines × 5 rules, full pipeline (disk I/O, concurrency, sorting)

```bash
make bench                                          # quick run
go test -bench=. -benchmem -count=5 ./internal/engine/  # stable comparison
```

For performance work, always use `-count=5` or more. Single runs have high variance. Profile before optimizing:

```bash
go test -bench=BenchmarkLintE2E -cpuprofile=cpu.prof ./internal/engine/
go tool pprof -top cpu.prof
```

See [ENGINE.md — Benchmarking](internal/engine/ENGINE.md#benchmarking) for detailed guidance on interpreting results.

### Adding a lint rule

1. Add rule definition in `internal/engine/`
2. Create `testdata/rules/<rule-name>/valid.js` — code that should NOT trigger the rule
3. Create `testdata/rules/<rule-name>/invalid.js` — code that SHOULD trigger, with annotations:

```js
var x = 1;         // expect-error: no-var
var y = 2;         // expect-error: no-var
const z = 3;       // OK
```

4. Run `make test` to verify

### Adding a formatter test

1. Create `testdata/format/<case-name>/input.ts` — unformatted code
2. Create `testdata/format/<case-name>/output.ts` — expected formatted output
3. Run `make test` to verify

## CGo Notes

This project has two CGo dependencies: **rure-go** (Rust regex) and **go-tree-sitter**.

- `CGO_ENABLED=1` is required for all build/test commands
- The Makefile handles this automatically
- Cross-compilation requires a C cross-compiler and pre-built librure for each target
- See [docs/ARCHITECTURE.md — CGo Best Practices](docs/ARCHITECTURE.md) for details

## Release Process

Releases are automated via release-please + GitHub Actions:

1. Merge feature PRs to `develop`
2. Create `release/vX.Y.Z` from `develop`, PR to `main`
3. Merge to `main` — release-please creates a release PR (bumps version in `internal/version/version.go`)
4. Merge release-please PR — tag created, matrix build runs, GitHub Release published
5. npm packages published automatically (requires `NPM_TOKEN` secret)
6. Back-merge `main` into `develop`

### Required Secrets

| Secret | Where | Purpose |
|---|---|---|
| `GITHUB_TOKEN` | Automatic | GitHub Release upload, release-please |
| `NPM_TOKEN` | Repo Settings > Secrets > Actions | npm publish (`ralf-lint` + platform packages) |
| `CODECOV_TOKEN` | Repo Settings > Secrets > Actions | Coverage upload |

**Setting up NPM_TOKEN:**

1. Go to [npmjs.com](https://www.npmjs.com/) > Account > Access Tokens
2. Generate New Token > type: **Automation**
3. Copy the token
4. GitHub repo > Settings > Secrets and variables > Actions > New repository secret
5. Name: `NPM_TOKEN`, Value: paste the token

## Questions?

Open an issue or check existing docs:
- [Architecture & Design](docs/ARCHITECTURE.md)
- [Branching & Releases](docs/BRANCHING.md)
- [Claude Instructions](CLAUDE.md)
