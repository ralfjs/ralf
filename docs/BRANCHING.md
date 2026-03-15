# Branching & Release Strategy

## Branch Model

Git Flow-based with `main` (stable releases) and `develop` (integration).

```
main          ──●──────────────────●──────────────────●──  (tagged releases only)
                │                  ↑                  ↑
                │                  │ merge             │ merge
                │                  │                   │
develop       ──●──●──●──●──●──●──●──●──●──●──●──●──●──  (integration branch)
                   ↑  ↑        ↑     ↑  ↑        ↑
                   │  │        │     │  │        │
feature/*      ────┘  │        │     │  │        │
fix/*             ────┘        │     │  │        │
feat/*                     ────┘     │  │        │
hotfix/*                          ───┘  │        │
release/*                           ────┘        │
chore/*                                      ────┘
```

## Branches

### Long-lived

| Branch | Purpose | Protected | CI |
|---|---|---|---|
| `main` | Stable releases. Every commit is a tagged release or hotfix. | Yes — no direct push, PR only, require CI pass | Full: lint, test, build |
| `develop` | Integration branch. Features merge here first. | Yes — PR only, require CI pass | Full: lint, test, build |

### Short-lived

| Pattern | Base | Merges into | Purpose |
|---|---|---|---|
| `feat/<ticket>-<description>` | `develop` | `develop` | New features |
| `fix/<ticket>-<description>` | `develop` | `develop` | Bug fixes (non-urgent) |
| `refactor/<description>` | `develop` | `develop` | Code restructuring |
| `test/<description>` | `develop` | `develop` | Test additions/improvements |
| `docs/<description>` | `develop` | `develop` | Documentation updates |
| `chore/<description>` | `develop` | `develop` | Tooling, CI, dependencies |
| `perf/<description>` | `develop` | `develop` | Performance improvements |
| `release/v<version>` | `develop` | `main` + `develop` | Release preparation |
| `hotfix/v<version>-<description>` | `main` | `main` + `develop` | Critical production fixes |

### Naming Rules

- Lowercase, hyphen-separated: `feat/add-regex-engine`, not `feat/Add_Regex_Engine`
- Include ticket ID when applicable: `feat/BP-42-ast-pattern-matching`
- Keep descriptions short (3-5 words max): `fix/duplicate-diagnostics`
- No nested slashes beyond the prefix: `feat/parser-error-recovery`, not `feat/parser/error-recovery`

## Workflow

### Feature Development

```bash
# 1. Create feature branch from develop
git checkout develop
git pull origin develop
git checkout -b feat/BP-42-ast-pattern-matching

# 2. Work on the feature (multiple commits OK)
git commit -m "feat(engine): add AST pattern parser"
git commit -m "feat(engine): add AST pattern matcher"
git commit -m "test(rules): add pattern matching fixtures"

# 3. Push and create PR targeting develop
git push -u origin feat/BP-42-ast-pattern-matching
gh pr create --base develop --title "feat(engine): AST pattern matching" --body "..."

# 4. PR review + CI must pass
# 5. Squash merge into develop (default)
# 6. Delete feature branch
```

### Release Process

```bash
# 1. Create release branch from develop
git checkout develop
git pull origin develop
git checkout -b release/v0.1.0

# 2. Version bump, changelog, final fixes only
# - Update version in main.go or version file
# - Update CHANGELOG.md
# - Fix release-blocking bugs only — NO new features
git commit -m "chore: bump version to v0.1.0"
git commit -m "docs: update changelog for v0.1.0"

# 3. PR into main
gh pr create --base main --title "release: v0.1.0" --body "..."

# 4. After merge to main — tag the release
git checkout main
git pull origin main
git tag -a v0.1.0 -m "v0.1.0: Linter MVP"
git push origin v0.1.0

# 5. Merge main back into develop (capture version bump + any release fixes)
git checkout develop
git merge main
git push origin develop

# 6. Delete release branch
git branch -d release/v0.1.0
git push origin --delete release/v0.1.0

# 7. GoReleaser builds and publishes automatically on tag push
```

