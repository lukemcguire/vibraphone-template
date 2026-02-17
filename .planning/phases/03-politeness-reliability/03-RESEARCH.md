# Phase 3: Politeness & Reliability - Research

**Researched:** 2026-02-16
**Domain:** Web-crawler etiquette and resilient error handling
**Confidence:** HIGH

## Summary

Phase 3 adds production-grade politeness and reliability to zombiecrawl: robots.txt compliance, retry logic with exponential backoff, comprehensive error categorization, and enhanced reporting with source page tracking. The user has made extensive locked decisions via CONTEXT.md that significantly constrain this research.

The Go ecosystem provides well-established libraries for all requirements: `golang.org/x/time/rate` for rate limiting, `github.com/temoto/robotstxt` or `github.com/jimsmart/grobotstxt` for robots.txt parsing, and `github.com/hashicorp/go-retryablehttp` or custom retry logic for resilient requests. Error classification uses Go's standard `errors.Is` and `errors.As` with `net.OpError` and `net.DNSError` types.

**Primary recommendation:** Use `temoto/robotstxt` for robots.txt (handles 404/5xx gracefully per user spec), `golang.org/x/time/rate` for token-bucket rate limiting, and implement custom retry with exponential backoff (hashicorp/retryablehttp is overkill for HEAD/GET-only crawler).

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Retry Behavior:**
- 2 retries (3 total attempts) before marking a link as broken
- Exponential backoff between retries (1s, 2s, 4s...)
- Retry all transient errors: network failures (timeout, DNS, connection refused), 5xx server errors, and 429 rate-limited responses
- Full user control via `--retries` and `--retry-delay` flags

