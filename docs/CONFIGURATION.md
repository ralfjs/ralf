# Configuration

Ralf uses a single flat config file at the project root. Zero config works out of the box — all 61 built-in rules are enabled with sensible defaults.

## Config Files

Searched in priority order:

| Format | File | Comments | Regex literals | Computed values |
|---|---|---|---|---|
| JavaScript | `.ralfrc.js` | Yes | `/regex/` | Yes (via goja) |
| JSON | `.ralfrc.json` | No | Strings only | No |
| YAML | `.ralfrc.yaml`, `.ralfrc.yml` | Yes | Strings only | No |
| TOML | `.ralfrc.toml` | Yes | Strings only | No |

Generate a starter config:

```bash
ralf init                          # .ralfrc.json (default)
ralf init --format yaml            # .ralfrc.yaml
ralf init --from-eslint            # migrate from ESLint
ralf init --from-biome             # migrate from Biome
```

## Structure

```json
{
  "rules": { ... },
  "ignores": [ ... ],
  "extends": [ ... ],
  "overrides": [ ... ]
}
```

## Rules

Each rule has a **severity** and a **matcher**. The matcher determines how the rule detects violations.

### Severity

| Value | Meaning |
|---|---|
| `"error"` | Violation causes non-zero exit code |
| `"warn"` | Reported but does not fail the build (unless `--max-warnings` exceeded) |
| `"off"` | Rule disabled |

### Matcher Types

#### Regex

Pattern-based matching via Rust's regex engine (rure-go). Fastest matcher type.

```json
{
  "no-magic-timeouts": {
    "severity": "warn",
    "regex": "setTimeout\\([^,]+,\\s*\\d{4,}\\)",
    "message": "Extract timeout to named constant"
  }
}
```

#### AST Pattern

Structural code matching with wildcard captures. `$NAME` matches a single node, `$$$NAME` matches zero or more nodes.

```json
{
  "no-console-in-prod": {
    "severity": "error",
    "pattern": "console.log($$$ARGS)",
    "message": "No console.log in production code"
  }
}
```

#### Structural Query

Tree-sitter AST node matching by kind, name, parent, and negation.

```json
{
  "no-lexical-in-case": {
    "severity": "error",
    "ast": {
      "kind": "lexical_declaration",
      "parent": { "kind": "switch_case" }
    },
    "message": "Wrap case clause in braces when using let/const"
  }
}
```

Supported fields:

| Field | Type | Description |
|---|---|---|
| `kind` | string | Tree-sitter node kind (e.g., `"function_declaration"`, `"binary_expression"`) |
| `name` | string or regex | Match the node's name field (exact string or `/regex/` in JS config) |
| `parent` | object | Parent node must match this query |
| `not` | object | Node must NOT match this sub-query |

#### Builtin

Custom Go checker rules. These are the 33 built-in rules that perform complex AST analysis (e.g., `valid-typeof`, `for-direction`, `getter-return`). Marked with `"builtin": true` in config.

```json
{
  "valid-typeof": {
    "severity": "error",
    "builtin": true,
    "message": "Invalid typeof comparison value."
  }
}
```

#### Import Ordering

Controls import statement ordering and grouping.

```json
{
  "import-order": {
    "severity": "warn",
    "imports": {
      "groups": ["builtin", "external", "internal", "parent", "sibling", "index"],
      "alphabetize": true,
      "newlineBetween": true
    }
  }
}
```

### Naming Conventions

Enforce naming patterns on AST captures. Used as a modifier on structural queries.

```json
{
  "react-boolean-prop-naming": {
    "severity": "warn",
    "ast": { "kind": "jsx_attribute" },
    "naming": {
      "match": "^(is|has|should|can|will|did)",
      "message": "Boolean props must start with is/has/should/can/will/did"
    }
  }
}
```

### Auto-Fix

Rules can specify a `fix` field with replacement text. Captures from pattern rules can be substituted.

