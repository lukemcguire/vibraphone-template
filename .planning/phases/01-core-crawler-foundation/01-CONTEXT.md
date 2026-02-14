# Phase 1: Core Crawler Foundation - Context

**Gathered:** 2026-02-13
**Status:** Ready for planning

<domain>
## Phase Boundary

Working recursive crawler that detects dead links (4xx, 5xx) with proper concurrency patterns. Supports configurable concurrency, same-domain recursive crawling, external link validation, and deduplication. Outputs basic text results. TUI (Phase 2), robots.txt (Phase 3), and structured output formats (Phase 4) are separate phases.

</domain>

<decisions>
## Implementation Decisions

### Worker pool design
- Per-request timeout (not global crawl timeout) — each request gets a deadline, then moves on
- Default concurrency: 17 workers
- Graceful shutdown on Ctrl+C — stop new requests, wait for in-flight to finish, show partial results
- Buffered channels as work queue (standard Go pattern)

### URL handling
- Same-domain includes subdomains (*.example.com all crawled recursively)
- Query params preserved for deduplication (different query = different page)
- Fragments (#) stripped for deduplication
- Normalize scheme (http/https treated as same domain)
- Trailing slashes stripped for deduplication (/about and /about/ = same URL)
- Non-HTTP schemes (mailto:, tel:, javascript:) skipped silently
- External links: HEAD request first, GET fallback if HEAD fails or is rejected
- Internal links: full GET (need to parse HTML for more links)

### Output during crawl (pre-TUI)
- Basic logging — print each URL as it's checked during crawl
- Broken link reports include: URL, status code/error, source page where link was found
- Summary at end: "Checked N URLs, found M broken links"
- Healthy link verbosity: Claude's discretion

### Project structure
- Go module: `github.com/lukemcguire/zombiecrawl`
- go.mod lives in `src/` (scoped to Go code, repo root stays clean)
- Go latest stable version
- Package organization: Claude's discretion (flat vs by-responsibility)
- Entry point location: Claude's discretion
- Tests follow Go convention: `_test.go` files alongside source (NOT in tests/ directory)

### Claude's Discretion
- Package organization within src/ (flat vs separate packages)
- Entry point location (src/main.go vs src/cmd/zombiecrawl/main.go)
- Healthy link logging verbosity
- Relative URL resolution approach
- Per-request timeout duration
- Channel buffer sizes

</decisions>

<specifics>
## Specific Ideas

- Default concurrency of 17 because "it's the only truly random number"
- External link validation uses HEAD-first-then-GET pattern to save bandwidth
- Graceful shutdown should show partial results found so far

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-core-crawler-foundation*
*Context gathered: 2026-02-13*
