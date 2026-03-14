# CLI Package

Cobra-based command-line interface for bepro.

## Architecture

```
cmd/bepro/main.go
  └─ cli.Execute() → int (exit code)
       └─ newRootCmd()
            ├─ --config (global flag)
            ├─ --version
            └─ lint [paths...]
                 ├─ --format (stylish|json|compact|github)
                 ├─ --threads (int)
                 └─ --max-warnings (int)
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

## Config Loading

1. If `--config` flag is set, load that specific file
2. Otherwise, search cwd for `.lintrc.json` → `.lintrc.yaml` → `.lintrc.yml` → `.lintrc.toml`
3. Validate config via `config.Validate`
4. Create engine via `engine.New` (compiles regex rules)

## Example Usage

```bash
# Lint cwd with auto-discovered config
bepro lint

# Lint specific paths
bepro lint src/ tests/

# Explicit config
bepro lint --config .lintrc.json src/

# JSON output for CI
bepro lint --format json src/

# GitHub Actions annotations
bepro lint --format github src/

# Fail on any warnings
bepro lint --max-warnings 0 src/
```

## Files

| File | Responsibility |
|---|---|
| `root.go` | Root cobra command, global flags, `Execute` entry point |
| `discover.go` | File discovery: walk, filter, ignore |
| `lint.go` | Lint subcommand: load config → engine → discover → lint → format |
| `format.go` | Output formatters: stylish, JSON, compact, GitHub |
