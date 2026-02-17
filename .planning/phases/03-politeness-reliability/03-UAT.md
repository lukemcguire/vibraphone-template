---
status: complete
phase: 03-politeness-reliability
source: [03-01-PLAN.md, 03-02-PLAN.md, 03-03-PLAN.md, 03-04-PLAN.md]
started: 2026-02-17T06:25:00Z
updated: 2026-02-17T06:45:00Z
---

## Current Test

[testing complete]

## Tests

### 1. CLI flags for rate limiting
expected: Running `zombiecrawl -h` shows flags for --rate-limit (-r), --retries (-n), --retry-delay, and --user-agent (-U) with correct defaults (10, 2, 1s, zombiecrawl/1.0).
result: issue
reported: "Flags work but help output is ugly: short/long flags on separate lines, duplicate defaults shown ((default 10) (default 10)), default concurrency is 17 instead of expected 10"
severity: cosmetic

### 2. CLI flags are accepted
expected: Running `zombiecrawl -rate-limit 5 -retries 3 -user-agent "test/1.0" <url>` accepts all flags without error.
result: pass

### 3. Broken links grouped by category
expected: When broken links are found, output displays them grouped by error category (Client Errors 4xx, Server Errors 5xx, Timeouts, DNS Failures, etc.) with styled headers.
result: pass

### 4. Source page shown for each broken link
expected: Each broken link in the output shows the URL where it was found (the "Found On" or "Source Page" column).
result: pass

### 5. Error categories in results
expected: Broken links are classified into categories: timeout, DNS failure, connection refused, 4xx, 5xx, redirect loop, or unknown.
result: pass

### 6. Rate limiting active
expected: Crawler paces requests according to the --rate-limit setting (default 10 req/sec). Multiple requests don't fire all at once.
result: pass

### 7. Robots.txt respected
expected: URLs disallowed by robots.txt are skipped during crawling (not fetched).
result: pass

### 8. Missing robots.txt allows all
expected: When a site has no robots.txt (404), crawler proceeds to crawl all discovered URLs.
result: pass

### 9. Retry logic with backoff
expected: Transient errors (5xx, 429, timeouts, network errors) are retried up to 3 times with increasing delays.
result: issue
reported: "-retries flag not working - both default and -retries 0 show '(after 3 attempts)'. The retries value isn't being passed through to the retry logic."
severity: major

### 10. 4xx errors not retried
expected: 4xx errors (except 429) are marked as broken immediately without retry attempts.
result: pass

### 11. Redirect loop detection
expected: URLs that cause redirect loops are detected and reported as broken with "redirect loop" error category.
result: pass

## Summary

total: 11
passed: 9
issues: 2
pending: 0
skipped: 0

## Gaps

- truth: "CLI help output is clean and readable with correct defaults"
  status: failed
  reason: "User reported: Flags work but help output is ugly: short/long flags on separate lines, duplicate defaults shown ((default 10) (default 10)), default concurrency is 17 instead of expected 10"
  severity: cosmetic
  test: 1

- truth: "-retries flag controls number of retry attempts"
  status: failed
  reason: "User reported: -retries flag not working - both default and -retries 0 show '(after 3 attempts)'. The retries value isn't being passed through to the retry logic."
  severity: major
  test: 9
