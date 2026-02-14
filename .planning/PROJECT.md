# zombiecrawl

## What This Is

A beautiful CLI tool written in Go that crawls a website and detects dead links. It recursively follows every anchor tag on pages within the target domain, checks that external links resolve, and reports broken links with a polished Charm-powered TUI experience. Built as a learning project for Go and CLI tooling.

## Core Value

Reliably find every dead link on a website by crawling all same-domain pages and checking every outbound link — with output that's prettier than it has any right to be.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Recursive crawling of same-domain pages via anchor tags
- [ ] External link validation (check reachability, don't crawl further)
- [ ] Dead link detection (4xx, 5xx, timeouts, DNS failures, connection errors)
- [ ] Treat redirects (3xx) as valid
- [ ] Deduplication — each URL checked exactly once (no infinite loops)
- [ ] Live TUI progress during crawl (Bubble Tea)
- [ ] Pretty formatted summary after crawl completes (Lip Gloss)
- [ ] Human-readable output by default
- [ ] Structured output via --json and --csv flags
- [ ] Configurable concurrency (--concurrency flag, reasonable default)
- [ ] Non-zero exit code when broken links found
- [ ] Static HTML only (no JS rendering)

### Out of Scope

- JavaScript-rendered pages — deferred to future version
- Scheduled/periodic monitoring — this is a run-once CLI tool
- CI/CD integration — works via exit codes but no dedicated integrations
- Link checking beyond anchor tags (images, scripts, stylesheets)

## Context

- Learning project focused on writing good Go and building CLI tools
- Using the Charm ecosystem for TUI/styling (Bubble Tea, Lip Gloss, Bubbles, Log)
- HTML parsing via `golang.org/x/net/html`, HTTP via stdlib `net/http`
- No heavy framework — stdlib where possible, Charm for the experience
- Lives within a vibraphone-template repo (coding framework project)
- Source code in `src/`, tests in `tests/unit/` and `tests/integration/`

## Constraints

- **Language**: Go — this is the learning goal
- **Dependencies**: Charm ecosystem + golang.org/x/net/html + stdlib. Minimal beyond that.
- **Project structure**: Code in `src/`, unit tests in `tests/unit/`, integration tests in `tests/integration/`
- **Rendering**: Static HTML only for v1. No headless browser or JS execution.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Charm ecosystem for CLI UX | User wants beautiful output, learning Charm libs | — Pending |
| No Cobra, use Bubble Tea for arg handling | Charm stack handles CLI interaction natively | — Pending |
| golang.org/x/net/html for parsing | Standard Go HTML parser, no extra deps needed | — Pending |
| Static HTML only for v1 | Keeps scope manageable, JS rendering is a separate concern | — Pending |
| Code in src/, tests in tests/ | Follows vibraphone-template repo conventions | — Pending |

---
*Last updated: 2026-02-13 after initialization*
