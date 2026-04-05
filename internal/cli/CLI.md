# CLI Package

Cobra-based command-line interface for ralf.

## Architecture

```
cmd/ralf/main.go
  └─ cli.Execute() → int (exit code)
       └─ newRootCmd()
            ├─ --config (global flag)
            ├─ --version
            ├─ lint [paths...]
            │    ├─ --format (stylish|json|compact|github|sarif)
            │    ├─ --threads (int)
            │    ├─ --max-warnings (int)
            │    ├─ --fix
            │    ├─ --fix-dry-run
            │    ├─ --no-cache
            │    └─ --watch
            └─ init
                 ├─ --from-eslint
                 ├─ --from-biome
                 ├─ --force
                 └─ --format (json|yaml|toml)
```

`Execute` returns an int exit code — `main.go` calls `os.Exit(cli.Execute())`.

## Exit Codes

| Code | Meaning |
|---|---|
| 0 | No lint errors (or only warnings within limit) |
| 1 | Lint errors found, or warnings exceeded `--max-warnings` |
| 2 | Config/usage error (missing config, invalid format, bad regex) |
| 3 | Internal error |

## File Discovery

`discoverFiles(paths, ignorePatterns)`:

1. Explicit file paths: accepted if they have a supported extension (`parser.LangFromPath`)
2. Directories: recursive `filepath.WalkDir`, filter by extension
3. Hardcoded skips: `.git`, `node_modules`, `dist`, `build`, `.next`, `coverage`
4. Ignore patterns: matched via `doublestar.Match` (supports `**`)
5. Returns absolute paths, sorted, deduplicated

## Output Formatters

All formatters display columns as 1-based (engine stores 0-based).

| Format | Use Case |
|---|---|
| **stylish** (default) | Human-readable, ESLint-style grouped by file with summary |
| **json** | Machine-readable array of diagnostic objects |
| **compact** | One line per diagnostic, grep-friendly |
| **github** | GitHub Actions `::error`/`::warning` workflow commands |
| **sarif** | SARIF v2.1.0 for GitHub Code Scanning and CI tools |

## Config Loading

1. If `--config` flag is set, load that specific file
2. Otherwise, search cwd for `.ralfrc.json` → `.ralfrc.yaml` → `.ralfrc.yml` → `.ralfrc.toml`
3. Validate config via `config.Validate`
4. Create engine via `engine.New` (compiles regex rules)

## Example Usage

```bash
# Lint cwd with auto-discovered config
ralf lint

# Lint specific paths
ralf lint src/ tests/

# Explicit config
ralf lint --config .ralfrc.json src/

# JSON output for CI
ralf lint --format json src/

# GitHub Actions annotations
ralf lint --format github src/

# Fail on any warnings
ralf lint --max-warnings 0 src/

# Watch for changes and re-lint
ralf lint --watch

# Generate default config
ralf init

# Migrate from ESLint
ralf init --from-eslint

# Migrate from Biome (YAML output)
ralf init --from-biome --format yaml

# Overwrite existing config
ralf init --force
```

## Init Command

`ralf init` generates a `.ralfrc` config file with all 61 built-in rules.

| Flag | Description |
|---|---|
| `--from-eslint` | Migrate from `.eslintrc.json`/`.yaml`/`.yml` (JS configs unsupported — use `npx eslint --print-config . > .eslintrc.json`) |
| `--from-biome` | Migrate from `biome.json`/`biome.jsonc` |
| `--force` | Overwrite existing config |
| `--format` | Output format: `json` (default), `yaml`, `toml` |

Migration starts from all 61 built-in rules, overriding severities from the source config. Unmapped rules are listed in a migration report on stderr.

## Files

| File | Responsibility |
|---|---|
| `root.go` | Root cobra command, global flags, `Execute` entry point |
| `discover.go` | File discovery: walk, filter, ignore |
| `lint.go` | Lint subcommand: load config → engine → discover → lint → format |
| `init.go` | Init subcommand: generate config, migration dispatch, serialization |
| `migrate_eslint.go` | ESLint config parser, rule mapping table, severity conversion |
| `migrate_biome.go` | Biome config parser, rule mapping table, JSONC stripping |
| `format.go` | Output formatters: stylish, JSON, compact, GitHub, SARIF |
