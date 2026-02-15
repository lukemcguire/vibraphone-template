# CONSTITUTION.md — Project Law

The immutable rules the code reviewer checks against. Each rule has a
machine-readable ID that the reviewer references in its JSON output.

---

## Naming Conventions

### `snake-case-files`

All file names use snake_case, following Go convention. No camelCase, PascalCase, or kebab-case in file names.

### `descriptive-names`

Variables, functions, and types must have descriptive names. No single-letter variables except loop counters (`i`, `j`, `k`) and receiver names. Exported names use PascalCase, unexported use camelCase, per Go convention.

### `branch-prefix`

Git branches must follow the pattern `feat/<task-id>`.

---

## Architectural Boundaries

### `single-responsibility`

Each package, function, or type should have a single responsibility. If a function does two things, split it.

### `no-circular-imports`

Go packages must not form import cycles. Structure packages so dependencies flow in one direction.

---

## Forbidden Patterns

### `no-hardcoded-secrets`

No secrets, API keys, passwords, or tokens hardcoded in source files. All secrets must come from environment variables.

### `no-naked-goroutines`

Goroutines must have proper error handling and lifecycle management. Use `errgroup`, context cancellation, or similar patterns. No fire-and-forget goroutines without recovery.

### `no-init-functions`

Avoid `init()` functions. Use explicit initialization so dependencies and side effects are visible at the call site.

### `no-panic-in-library-code`

Library packages must not call `panic()`. Return errors and let the caller decide how to handle them. `main` and test code may panic.

---

## Required Patterns

### `require-error-wrapping`

Wrap errors with context using `fmt.Errorf("context: %w", err)`. Do not discard or swallow errors silently.

### `require-doc-comments`

All exported functions, types, and package declarations must have doc comments following Go convention (`// FunctionName does...`).

### `require-error-handling`

All external calls (HTTP, file I/O, subprocess) must have explicit error handling. No ignored error return values.

---

## Dependency Rules

### `require-adr-for-new-deps`

No new dependencies may be added without a corresponding ADR entry in `docs/DECISIONS.md` explaining the choice.

### `no-duplicate-deps`

Do not add a dependency that duplicates functionality already provided by an existing dependency or the standard library.

---

## Test Requirements

### `require-tests-for-public-functions`

Every exported function must have at least one test.

### `require-test-before-code`

Follow TDD: write a failing test before writing the implementation.

### `no-skipped-tests`

No `t.Skip()` in committed test files without an accompanying issue ID explaining why.

---

## Security Checklist

### `no-secrets-in-code`

No secrets, credentials, or API keys in source code. Use `.env` and environment variables.

---

## Diagram Requirements

### `require-diagram-update`

New packages, services, or significant architectural changes must include updated Mermaid diagrams in `docs/ARCHITECTURE.md`.

### `diagram-matches-code`

Mermaid diagrams in ARCHITECTURE.md must accurately reflect the current codebase. Stale diagrams are treated as warnings.