### Hotfix Process

```bash
# 1. Create hotfix branch from main
git checkout main
git pull origin main
git checkout -b hotfix/v0.1.1-fix-crash-on-empty-file

# 2. Fix the issue
git commit -m "fix(engine): handle empty file without crash"

# 3. PR into main
gh pr create --base main --title "fix(engine): handle empty file without crash" --body "..."

# 4. After merge — tag
git checkout main
git pull origin main
git tag -a v0.1.1 -m "v0.1.1: Fix crash on empty file"
git push origin v0.1.1

# 5. Merge main back into develop
git checkout develop
git merge main
git push origin develop

# 6. Delete hotfix branch
```

## Versioning

Follows [Semantic Versioning 2.0.0](https://semver.org/).

```
v<MAJOR>.<MINOR>.<PATCH>

MAJOR  — breaking changes (config format, CLI flags, rule behavior)
MINOR  — new features (new rules, new config options, new CLI commands)
PATCH  — bug fixes, performance improvements, documentation
```

### Pre-1.0 Rules

While in `v0.x`:
- `MINOR` bumps may include breaking changes (documented in changelog)
- `PATCH` bumps are strictly backwards-compatible
- No stability guarantee until `v1.0.0`

### Version Mapping to Milestones

| Version | Milestone |
|---|---|
| `v0.1.0` | Linter MVP — regex + AST patterns, CLI, 50 rules |
| `v0.2.0` | Project-aware — cache, module graph, LSP, VS Code |
| `v0.3.0` | Formatter — dprint WASM, auto-fix, import sorting |
| `v0.4.0` | WASM plugins — Go/Rust/AS SDKs |
| `v1.0.0` | Type-aware rules, production-ready |

## Merge Strategy

| Target | Strategy | Why |
|---|---|---|
| Feature → `develop` | **Squash merge** | Clean history, one commit per feature |
| Release → `main` | **Merge commit** | Preserve release branch history |
| Hotfix → `main` | **Merge commit** | Preserve hotfix context |
| `main` → `develop` (back-merge) | **Merge commit** | Track integration point |

### PR Rules

- All PRs require at least **1 approval** (when team > 1)
- All PRs must pass CI (lint, test, build)
- Feature PRs should be **squash merged** with a conventional commit message as the squash title
- PR description must include: what changed, why, how to test
- Branch must be up to date with base before merge

## Tags

- Tags are created only on `main` branch
- Format: `v<semver>` — e.g., `v0.1.0`, `v0.2.0-beta.1`
- Annotated tags only (`git tag -a`), with release description
- Tag push triggers GoReleaser (builds binaries, creates GitHub Release)
- Pre-release tags use semver pre-release: `v0.2.0-alpha.1`, `v0.2.0-beta.1`, `v0.2.0-rc.1`

## Changelog

`CHANGELOG.md` at project root, updated during release branch preparation.

Format follows [Keep a Changelog](https://keepachangelog.com/):

```markdown
# Changelog

## [v0.1.0] — 2026-XX-XX

### Added
- Regex rule engine with rure-go parallel scanning
- AST pattern matching with $VAR syntax
- CLI: `ralf lint`, `ralf check`, `ralf init`
- 50 built-in rules (ESLint recommended + React plugin)
- JSON, YAML, JS config support

### Fixed
- ...

### Changed
- ...
```

## Branch Protection (GitHub Settings)

### `main`
- Require PR reviews (1+)
- Require status checks: `lint`, `test`, `build`, `verify`
- Require branch to be up to date
- No direct push
- No force push
- Require linear history: no (merge commits from releases/hotfixes)

### `develop`
- Require PR reviews (1+ when team > 1, optional for solo dev)
- Require status checks: `lint`, `test`, `build`
- No direct push
- No force push
