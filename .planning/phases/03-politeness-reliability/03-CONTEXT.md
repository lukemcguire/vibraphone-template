# Phase 3: Politeness & Reliability - Context

**Gathered:** 2026-02-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Web-crawler etiquette and resilient error handling for production use. This phase adds robots.txt respect, retry logic, comprehensive error detection, and enhanced reporting with source page tracking. Configuration flags for depth/JSON/CSV belong in Phase 4.

</domain>

<decisions>
## Implementation Decisions

### Retry Behavior
- 2 retries (3 total attempts) before marking a link as broken
- Exponential backoff between retries (1s, 2s, 4s...)
- Retry all transient errors: network failures (timeout, DNS, connection refused), 5xx server errors, and 429 rate-limited responses
- Full user control via `--retries` and `--retry-delay` flags

### robots.txt Handling
- Strict compliance by default — respect robots.txt directives
- Cache robots.txt for 1 hour before re-fetching
- If robots.txt returns 404 (missing): proceed freely (treat as allow-all)
- If robots.txt times out or returns 5xx: proceed freely (don't block on robots.txt failure)

### Error Reporting
- Detailed error categories: timeout, DNS failure, connection refused, 4xx, 5xx, redirect loop
- Grouped display by category (no explicit severity labels like "critical/warning")
- Show source pages where each broken link was found
- Full list of broken links + summary stats at end
- No timestamps in error output (cleaner display)
- Stream broken links as found (real-time feedback during crawl)
- Context-aware display limits: different behavior for TUI vs non-TUI modes
- Redirect loop reporting format: Claude's discretion

### Politeness Defaults
- User-Agent: `zombiecrawl/1.0 (+https://github.com/.../zombiecrawl)` — tool name + URL
- 10 concurrent workers by default (balanced I/O efficiency)
- 10 requests/second rate limit by default (reasonable for most sites)
- No extra delay between requests (rate limiting spreads requests sufficiently)
- Full user control via `--rate-limit`, `--delay`, `--user-agent` flags

### Claude's Discretion
- Exact redirect loop reporting format (show final URL vs original + target vs full chain)
- How context-aware display limits work (TUI vs non-TUI thresholds)
- Exact exponential backoff calculation (base multiplier, max delay cap)

</decisions>

<specifics>
## Specific Ideas

- Balance concurrency and rate limiting: workers share the rate limit budget, so 10 workers at 10 req/sec means requests are distributed but bounded
- No artificial delay needed — rate limiting naturally spreads requests over time
- Streaming broken links as found gives immediate feedback during long crawls

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 03-politeness-reliability*
*Context gathered: 2026-02-16*
