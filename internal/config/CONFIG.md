# internal/config — Configuration Loader

Loads, validates, and resolves linter configuration from `.ralfrc.{json,yaml,yml,toml}` files.

## Architecture

```
  .ralfrc.yaml (or .json / .toml)
         │
         ▼
  ┌──────────────┐
  │  Load(dir)   │  Discover config file in priority order
  │  LoadFile()  │  Parse by extension (JSON/YAML/TOML)
  └──────┬───────┘
         │ *Config
         ▼
  ┌──────────────┐
  │  Validate()  │  Structural checks: severity, matchers, globs
  └──────┬───────┘
         │ error / nil
         ▼
  ┌──────────────┐
  │  Merge()     │  Apply file-scoped overrides for a given path
  └──────┬───────┘
         │ map[string]RuleConfig
         ▼
      Engine consumes effective rules per file
```

## Files

| File | Responsibility |
|---|---|
| `config.go` | Data types: `Config`, `RuleConfig`, `Severity`, `Override`, matcher stubs (`ASTMatcher`, `ImportsMatcher`, `NamingMatcher`, `WherePredicate`) |
| `loader.go` | `Load` (directory search) and `LoadFile` (explicit path). Dispatches to `encoding/json`, `yaml.v3`, or `BurntSushi/toml` by extension |
| `validate.go` | `Validate` — checks each rule (including override rules) has exactly one matcher, valid severity, non-empty globs |
| `merge.go` | `Merge` — applies matching override globs on top of base rules for a given file path |
| `defaults.go` | `DefaultConfig` — returns empty config with initialized `Rules` map |

## Config File Discovery

`Load(dir)` searches for config files in this priority order:

1. `.ralfrc.json`
2. `.ralfrc.yaml`
3. `.ralfrc.yml`
4. `.ralfrc.toml`

First match wins. Non-existence errors are skipped; other `os.Stat` errors (permission denied, etc.) are surfaced immediately.

## Config Structure

```yaml
rules:
  rule-name:
    severity: error | warn | off    # required
    message: "..."                   # optional diagnostic message
    # Exactly one matcher (validation enforces this):
    regex: "pattern"                 # regex matcher
    pattern: "console.log($$$)"     # AST pattern matcher
    ast: { kind: "..." }            # structural AST matcher
    imports: { groups: [...] }      # import ordering matcher
    naming: { match: "^..." }       # naming convention matcher
    # Optional:
    where: { file: "src/**" }       # file/context predicate
    scope: "cross-file"             # analysis scope
    fix: "replacement"              # auto-fix template

ignores:
  - "node_modules/**"
  - "dist/**"

overrides:
  - files: ["*.test.js"]
    rules:
      rule-name:
        severity: "off"
        regex: "placeholder"        # matcher currently required (see known limitations)
```

## Validation

`Validate` checks both top-level rules and override rules:

- **Severity** — required, must be `"error"`, `"warn"`, or `"off"` (missing severity is a validation error)
- **Matcher** — exactly one of `regex`, `pattern`, `ast`, `imports`, `naming` must be set
- **Override globs** — `files` array must be non-empty, no empty/whitespace strings, no malformed glob syntax

Returns `*ValidationError` containing `[]FieldError` with rule name, field, and message. Override rule errors use paths like `overrides[0].rules.rule-name`.

## Override Merging

`Merge(cfg, filePath)` returns the effective rule set for a specific file:

1. Start with all base rules from `cfg.Rules`
2. For each override whose `files` globs match `filePath`, apply its rules on top
3. Later overrides win over earlier ones
4. Overrides can add new rules not present in the base config

Glob matching uses `filepath.Match` (single-level wildcards). Matches against both the full path and the basename.

## Known Limitations

These are tracked as GitHub issues for future sprints:

- **No `**` globstar** (#4) — `filepath.Match` only supports `*` (single level). Override patterns like `**/*.test.*` won't match. Will switch to `doublestar` when the engine integrates config.
- **No field-level override merge** (#3) — overrides replace the entire `RuleConfig`. An override that only changes severity must also restate the matcher. Will add field-level merging when the engine consumes overrides.
- **No JSONC support** (#5) — JSON config files don't support comments. Use YAML if comments are needed. JSONC support will come with `.ralfrc.js` loader (week 17).
- **No `.ralfrc.js` support** — JS config via `goja` is planned for week 17.
- **No `extends` resolution** — the `Extends` field is deserialized but not resolved. Will be implemented with the config compiler.

## Dependencies

- `gopkg.in/yaml.v3` — YAML parsing
- `github.com/BurntSushi/toml` — TOML parsing
- `encoding/json` (stdlib) — JSON parsing
