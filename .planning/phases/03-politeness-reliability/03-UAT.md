---
status: resolved
phase: 03-politeness-reliability
source: [03-01-PLAN.md, 03-02-PLAN.md, 03-03-PLAN.md, 03-04-PLAN.md, 03-05-PLAN.md]
started: 2026-02-17T06:25:00Z
updated: 2026-02-17T00:05:00Z
resolved_by: 03-05-PLAN.md
---

## Current Test

[testing complete]

## Tests

### 1. CLI flags for rate limiting
expected: Running `zombiecrawl -h` shows flags for --rate-limit (-r), --retries (-n), --retry-delay, and --user-agent (-U) with correct defaults (10, 2, 1s, zombiecrawl/1.0).
result: pass
fixed_by: 03-05-PLAN.md

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
result: pass
fixed_by: 03-05-PLAN.md

### 10. 4xx errors not retried
expected: 4xx errors (except 429) are marked as broken immediately without retry attempts.
result: pass

### 11. Redirect loop detection
expected: URLs that cause redirect loops are detected and reported as broken with "redirect loop" error category.
result: pass

## Summary

total: 11
passed: 11
issues: 0
pending: 0
skipped: 0

## Gaps

- truth: "CLI help output is clean and readable with correct defaults"
  status: resolved
  reason: "User reported: Flags work but help output is ugly: short/long flags on separate lines, duplicate defaults shown ((default 10) (default 10)), default concurrency is 17 instead of expected 10"
  severity: cosmetic
  test: 1
  root_cause: "main.go uses both flag.Int and flag.IntVar for same variables (creates duplicate entries). Default concurrency hardcoded to 17 in crawler.go line 38."
  resolved_by: 03-05-PLAN.md
  artifacts:
    - path: "src/main.go"
      issue: "Duplicate flag definitions for -c/-concurrency, -r/-rate-limit, -n/-retries"
    - path: "src/crawler/crawler.go"
      issue: "Default concurrency set to 17 instead of 10"
  missing:
    - "Use single flag definition pattern or custom flag type for short/long aliases"
    - "Change default concurrency to 10 in crawler.go"

- truth: "-retries flag controls number of retry attempts"
  status: resolved
  reason: "User reported: -retries flag not working - both default and -retries 0 show '(after 3 attempts)'. The retries value isn't being passed through to the retry logic."
  severity: major
  test: 9
  root_cause: "crawler.go line 49: `if cfg.RetryPolicy.MaxRetries == 0` overwrites user's explicit -retries 0 with default. Can't distinguish between 'not set' and 'explicitly set to 0'."
  resolved_by: 03-05-PLAN.md
  artifacts:
    - path: "src/crawler/crawler.go"
      issue: "Zero-value check conflates 'unset' with 'explicitly zero'"
  missing:
    - "Use pointer or separate 'isSet' flag to distinguish unset from explicit zero"
    - "Or change default check to happen before CLI parsing, using non-zero default in Config struct"