**robots.txt Handling:**
- Strict compliance by default - respect robots.txt directives
- Cache robots.txt for 1 hour before re-fetching
- If robots.txt returns 404 (missing): proceed freely (treat as allow-all)
- If robots.txt times out or returns 5xx: proceed freely (don't block on robots.txt failure)

**Error Reporting:**
- Detailed error categories: timeout, DNS failure, connection refused, 4xx, 5xx, redirect loop
- Grouped display by category (no explicit severity labels like "critical/warning")
- Show source pages where each broken link was found
- Full list of broken links + summary stats at end
- No timestamps in error output (cleaner display)
- Stream broken links as found (real-time feedback during crawl)
- Context-aware display limits: different behavior for TUI vs non-TUI modes
- Redirect loop reporting format: Claude's discretion

**Politeness Defaults:**
- User-Agent: `zombiecrawl/1.0 (+https://github.com/.../zombiecrawl)` - tool name + URL
- 10 concurrent workers by default (balanced I/O efficiency)
- 10 requests/second rate limit by default (reasonable for most sites)
- No extra delay between requests (rate limiting spreads requests sufficiently)
- Full user control via `--rate-limit`, `--delay`, `--user-agent` flags

### Claude's Discretion

- Exact redirect loop reporting format (show final URL vs original + target vs full chain)
- How context-aware display limits work (TUI vs non-TUI thresholds)
- Exact exponential backoff calculation (base multiplier, max delay cap)

### Deferred Ideas (OUT OF SCOPE)

None - discussion stayed within phase scope

</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CRWL-07 | Tool respects robots.txt directives | `temoto/robotstxt` library with FromStatusAndBytes for 404/5xx handling, 1-hour cache with sync.Map or dedicated cache type |
| DETC-03 | Tool reports links that timeout as broken | `errors.Is(err, context.DeadlineExceeded)` and `net.OpError.Timeout()` for classification |
| DETC-04 | Tool reports links with DNS resolution failures as broken | `errors.As(err, &net.DNSError)` for DNS-specific error detection |
| DETC-06 | Tool detects and reports redirect loops | `http.Client.CheckRedirect` function with URL tracking, default 10-redirect limit |
| DETC-07 | Tool retries failed requests with exponential backoff before marking broken | Custom retry loop with `time.Sleep(backoff)` where backoff doubles each attempt, or `hashicorp/go-retryablehttp` |
| DETC-08 | Each broken link report includes the source page where it was found | Already implemented in Phase 1 (LinkResult.SourcePage), extend error categorization |

</phase_requirements>

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| golang.org/x/time/rate | v0.11.0+ | Token bucket rate limiter | Official Go team package, goroutine-safe, well-tested |
| github.com/temoto/robotstxt | v1.1.2+ | Robots.txt parsing | Handles status codes (404=allow, 5xx=allow), RFC-compliant |
| net/http | stdlib | HTTP client (already used) | Custom CheckRedirect for loop detection |
| errors | stdlib | Error classification with Is/As | Go 1.13+ error wrapping pattern |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/hashicorp/go-retryablehttp | v0.7.8+ | Full retry client | If you want drop-in retry client; may be overkill for simple crawler |
| net | stdlib | net.OpError, net.DNSError types | Error classification for network failures |
| context | stdlib | DeadlineExceeded error | Timeout detection |
| sync | stdlib | sync.Map for robots.txt cache | Thread-safe caching per host |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| temoto/robotstxt | jimsmart/grobotstxt | grobotstxt is Google's official parser port, but temoto's FromStatusAndBytes directly handles user's 404/5xx logic |
| golang.org/x/time/rate | Custom time.Sleep loop | Rate limiter handles token bucket correctly with burst support; manual sleep is error-prone |
| Custom retry loop | hashicorp/go-retryablehttp | retryablehttp adds dependency and complexity; custom retry is ~20 lines for our simple case |

**Installation:**

```bash
cd src/
go get golang.org/x/time/rate
go get github.com/temoto/robotstxt
```

---

## Architecture Patterns

### Recommended Package Structure

```
src/
├── crawler/
│   ├── crawler.go           # Add rate limiter, robots checker
│   ├── worker.go            # Add retry logic, error classification
│   ├── events.go            # Add ErrorCategory field to CrawlEvent
│   ├── robots.go            # NEW: robots.txt fetching, caching, checking
│   └── retry.go             # NEW: exponential backoff retry wrapper
├── result/
│   ├── result.go            # Add ErrorCategory field to LinkResult
│   ├── printer.go           # Add grouped-by-category output
│   └── errors.go            # NEW: error classification utilities
├── tui/
│   ├── model.go             # Add streaming broken link display
│   └── styles.go            # Add category-based styling
└── main.go                  # Add --rate-limit, --delay, --user-agent, --retries flags
```

### Pattern 1: Token Bucket Rate Limiting

**What:** Use `golang.org/x/time/rate` to limit requests per second across all workers.

**When to use:** Required for polite crawling (user's locked decision: 10 req/sec default).

**Example:**

```go
// Source: https://pkg.go.dev/golang.org/x/time/rate

import "golang.org/x/time/rate"

type Crawler struct {
    // ... existing fields
    limiter *rate.Limiter
}

func New(cfg Config, progressCh chan<- CrawlEvent) *Crawler {
    // Default 10 requests/second, burst of 10
    limiter := rate.NewLimiter(rate.Limit(cfg.RateLimit), cfg.RateLimit)

    return &Crawler{
        limiter: limiter,
        // ...
    }
}

func (c *Crawler) checkURL(ctx context.Context, job CrawlJob) Result {
    // Wait for rate limiter before making request
    if err := c.limiter.Wait(ctx); err != nil {
        // Context was cancelled while waiting
        return Result{URL: job.URL, Err: err}
    }

    // Now make the HTTP request
    // ...
}
```

**Key:** `limiter.Wait(ctx)` blocks until a token is available or context cancels. All workers share the same limiter, so 10 workers at 10 req/sec means requests are distributed but bounded.

### Pattern 2: robots.txt Compliance with Caching

**What:** Fetch and parse robots.txt per host, cache for 1 hour, handle 404/5xx gracefully.

**When to use:** Required for ethical crawling (user's locked decision).

**Example:**

```go
// Source: https://github.com/temoto/robotstxt

import "github.com/temoto/robotstxt"

type RobotsChecker struct {
    client *http.Client
    cache  sync.Map // map[string]*cachedRobots (host -> cached entry)
}

type cachedRobots struct {
    data      *robotstxt.RobotsData
    fetchedAt time.Time
}

func (r *RobotsChecker) Allowed(ctx context.Context, rawURL, userAgent string) (bool, error) {
    u, err := url.Parse(rawURL)
    if err != nil {
        return false, err
    }

    host := u.Host
    path := u.Path
    if path == "" {
        path = "/"
    }

    // Check cache
    if cached, ok := r.cache.Load(host); ok {
        cr := cached.(*cachedRobots)
        if time.Since(cr.fetchedAt) < time.Hour {
            if cr.data == nil {
                return true, nil // Cached allow-all (404 case)
            }
            return cr.data.TestAgent(path, userAgent), nil
        }
    }

    // Fetch robots.txt
    robotsURL := fmt.Sprintf("%s://%s/robots.txt", u.Scheme, host)
    req, _ := http.NewRequestWithContext(ctx, "GET", robotsURL, nil)
    resp, err := r.client.Do(req)
    if err != nil {
        // Timeout/network error: proceed freely (user decision)
        return true, nil
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)

    // Use FromStatusAndBytes for user's 404/5xx logic
    robots, err := robotstxt.FromStatusAndBytes(resp.StatusCode, body)
    if err != nil {
        // Parse error: cache allow-all and proceed
        r.cache.Store(host, &cachedRobots{data: nil, fetchedAt: time.Now()})
        return true, nil
    }

    // Cache and test
    r.cache.Store(host, &cachedRobots{data: robots, fetchedAt: time.Now()})
    return robots.TestAgent(path, userAgent), nil
}
```

**Key:** `robotstxt.FromStatusAndBytes(statusCode, body)` implements user's logic:
- 2xx: parse body and apply rules
- 4xx (including 404): return nil (allow all)
- 5xx: return nil (allow all, treat as temporary unavailability)

### Pattern 3: Exponential Backoff Retry

**What:** Retry transient errors with exponentially increasing delays (1s, 2s, 4s...).

**When to use:** Required for reliability (user's locked decision: 2 retries = 3 attempts).

**Example:**

```go
// Source: Custom implementation (hashicorp/retryablehttp is overkill)

type RetryConfig struct {
    MaxRetries int           // Default: 2 (3 total attempts)
    BaseDelay  time.Duration // Default: 1s
    MaxDelay   time.Duration // Default: 30s (cap)
}

func (c *Crawler) checkURLWithRetry(ctx context.Context, job CrawlJob, cfg RetryConfig) Result {
    var lastErr error
    backoff := cfg.BaseDelay

    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        if attempt > 0 {
            // Wait before retry
            select {
            case <-ctx.Done():
                return Result{URL: job.URL, Err: ctx.Err(), SourcePage: job.SourcePage}
            case <-time.After(backoff):
            }
            backoff = time.Duration(float64(backoff) * 2) // Double
            if backoff > cfg.MaxDelay {
                backoff = cfg.MaxDelay
            }
        }

        result := c.checkURL(ctx, job)
        if result.Err == nil && result.StatusCode < 400 {
            return result // Success
        }

        // Classify error
        if !c.shouldRetry(result) {
            return result // Non-retryable error (4xx except 429)
        }

        lastErr = result.Err
        if result.StatusCode > 0 {
            lastErr = fmt.Errorf("HTTP %d", result.StatusCode)
        }
    }

    // All retries exhausted
    return Result{
        URL:        job.URL,
        Err:        fmt.Errorf("after %d retries: %w", cfg.MaxRetries, lastErr),
        SourcePage: job.SourcePage,
    }
}

func (c *Crawler) shouldRetry(result Result) bool {
    // Retry transient errors (user decision)
    // - Network failures: timeout, DNS, connection refused
    // - 5xx server errors
    // - 429 rate-limited

    if result.StatusCode == 0 {
        // Network error - check if retryable
        return isRetryableNetworkError(result.Err)
    }

    // HTTP status codes
    return result.StatusCode == 429 || result.StatusCode >= 500
}

func isRetryableNetworkError(err error) bool {
    if err == nil {
        return false
    }

    // Context timeout/deadline
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }

    // Network operation errors
    var opErr *net.OpError
    if errors.As(err, &opErr) {
        // Timeout, connection refused, etc.
        return true
    }

    // DNS errors
    var dnsErr *net.DNSError
    if errors.As(err, &dnsErr) {
        return true // DNS might be temporary
    }

    return false
}
```

### Pattern 4: Error Classification for Reporting

**What:** Categorize errors into types for grouped display: timeout, DNS failure, connection refused, 4xx, 5xx, redirect loop.

**When to use:** Required for detailed error reporting (user's locked decision).

**Example:**

```go
// Source: Standard library error types

type ErrorCategory string

const (
    CategoryTimeout        ErrorCategory = "timeout"
    CategoryDNSFailure     ErrorCategory = "dns_failure"
    CategoryConnectionRefused ErrorCategory = "connection_refused"
    Category4xx            ErrorCategory = "4xx"
    Category5xx            ErrorCategory = "5xx"
    CategoryRedirectLoop   ErrorCategory = "redirect_loop"
    CategoryUnknown        ErrorCategory = "unknown"
)

func ClassifyError(err error, statusCode int, isRedirectLoop bool) ErrorCategory {
    if isRedirectLoop {
        return CategoryRedirectLoop
    }

    if statusCode > 0 {
        if statusCode >= 400 && statusCode < 500 {
            return Category4xx
        }
        if statusCode >= 500 {
            return Category5xx
        }
    }

    if err == nil {
        return CategoryUnknown
    }

    // Context deadline exceeded
    if errors.Is(err, context.DeadlineExceeded) {
        return CategoryTimeout
    }

    // DNS lookup failure
    var dnsErr *net.DNSError
    if errors.As(err, &dnsErr) {
        return CategoryDNSFailure
    }

    // Connection refused (specific net.OpError)
    var opErr *net.OpError
    if errors.As(err, &opErr) {
        if opErr.Op == "dial" {
            if strings.Contains(opErr.Error(), "connection refused") {
                return CategoryConnectionRefused
            }
        }
        if opErr.Timeout() {
            return CategoryTimeout
        }
    }

    return CategoryUnknown
}
```

### Pattern 5: Redirect Loop Detection

**What:** Track redirect chains using http.Client's CheckRedirect to detect loops.

**When to use:** Required for DETC-06 requirement.

**Example:**

```go
// Source: Go standard library net/http

func (c *Crawler) checkURL(ctx context.Context, job CrawlJob) Result {
    var redirectLoop bool
    var visitedURLs []string

    client := &http.Client{
        Timeout: c.cfg.RequestTimeout,
        CheckRedirect: func(req *http.Request, via []*http.Request) error {
            // Track visited URLs in this redirect chain
            currentURL := req.URL.String()
            for _, v := range visitedURLs {
                if v == currentURL {
                    redirectLoop = true
                    return http.ErrUseLastResponse // Stop redirecting
                }
            }
            visitedURLs = append(visitedURLs, currentURL)

            // Also limit total redirects
            if len(via) >= 10 {
                redirectLoop = true
                return errors.New("too many redirects (10)")
            }
            return nil
        },
    }

    // ... make request, handle response

    if redirectLoop {
        return Result{
            URL:           job.URL,
            Err:           fmt.Errorf("redirect loop detected"),
            ErrorCategory: CategoryRedirectLoop,
            SourcePage:    job.SourcePage,
        }
    }
    // ...
}
```

### Pattern 6: Grouped Error Display

**What:** Group broken links by error category in output, show source pages.

**When to use:** Required for user's error reporting decision (grouped by category).

**Example:**

```go
// In result/printer.go

func PrintGroupedResults(w io.Writer, res *Result) {
    // Group by category
    grouped := make(map[ErrorCategory][]LinkResult)
    for _, link := range res.BrokenLinks {
        cat := link.ErrorCategory
        if cat == "" {
            cat = CategoryUnknown
        }
        grouped[cat] = append(grouped[cat], link)
    }

    // Print each category
    for _, cat := range []ErrorCategory{
        CategoryTimeout,
        CategoryDNSFailure,
        CategoryConnectionRefused,
        Category4xx,
        Category5xx,
        CategoryRedirectLoop,
        CategoryUnknown,
    } {
        links, ok := grouped[cat]
        if !ok {
            continue
        }

        fmt.Fprintf(w, "\n## %s (%d)\n", formatCategory(cat), len(links))
        for _, link := range links {
            fmt.Fprintf(w, "  URL: %s\n", link.URL)
            if link.StatusCode > 0 {
                fmt.Fprintf(w, "  Status: %d\n", link.StatusCode)
            } else if link.Error != "" {
                fmt.Fprintf(w, "  Error: %s\n", link.Error)
            }
            fmt.Fprintf(w, "  Found on: %s\n\n", link.SourcePage)
        }
    }

    // Summary
    fmt.Fprintf(w, "\nChecked %d URLs, found %d broken links\n",
        res.Stats.TotalChecked, res.Stats.BrokenCount)
}

func formatCategory(cat ErrorCategory) string {
    switch cat {
    case CategoryTimeout:
        return "Timeouts"
    case CategoryDNSFailure:
        return "DNS Failures"
    case CategoryConnectionRefused:
        return "Connection Refused"
    case Category4xx:
        return "Client Errors (4xx)"
    case Category5xx:
        return "Server Errors (5xx)"
    case CategoryRedirectLoop:
        return "Redirect Loops"
    default:
        return "Other Errors"
    }
}
```

### Anti-Patterns to Avoid

- **Hammering servers without rate limiting**: Will get IP banned
- **Ignoring robots.txt**: Unethical, will be blocked by many sites
- **Retrying 4xx errors (except 429)**: 404, 403 won't fix themselves
- **Not caching robots.txt**: Wastes bandwidth and time
- **Infinite retry loops**: Always have max retry count

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Rate limiting | Custom sleep loop | golang.org/x/time/rate | Token bucket is non-trivial, handles bursts correctly |
| robots.txt parsing | Regex/string matching | temoto/robotstxt | RFC-compliant, handles edge cases, status code logic |
| Retry backoff | Simple sleep | Custom with exponential | Actually OK to hand-roll for simple case, but use jitter |

**Key insight:** Rate limiting and robots.txt parsing have subtle edge cases. Use battle-tested libraries.

---

## Common Pitfalls

### Pitfall 1: Rate Limiter Bottleneck

**What goes wrong:** Creating a rate limiter per worker instead of sharing one causes N times the allowed rate.

**Why it happens:** Not realizing rate.Limiter must be shared across goroutines.

**How to avoid:** Create one limiter in Crawler, pass to workers, or access via Crawler reference.

**Warning signs:** Getting rate-limited/banned despite setting conservative rate limit.

### Pitfall 2: robots.txt Cache Never Expires

**What goes wrong:** Caching robots.txt forever means missing rule changes.

**Why it happens:** Forgetting to implement cache expiration.

**How to avoid:** Store `fetchedAt` with cache entry, check before use (1 hour TTL as per user decision).

**Warning signs:** Can't crawl pages that were recently allowed in robots.txt.

### Pitfall 3: Retrying Non-Retryable Errors

**What goes wrong:** Retrying 404s, 403s, SSL errors wastes time and won't succeed.

**Why it happens:** Not classifying errors before retrying.

**How to avoid:** Check `shouldRetry()` before retrying. Only retry 429, 5xx, and network errors.

**Warning signs:** Crawl takes much longer than expected, lots of retry attempts.

### Pitfall 4: Missing Crawl-Delay from robots.txt

**What goes wrong:** Ignoring `Crawl-delay` directive angers site owners even if you respect other rules.

**Why it happens:** Not extracting CrawlDelay from parsed robots.txt.

**How to avoid:** Use `robots.FindGroup(userAgent).CrawlDelay` and adjust rate limiter accordingly.

**Warning signs:** Sites blocking your crawler despite respecting Disallow rules.

---

## Code Examples

### Complete Retry Wrapper with Error Classification

```go
// Source: Custom implementation for zombiecrawl

package crawler

import (
    "context"
    "errors"
    "fmt"
    "net"
    "strings"
    "time"

    "github.com/lukemcguire/zombiecrawl/result"
)

type RetryPolicy struct {
    MaxRetries int           // 2 = 3 total attempts
    BaseDelay  time.Duration // 1s
    MaxDelay   time.Duration // 30s
}

func DefaultRetryPolicy() RetryPolicy {
    return RetryPolicy{
        MaxRetries: 2,
        BaseDelay:  1 * time.Second,
        MaxDelay:   30 * time.Second,
    }
}

func (c *Crawler) CheckURLWithRetry(ctx context.Context, job CrawlJob, policy RetryPolicy) CrawlResult {
    var lastResult CrawlResult
    backoff := policy.BaseDelay

    for attempt := 0; attempt <= policy.MaxRetries; attempt++ {
        // Wait before retry (not on first attempt)
        if attempt > 0 {
            select {
            case <-ctx.Done():
                return CrawlResult{
                    Job: job,
                    Result: &result.LinkResult{
                        URL:        job.URL,
                        SourcePage: job.SourcePage,
                        IsExternal: job.IsExternal,
                        Error:      ctx.Err().Error(),
                    },
                }
            case <-time.After(backoff):
            }
            // Exponential backoff: 1s -> 2s -> 4s -> ...
            backoff = time.Duration(float64(backoff) * 2)
            if backoff > policy.MaxDelay {
                backoff = policy.MaxDelay
            }
        }

        lastResult = c.CheckURL(ctx, job)

        // Success - no retry needed
        if lastResult.Err == nil && lastResult.Result == nil {
            return lastResult
        }

        // Determine if we should retry
        if !shouldRetry(lastResult) {
            return lastResult
        }
    }

    // All retries exhausted - mark with retry info
    if lastResult.Result != nil {
        lastResult.Result.Error = fmt.Sprintf("after %d retries: %s",
            policy.MaxRetries, lastResult.Result.Error)
    }
    return lastResult
}

func shouldRetry(res CrawlResult) bool {
    // No error and no broken link = success
    if res.Err == nil && res.Result == nil {
        return false
    }

    // If we have a status code, check it
    if res.Result != nil && res.Result.StatusCode > 0 {
        code := res.Result.StatusCode
        // Retry: 429 (rate limited), 5xx (server errors)
        return code == 429 || code >= 500
    }

    // Network error - check type
    if res.Err != nil {
        return isRetryableError(res.Err)
    }
    if res.Result != nil && res.Result.Error != "" {
        // Parse error string for network errors
        return strings.Contains(res.Result.Error, "timeout") ||
               strings.Contains(res.Result.Error, "connection refused") ||
               strings.Contains(res.Result.Error, "DNS")
    }

    return false
}

func isRetryableError(err error) bool {
    if errors.Is(err, context.DeadlineExceeded) {
        return true
    }

    var opErr *net.OpError
    if errors.As(err, &opErr) {
        return true // Most network errors are retryable
    }

    var dnsErr *net.DNSError
    if errors.As(err, &dnsErr) {
        return true // DNS might be temporary
    }

    return false
}
```

---

## Open Questions

1. **Redirect loop display format**
   - What we know: User wants redirect loop reporting, format is Claude's discretion
   - Recommendation: Show original URL + detected cycle (e.g., "A -> B -> C -> A")

2. **TUI vs non-TUI display limits**
   - What we know: User wants context-aware limits
   - Recommendation: TUI shows all (scrollable), non-TUI limits to 50 per category with summary

---

## Sources

### Primary (HIGH confidence)
- [golang.org/x/time/rate documentation](https://pkg.go.dev/golang.org/x/time/rate) - Token bucket rate limiter
- [temoto/robotstxt GitHub](https://github.com/temoto/robotstxt) - robots.txt parser for Go
- [Go net/http documentation](https://pkg.go.dev/net/http) - CheckRedirect, http.Client

### Secondary (MEDIUM confidence)
- [hashicorp/go-retryablehttp](https://pkg.go.dev/github.com/hashicorp/go-retryablehttp) - Retry patterns reference
- [Go error handling with errors.Is/As](https://go.dev/blog/error-handling) - Error classification patterns

### Tertiary (LOW confidence)
- Web search results for retry patterns and exponential backoff implementations

---

Now let me write this research to the RESEARCH.md file.Bashcommandnode /home/luke/.claude/get-shit-done/bin/gsd-tools.cjs init phase-op "03" 2>/dev/null | head -1