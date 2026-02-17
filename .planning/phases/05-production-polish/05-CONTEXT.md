# Phase 5: Production Polish - Context

**Gathered:** 2026-02-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Edge case handling and refinements for reliable production use. Memory management at scale, performance optimization, and user-friendly error messaging. No new features — hardening existing functionality for real-world deployment.

</domain>

<decisions>
## Implementation Decisions

### Edge Case Handling
- Malformed HTML that fails to parse → treat as broken link, report in results
- Redirect chains → no hard limit, detect cycles only
- Binary files (PDFs, images, zips) → quick HEAD check, report as valid if 2xx response, skip parsing
- Auth-gated pages (401/403) → detect and classify as "requires auth" rather than broken

### Memory Strategy
- Target scale: 100,000+ pages
- URL tracking: disk-backed bloom filter (replace current `sync.Map` in-memory approach)
  - Memory-mapped file for flat memory footprint
  - Zero false positives — if bloom says visited, it's visited
- Memory pressure response: dynamic throttling (reduce concurrency, prune if needed)
- HTML buffering: use Go's http client default behavior

### Error Messages
- Style: minimal by default, trust user to investigate
- Invalid URLs: explain the issue (missing scheme, etc.)
- Network errors: add `--verbose-network` flag for detailed diagnostics (DNS, timeout, connection refused)
  - Default: simple error messages
  - `--verbose-network`: full error chain with context
- Exit codes: binary (0 = success, 1 = failure) — already implemented

### Performance Tuning
- Target speed: 50 pages/second (matches success criteria)
- Auto-tune concurrency: dynamically adjust worker count based on server response times
  - Ramp up if server handles load well
  - Back off if responses slow down
  - Explicit `--concurrency` flag caps or disables auto-tune
- Rename `--rate-limit` to `--delay` (breaking change)
  - Change semantics from req/sec to ms between requests
  - 10 req/sec → 100ms delay
  - Apply stricter of `--delay` or robots.txt `Crawl-delay`
- HTTP connections: use Go's default transport behavior

### Breaking Changes
- `--rate-limit` renamed to `--delay` with inverted semantics (delay vs rate)
- Update all code references from rate-limit to delay

</decisions>

<specifics>
## Specific Ideas

- Auto-tune should feel "smart but predictable" — user always knows they can override with explicit flag
- Memory monitoring should kick in before OS kills the process, not after

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 05-production-polish*
*Context gathered: 2026-02-17*
