# CONSTITUTION.md — Project Law

The immutable rules the code reviewer checks against. Each rule has a
machine-readable ID that the reviewer references in its JSON output.

---

## Naming Conventions

### `kebab-case-files`

All file names use kebab-case. No camelCase, PascalCase, or snake_case in file names.

### `descriptive-names`

Variables, functions, and classes must have descriptive names. No single-letter variables except loop counters (`i`, `j`, `k`).

### `branch-prefix`

Git branches must follow the pattern `feat/<task-id>`.

---

## Architectural Boundaries

### `no-business-logic-in-controller`

Route handlers and controllers must not contain business logic. Delegate to service or domain layers.

### `no-direct-db-in-routes`

No direct database calls from route handlers. All data access goes through a repository or data-access layer.

### `single-responsibility`

Each module, class, or function should have a single responsibility. If a function does two things, split it.

---

## Forbidden Patterns

### `no-any-types`

Do not use `any` types in TypeScript. Use proper types, generics, or `unknown` with type guards.

### `no-raw-sql-outside-orm`

No raw SQL queries outside the ORM/query-builder layer. All database access must use the project's data-access abstraction.

### `no-console-log-in-production`

No `console.log` in production code. Use the project's logging abstraction.

### `no-hardcoded-secrets`

No secrets, API keys, passwords, or tokens hardcoded in source files. All secrets must come from environment variables.

---

## Required Patterns

### `require-input-validation`

All API endpoints must validate input before processing. Use schema validation (e.g., Zod, Pydantic).

### `require-error-handling`

All external calls (API, database, file I/O) must have explicit error handling. No unhandled promise rejections or bare exceptions.

### `require-return-types`

All exported functions must have explicit return type annotations.

---

## Dependency Rules

### `require-adr-for-new-deps`

No new dependencies may be added without a corresponding ADR entry in `docs/DECISIONS.md` explaining the choice.

### `no-duplicate-deps`

Do not add a dependency that duplicates functionality already provided by an existing dependency.

---

## Test Requirements

### `require-tests-for-public-functions`

Every public function or exported module must have at least one test.

### `require-test-before-code`

Follow TDD: write a failing test before writing the implementation.

### `no-skipped-tests`

No `.skip` or `@pytest.mark.skip` in committed test files without an accompanying issue ID explaining why.

---

## Security Checklist

### `no-secrets-in-code`

No secrets, credentials, or API keys in source code. Use `.env` and environment variables.

### `sanitize-user-input`

All user input must be sanitized before use in queries, commands, or output rendering.

### `no-eval`

No use of `eval()`, `exec()`, or equivalent dynamic code execution on user-provided data.

---

## Diagram Requirements

### `require-diagram-update`

New API endpoints, database tables, services, or significant architectural changes must include updated Mermaid diagrams in `docs/ARCHITECTURE.md`.

### `diagram-matches-code`

Mermaid diagrams in ARCHITECTURE.md must accurately reflect the current codebase. Stale diagrams are treated as warnings.