```json
{
  "no-var": {
    "severity": "error",
    "pattern": "var $NAME = $VALUE",
    "fix": "const $NAME = $VALUE",
    "message": "Use const instead of var"
  }
}
```

Use `"fix": "delete-statement"` to remove the entire matched statement.

### Where Predicates

Restrict rules to specific files using glob patterns.

```json
{
  "no-console-in-prod": {
    "severity": "error",
    "pattern": "console.log($$$)",
    "where": {
      "file": "src/**",
      "not": { "file": "**/*.test.*" }
    }
  }
}
```

## Ignores

Global file patterns to exclude from linting. Uses [doublestar](https://github.com/bmatcuk/doublestar) glob syntax.

```json
{
  "ignores": [
    "dist/**",
    "node_modules/**",
    "*.generated.*",
    "coverage/**"
  ]
}
```

Hardcoded skips (always ignored): `.git`, `node_modules`, `dist`, `build`, `.next`, `coverage`.

## Extends

Inherit rules from another config file. Resolved relative to the config file's directory.

```json
{
  "extends": ["./base.ralfrc.json"],
  "rules": {
    "no-console": { "severity": "off" }
  }
}
```

Rules in the current config override extended rules. Multiple extends are merged left-to-right.

## Overrides

Apply rule changes to specific file patterns. Later overrides take priority.

```json
{
  "rules": {
    "no-console": { "severity": "error" },
    "eqeqeq": { "severity": "error" }
  },
  "overrides": [
    {
      "files": ["**/*.test.*", "**/*.spec.*"],
      "rules": {
        "no-console": { "severity": "off" }
      }
    },
    {
      "files": ["src/legacy/**"],
      "rules": {
        "no-var": { "severity": "warn" },
        "eqeqeq": { "severity": "off" }
      }
    }
  ]
}
```

## Resolution Order

```
1. Base rules from config (or built-in defaults if no config)
2. Extended configs merged (left-to-right, current wins)
3. Override blocks matched by file glob (later blocks win)
4. Inline suppression comments (highest priority)
```

## Inline Suppression

```js
// Disable for next line
// ralf-disable-next-line no-console
console.log("debug");

// Disable for current line
console.log("debug"); // ralf-disable no-console

// Disable block
// ralf-disable no-console, no-var
var x = 1;
console.log(x);
// ralf-enable no-console, no-var

// Disable entire file
// ralf-disable-file no-console
```

Suppression reason syntax (`-- reason text`) is planned: [#28](https://github.com/Hideart/ralf/issues/28).

## JavaScript Config

`.ralfrc.js` is evaluated once at startup via [goja](https://github.com/dop251/goja) (pure Go JS runtime). This allows computed values and regex literals:

```js
export default {
  rules: {
    "require-error-boundary": {
      ast: {
        kind: "jsx_element",
        name: /^[A-Z]/,
        parent: { not: { kind: "jsx_element", name: "ErrorBoundary" } }
      },
      where: { file: "src/pages/**" },
      message: "Page components must be wrapped in ErrorBoundary"
    }
  }
}
```

The JS runtime is only used for config loading — no JS executes during linting.

## Migration

### From ESLint

```bash
ralf init --from-eslint
```

Reads `.eslintrc.json`, `.eslintrc.yaml`, or `.eslintrc.yml`. Maps 60 ESLint core rule names to ralf equivalents (nearly all 1:1). Converts severity values (`0`/`1`/`2`, `"off"`/`"warn"`/`"error"`, array form `["error", {...}]`). Migrates `ignorePatterns`.

JS-based ESLint configs require export first: `npx eslint --print-config . > .eslintrc.json`

### From Biome

```bash
ralf init --from-biome
```

Reads `biome.json` or `biome.jsonc` (with comment/trailing comma stripping). Maps 48 Biome `category/ruleName` entries to ralf equivalents. Migrates `files.ignore`.

Both commands print a migration report listing migrated and unsupported rules.
