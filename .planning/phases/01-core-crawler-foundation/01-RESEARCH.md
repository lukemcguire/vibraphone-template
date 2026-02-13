# Phase 1: Core Crawler Foundation - Research

**Researched:** 2026-02-13
**Domain:** Concurrent web crawler in Go
**Confidence:** HIGH

## Summary

Building a concurrent web crawler in Go requires understanding the standard library's HTTP client, HTML parsing, URL handling, and Go's concurrency primitives (goroutines, channels, context). The user has already made key architectural decisions via CONTEXT.md that constrain this research: worker pool with buffered channels, per-request timeouts, graceful shutdown, and specific URL normalization rules.

The Go ecosystem provides everything needed in the standard library and official extensions (golang.org/x/net/html). The worker pool pattern is well-established for this use case, using buffered channels as work queues and sync.WaitGroup or errgroup for coordination. Common pitfalls include goroutine leaks (prevented with context cancellation), concurrent map access (use sync.Map or mutex), and infinite crawler loops (prevented with proper deduplication and visited tracking).

**Primary recommendation:** Use standard library net/http with context-based timeouts, golang.org/x/net/html for parsing, worker pool pattern with buffered channels, and errgroup for coordinated error handling and cancellation.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Worker pool design:**
- Per-request timeout (not global crawl timeout) — each request gets a deadline, then moves on
- Default concurrency: 17 workers
- Graceful shutdown on Ctrl+C — stop new requests, wait for in-flight to finish, show partial results
- Buffered channels as work queue (standard Go pattern)

**URL handling:**
- Same-domain includes subdomains (*.example.com all crawled recursively)
- Query params preserved for deduplication (different query = different page)
- Fragments (#) stripped for deduplication
- Normalize scheme (http/https treated as same domain)
- Trailing slashes stripped for deduplication (/about and /about/ = same URL)
- Non-HTTP schemes (mailto:, tel:, javascript:) skipped silently
- External links: HEAD request first, GET fallback if HEAD fails or is rejected
- Internal links: full GET (need to parse HTML for more links)

**Output during crawl (pre-TUI):**
- Basic logging — print each URL as it's checked during crawl
- Broken link reports include: URL, status code/error, source page where link was found
- Summary at end: "Checked N URLs, found M broken links"
- Healthy link verbosity: Claude's discretion

**Project structure:**
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

</user_constraints>

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| net/http | stdlib | HTTP client with timeout/redirect handling | Standard library, production-ready, context-aware |
| golang.org/x/net/html | v0.50.0 | HTML5-compliant parser and tokenizer | Official Go team package, standards-compliant, 15,626+ projects |
| net/url | stdlib | URL parsing, normalization, relative resolution | Standard library, RFC 3986 compliant |
| context | stdlib | Timeout, cancellation, graceful shutdown | Standard library, idiomatic Go concurrency control |
| sync | stdlib | WaitGroup, Map for concurrency primitives | Standard library, foundational concurrency tools |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| golang.org/x/sync/errgroup | Latest | Coordinated goroutine error handling with context | When you need first-error cancellation across workers |
| flag | stdlib | CLI argument parsing | Simple flags (adequate for --concurrency, url arg) |
| os/signal | stdlib | Signal handling (SIGINT, SIGTERM) | Graceful shutdown on Ctrl+C |
| testing/httptest | stdlib | Mock HTTP servers for testing | All HTTP client tests |
| testing/synctest | stdlib (Go 1.24+) | Testing concurrent code | Experimental, for testing goroutine coordination |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| flag | github.com/spf13/cobra | Cobra adds subcommands, richer help, but overkill for single-command CLI |
| golang.org/x/net/html | github.com/PuerkitoBio/goquery | goquery has jQuery-like API (easier), but adds dependency and less control |
| sync.Map + mutex | Plain map + sync.RWMutex | Map+RWMutex simpler for write-heavy workloads; sync.Map optimized for read-heavy |

**Installation:**

```bash
cd src/
go mod init github.com/lukemcguire/zombiecrawl
go get golang.org/x/net/html
go get golang.org/x/sync/errgroup
```

**Go Version:**
Go 1.26 (released Feb 10, 2026) or Go 1.24.13 (released Feb 4, 2026) are both stable. Recommend Go 1.24.13 for stability unless specific 1.26 features are needed.

---

## Architecture Patterns

### Recommended Project Structure (Claude's Discretion)

```
src/
├── main.go                 # Entry point, CLI parsing, orchestration
├── crawler/
│   ├── crawler.go          # Core crawler logic, BFS coordinator
│   ├── crawler_test.go     # Integration tests with httptest
│   ├── worker.go           # Worker pool implementation
│   └── worker_test.go      # Worker unit tests
├── urlutil/
│   ├── normalize.go        # URL normalization (trailing slash, fragment, scheme)
│   ├── normalize_test.go
│   ├── filter.go           # Same-domain check, scheme filtering
│   └── filter_test.go
├── result/
│   ├── result.go           # Result types (BrokenLink, Summary)
│   └── printer.go          # Output formatting (text reports)
└── go.mod
```

**Alternative (flat):**
For simplicity, could keep all in `src/` root: `main.go`, `crawler.go`, `worker.go`, `urlutil.go`, `result.go`. This is valid for <1000 LOC projects. Choose based on expected growth.

**Rationale for packages:**
- `crawler/`: Core crawl logic isolated from URL utilities and output
- `urlutil/`: URL normalization is complex (see user constraints), deserves dedicated package
- `result/`: Output formatting may grow (Phase 4 adds JSON/XML), separate concern

### Pattern 1: Worker Pool with Buffered Channels

**What:** Fixed number of worker goroutines consuming from a shared buffered channel (work queue).

**When to use:** When you need bounded concurrency to avoid overwhelming resources (CPU, network, target server).

**Example:**

```go
// Source: Adapted from Go by Example + web search findings
// https://gobyexample.com/worker-pools
// https://corentings.dev/blog/go-pattern-worker/

type WorkItem struct {
    URL    string
    Depth  int
    Source string // Where this link was found
}

func (c *Crawler) Start(startURL string, concurrency int) error {
    workQueue := make(chan WorkItem, 100) // Buffered channel as queue
    results := make(chan Result, 100)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Start workers
    g, ctx := errgroup.WithContext(ctx)
    for i := 0; i < concurrency; i++ {
        g.Go(func() error {
            return c.worker(ctx, workQueue, results)
        })
    }

    // Seed initial work
    workQueue <- WorkItem{URL: startURL, Depth: 0, Source: "start"}

    // Coordinate shutdown
    go func() {
        g.Wait()
        close(results)
    }()

    // Collect results
    for result := range results {
        c.processResult(result)
    }

    return g.Wait()
}

func (c *Crawler) worker(ctx context.Context, work <-chan WorkItem, results chan<- Result) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case item, ok := <-work:
            if !ok {
                return nil // Channel closed
            }
            // Check if already visited (deduplication)
            if !c.markVisited(item.URL) {
                continue
            }
            // Make HTTP request with timeout
            result := c.checkURL(ctx, item)
            results <- result
        }
    }
}
```

### Pattern 2: Per-Request Timeout with Context

**What:** Each HTTP request gets its own context with timeout, independent of other requests.

**When to use:** When you want individual requests to timeout without affecting the overall crawl (user's locked decision).

**Example:**

```go
// Source: https://pkg.go.dev/net/http official docs
// https://betterstack.com/community/guides/scaling-go/golang-timeouts/

func (c *Crawler) checkURL(parentCtx context.Context, item WorkItem) Result {
    // Per-request timeout (e.g., 10 seconds)
    ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
    defer cancel()

    req, err := http.NewRequestWithContext(ctx, "GET", item.URL, nil)
    if err != nil {
        return Result{URL: item.URL, Err: err}
    }

    resp, err := c.client.Do(req)
    if err != nil {
        return Result{URL: item.URL, Err: err, Source: item.Source}
    }
    defer resp.Body.Close()

    return Result{
        URL:        item.URL,
        StatusCode: resp.StatusCode,
        Source:     item.Source,
    }
}
```

**Key:** `context.WithTimeout` creates a child context that cancels after duration. HTTP client respects this and aborts the request.

### Pattern 3: Graceful Shutdown with Signal Handling

**What:** Catch SIGINT (Ctrl+C) and SIGTERM, cancel context to stop new work, wait for in-flight goroutines to finish.

**When to use:** Required for user-friendly CLI tools (user's locked decision).

**Example:**

```go
// Source: https://victoriametrics.com/blog/go-graceful-shutdown/
// https://oneuptime.com/blog/post/2026-01-23-go-graceful-shutdown/view

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    crawler := NewCrawler()

    // Run crawler with cancellable context
    errChan := make(chan error, 1)
    go func() {
        errChan <- crawler.Start(ctx, startURL, concurrency)
    }()

    select {
    case <-ctx.Done():
        log.Println("Shutdown signal received, stopping crawler...")
        // Context cancellation propagates to all workers via errgroup
        // Wait for workers to finish current requests
        if err := <-errChan; err != nil && err != context.Canceled {
            log.Fatal(err)
        }
        // Show partial results
        crawler.PrintSummary()
    case err := <-errChan:
        if err != nil {
            log.Fatal(err)
        }
    }
}
```

**Key:** `signal.NotifyContext` returns a context that cancels when signals arrive. Workers check `ctx.Done()` and exit gracefully.

### Pattern 4: URL Normalization for Deduplication

**What:** Normalize URLs before deduplication to treat equivalent URLs as identical.

**When to use:** Always, to prevent re-crawling same page via different URL representations (user's locked decisions).

**Example:**

```go
// Source: User's CONTEXT.md decisions + net/url docs
// https://pkg.go.dev/net/url

func NormalizeURL(rawURL string) (string, error) {
    u, err := url.Parse(rawURL)
    if err != nil {
        return "", err
    }

    // Strip fragment (user decision)
    u.Fragment = ""

    // Normalize scheme to lowercase (url.Parse does this)
    // Treat http/https as same domain (user decision) - handle in domain check

    // Strip trailing slash from path (user decision)
    if len(u.Path) > 1 && strings.HasSuffix(u.Path, "/") {
        u.Path = strings.TrimSuffix(u.Path, "/")
    }

    // Preserve query params (user decision) - url.String() includes them

    return u.String(), nil
}

func SameDomain(base, target *url.URL) bool {
    // Normalize scheme: http/https treated as same domain
    baseHost := strings.ToLower(base.Hostname())
    targetHost := strings.ToLower(target.Hostname())

    // Same-domain includes subdomains (user decision)
    // *.example.com all match
    if baseHost == targetHost {
        return true
    }

    // Check if target is subdomain of base or vice versa
    return strings.HasSuffix(targetHost, "."+baseHost) ||
           strings.HasSuffix(baseHost, "."+targetHost)
}
```

### Pattern 5: HEAD-then-GET for External Links

**What:** Try HEAD request first for external links (bandwidth savings), fall back to GET if HEAD fails or returns 405.

**When to use:** Checking external link validity without parsing (user's locked decision).

**Example:**

```go
// Source: User's CONTEXT.md + https://pkg.go.dev/net/http

func (c *Crawler) checkExternalLink(ctx context.Context, url string) Result {
    // Try HEAD first
    headReq, _ := http.NewRequestWithContext(ctx, "HEAD", url, nil)
    headResp, err := c.client.Do(headReq)

    if err == nil {
        defer headResp.Body.Close()
        // HEAD succeeded, use its status
        if headResp.StatusCode != 405 { // Method Not Allowed
            return Result{URL: url, StatusCode: headResp.StatusCode}
        }
        // 405 means server doesn't support HEAD, fall back to GET
    }

    // HEAD failed or returned 405, try GET
    getReq, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    getResp, err := c.client.Do(getReq)
    if err != nil {
        return Result{URL: url, Err: err}
    }
    defer getResp.Body.Close()

    return Result{URL: url, StatusCode: getResp.StatusCode}
}
```

### Pattern 6: Relative URL Resolution

**What:** Convert relative hrefs to absolute URLs using the current page's URL as base.

**When to use:** Always when extracting links from HTML (RFC 3986 compliance).

**Example:**

```go
// Source: https://pkg.go.dev/net/url

func ResolveReference(baseURL, href string) (string, error) {
    base, err := url.Parse(baseURL)
    if err != nil {
        return "", err
    }

    ref, err := url.Parse(href)
    if err != nil {
        return "", err
    }

    // ResolveReference handles relative URLs per RFC 3986
    // ../foo, /bar, //host/path all resolved correctly
    resolved := base.ResolveReference(ref)
    return resolved.String(), nil
}
```

**Note:** This handles `<base href>` tag indirectly: if you extract the base href from HTML, use it as the base URL for subsequent link resolution.

### Pattern 7: Concurrent Map Access with sync.Map

**What:** Thread-safe map for tracking visited URLs across goroutines.

**When to use:** Read-heavy workloads where URLs are checked frequently but written once (deduplication).

**Example:**

```go
// Source: https://victoriametrics.com/blog/go-sync-map/
// https://oneuptime.com/blog/post/2026-01-25-sync-map-vs-mutex-maps-go/view

type Crawler struct {
    visited sync.Map // map[string]bool
}

// markVisited returns true if this is the first visit (and marks it)
func (c *Crawler) markVisited(url string) bool {
    _, loaded := c.visited.LoadOrStore(url, true)
    return !loaded // true if we just stored it (first visit)
}
```

**Alternative:** For write-heavy or complex operations, use `map[string]bool` with `sync.RWMutex`. sync.Map is optimized for read-heavy, write-once patterns.

### Anti-Patterns to Avoid

- **Global timeout on entire crawl:** User wants per-request timeout so slow servers don't block entire crawl
- **Unbounded goroutines:** Spawning goroutine per URL causes resource exhaustion; use worker pool
- **Plain map with goroutines:** Data race, will panic; use sync.Map or mutex
- **Blocking channel sends without select:** Causes goroutine leaks if receiver stops; always select with ctx.Done()
- **Ignoring redirect chains:** 3xx are valid per user requirements; don't treat as broken

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTML parsing | Regex or string matching for tags | golang.org/x/net/html | HTML is context-sensitive, not regular; regex misses edge cases (malformed HTML, CDATA, comments); parser handles all HTML5 spec |
| URL normalization | Custom string manipulation for paths | net/url.Parse + url.URL methods | RFC 3986 is complex (scheme, authority, path, query encoding); url package handles all cases correctly |
| HTTP client | Raw TCP sockets + HTTP protocol | net/http.Client with context | Connection pooling, redirect handling, timeout, compression all built-in; hand-rolled version misses edge cases |
| Worker pool | Manual goroutine spawning + sync primitives | Buffered channel + errgroup pattern | Easy to leak goroutines or deadlock; established pattern is well-tested and idiomatic |
| Graceful shutdown | Manual signal handling + custom coordination | signal.NotifyContext + errgroup.WithContext | Context cancellation propagates correctly; manual coordination error-prone |

**Key insight:** Web crawling involves many edge cases (malformed HTML, redirect loops, encoding issues, connection failures). Standard library and official extensions have handled these for years. Custom implementations will rediscover these edge cases the hard way.

---

## Common Pitfalls

### Pitfall 1: Goroutine Leaks from Blocking Channel Operations

**What goes wrong:** Workers block forever on channel send/receive when context is canceled but channel operation doesn't check ctx.Done().

**Why it happens:** Channel operations (send/receive) block until the other side is ready. If context cancels and you're blocked on a channel, you never check ctx.Done() and goroutine never exits.

**How to avoid:** Always use select with ctx.Done() when doing channel operations:

```go
// BAD: blocks forever if ctx cancels while waiting for work
item := <-workQueue

// GOOD: checks for cancellation
select {
case <-ctx.Done():
    return ctx.Err()
case item := <-workQueue:
    // process item
}
```

**Warning signs:** Tests with `-race` flag hang, goroutine count increases without bound, program doesn't exit cleanly on Ctrl+C.

**Detection:** Use `goleak` package in tests to detect leaked goroutines.

### Pitfall 2: Concurrent Map Access (Data Race)

**What goes wrong:** Multiple goroutines read/write plain `map[string]bool` for visited tracking, causing data race and runtime panic.

**Why it happens:** Go's map is not thread-safe. Concurrent reads are safe, but concurrent writes or read+write cause undefined behavior.

**How to avoid:** Use sync.Map (read-optimized) or map + sync.RWMutex (write-heavy):

```go
// BAD: data race
type Crawler struct {
    visited map[string]bool // NOT SAFE
}

// GOOD: sync.Map for read-heavy
type Crawler struct {
    visited sync.Map
}

// GOOD: mutex for write-heavy or complex operations
type Crawler struct {
    visited map[string]bool
    mu      sync.RWMutex
}
```

**Warning signs:** `go test -race` reports data race, program panics with "concurrent map writes", non-deterministic behavior.

**Detection:** Always run tests with `go test -race` flag. Race detector catches this immediately.

### Pitfall 3: Infinite Crawl Loops (Self-Referencing Links)

**What goes wrong:** Page links to itself or pages link in a cycle (A→B→C→A), causing crawler to loop forever.

**Why it happens:** Websites often have navigation links, breadcrumbs, or pagination that create cycles. Without deduplication, crawler revisits same pages.

**How to avoid:** Mark URLs as visited BEFORE crawling them (not after), use normalized URLs for deduplication:

```go
// BAD: mark after crawling (allows duplicates to enter queue)
result := crawl(url)
visited[url] = true

// GOOD: mark before crawling (prevents duplicates from entering queue)
if visited[url] {
    return // Skip already visited
}
visited[url] = true
result := crawl(url)
```

**Warning signs:** Crawler never finishes, same URLs appear in logs repeatedly, visited count far exceeds expected page count.

**Detection:** Log all visited URLs and check for duplicates. Monitor crawl progress (pages/second should be non-zero).

### Pitfall 4: Not Handling Redirect Chains

**What goes wrong:** Treating 3xx redirects as errors instead of following them, or following infinite redirect loops.

**Why it happens:** HTTP client needs explicit configuration for how to handle redirects.

**How to avoid:** Use default http.Client behavior (follows up to 10 redirects), or customize with CheckRedirect:

```go
// Default client follows up to 10 redirects automatically
client := &http.Client{
    Timeout: 30 * time.Second,
    // CheckRedirect: nil means default (follow up to 10)
}

// Custom: stop after 3 redirects
client := &http.Client{
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        if len(via) >= 3 {
            return fmt.Errorf("stopped after 3 redirects")
        }
        return nil
    },
}
```

**User requirement:** 3xx redirects are valid (not broken). Default client behavior (follow redirects) satisfies this. Final status code after redirects determines if link is broken.

**Warning signs:** Valid pages reported as broken due to redirects, infinite loops on redirect chains.

### Pitfall 5: Ignoring HTTP Client Defaults (No Timeout)

**What goes wrong:** Using `http.DefaultClient` or creating `http.Client{}` without setting timeout, causing requests to hang forever on unresponsive servers.

**Why it happens:** Go's HTTP client has NO timeout by default (`Timeout: 0` means infinite).

**How to avoid:** Always create custom client with timeout:

```go
// BAD: no timeout, can hang forever
client := &http.Client{}

// GOOD: per-client timeout (applies to all requests)
client := &http.Client{
    Timeout: 30 * time.Second,
}

// BETTER: per-request timeout with context (user's decision)
ctx, cancel := context.WithTimeout(parentCtx, 10*time.Second)
defer cancel()
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)
```

**User requirement:** Per-request timeout (not global). Use context.WithTimeout on each request.

**Warning signs:** Crawler hangs on certain URLs, goroutines stuck in HTTP reads, program doesn't respond to Ctrl+C.

### Pitfall 6: Parsing URLs Without Validation

**What goes wrong:** Extracting href attributes and using them directly without validating scheme, causing crawler to attempt invalid URLs (mailto:, javascript:, tel:).

**Why it happens:** HTML allows any URI scheme in href. Not all are HTTP URLs.

**How to avoid:** Validate scheme after parsing, skip non-HTTP(S) URLs:

```go
u, err := url.Parse(href)
if err != nil {
    return nil, err // Invalid URL
}

// Skip non-HTTP schemes (user decision)
if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "" {
    return nil, nil // Skip silently
}

// Relative URL (no scheme) is valid, will be resolved against base
```

**User requirement:** Non-HTTP schemes (mailto:, tel:, javascript:) skipped silently.

**Warning signs:** Errors on invalid URLs, crawler attempts to fetch `mailto:` or `javascript:` URLs.

### Pitfall 7: Not Closing Response Bodies

**What goes wrong:** Forgetting `defer resp.Body.Close()` causes connection leaks, eventually exhausting connection pool.

**Why it happens:** HTTP client connection pool reuses connections only if body is fully read and closed.

**How to avoid:** Always defer close immediately after checking error:

```go
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close() // ALWAYS defer this immediately

// Now process response
```

**Warning signs:** Crawler slows down over time, "too many open files" errors, connection pool exhausted.

**Detection:** Monitor open file descriptors (`lsof` on Linux/Mac). Run extended tests that crawl many pages.

### Pitfall 8: Trailing Slash Inconsistency

**What goes wrong:** Treating `/about` and `/about/` as different URLs causes duplicate crawling and incorrect deduplication.

**Why it happens:** Servers may or may not treat these as equivalent. URL string comparison treats them as different.

**How to avoid:** Normalize by stripping trailing slashes (user's decision):

```go
// Normalize before deduplication
path := u.Path
if len(path) > 1 && strings.HasSuffix(path, "/") {
    u.Path = strings.TrimSuffix(path, "/")
}
normalized := u.String()
```

**User requirement:** Trailing slashes stripped for deduplication.

**Warning signs:** Duplicate URLs in visited set with only trailing slash difference, same page crawled twice.

---

## Code Examples

Verified patterns from official sources:

### Extracting Links from HTML

```go
// Source: https://pkg.go.dev/golang.org/x/net/html

import (
    "golang.org/x/net/html"
    "io"
)

func ExtractLinks(body io.Reader, baseURL string) ([]string, error) {
    doc, err := html.Parse(body)
    if err != nil {
        return nil, err
    }

    var links []string
    var traverse func(*html.Node)
    traverse = func(n *html.Node) {
        if n.Type == html.ElementNode && n.Data == "a" {
            for _, attr := range n.Attr {
                if attr.Key == "href" {
                    // Resolve relative URL
                    absURL, err := ResolveReference(baseURL, attr.Val)
                    if err == nil {
                        links = append(links, absURL)
                    }
                    break
                }
            }
        }
        for c := n.FirstChild; c != nil; c = c.NextSibling {
            traverse(c)
        }
    }
    traverse(doc)
    return links, nil
}
```

**Alternative using Descendants iterator (Go 1.24+):**

```go
func ExtractLinks(body io.Reader, baseURL string) ([]string, error) {
    doc, err := html.Parse(body)
    if err != nil {
        return nil, err
    }

    var links []string
    for n := range doc.Descendants() {
        if n.Type == html.ElementNode && n.Data == "a" {
            for _, attr := range n.Attr {
                if attr.Key == "href" {
                    absURL, err := ResolveReference(baseURL, attr.Val)
                    if err == nil {
                        links = append(links, absURL)
                    }
                    break
                }
            }
        }
    }
    return links, nil
}
```

### Testing with httptest

```go
// Source: https://pkg.go.dev/net/http/httptest

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestCrawler_BrokenLink(t *testing.T) {
    // Create mock server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/broken" {
            w.WriteHeader(http.StatusNotFound)
            return
        }
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`<html><a href="/broken">Link</a></html>`))
    }))
    defer server.Close()

    // Test crawler against mock server
    crawler := NewCrawler()
    results, err := crawler.Start(server.URL, 1)
    if err != nil {
        t.Fatal(err)
    }

    // Verify broken link detected
    if len(results.BrokenLinks) != 1 {
        t.Errorf("Expected 1 broken link, got %d", len(results.BrokenLinks))
    }
}
```

### Table-Driven Tests for URL Normalization

```go
// Source: Go testing best practices

func TestNormalizeURL(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "strip fragment",
            input:    "https://example.com/page#section",
            expected: "https://example.com/page",
        },
        {
            name:     "strip trailing slash",
            input:    "https://example.com/about/",
            expected: "https://example.com/about",
        },
        {
            name:     "preserve query params",
            input:    "https://example.com/search?q=test",
            expected: "https://example.com/search?q=test",
        },
        {
            name:     "root path keeps slash",
            input:    "https://example.com/",
            expected: "https://example.com/",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := NormalizeURL(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("NormalizeURL() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.expected {
                t.Errorf("NormalizeURL() = %v, want %v", got, tt.expected)
            }
        })
    }
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| context.WithCancel + manual timeout | context.WithTimeout | Go 1.7 (2016) | Simpler timeout handling, less boilerplate |
| Manual signal handling with os.Signal | signal.NotifyContext | Go 1.16 (2021) | Context cancellation on signals, cleaner shutdown |
| sync.WaitGroup for coordinated errors | errgroup.WithContext | Available since 2017 | First-error cancellation, cleaner error propagation |
| html.Tokenizer for all parsing | html.Parse with Descendants() | Go 1.24 (2025) | Easier tree traversal with iterator methods |
| Request.Cancel field | context-based cancellation | Go 1.7 (2016) | Request.Cancel deprecated, context is standard |
| testing concurrent code manually | testing/synctest package | Go 1.24 (2025) | Experimental, aids testing goroutine coordination |

**Deprecated/outdated:**
- **Request.Cancel:** Deprecated since Go 1.7, use context instead
- **Manual goroutine coordination:** errgroup is idiomatic for coordinated goroutines with errors
- **Ignoring signal.NotifyContext:** Older tutorials show manual signal.Notify + channels, NotifyContext is cleaner

---

## Open Questions

### 1. Optimal per-request timeout duration

**What we know:** Context-based timeouts are user's decision. Typical web requests complete in 1-5 seconds.

**What's unclear:** User hasn't specified exact duration (marked as Claude's discretion).

**Recommendation:** Start with 10 seconds per request. This allows for slow servers while preventing indefinite hangs. Make it configurable via flag if needed. Can be tuned based on observed crawl behavior.

### 2. Channel buffer sizes

**What we know:** Buffered channels prevent sender goroutines from blocking. User decided buffered channels for work queue.

**What's unclear:** Exact buffer size (marked as Claude's discretion).

**Recommendation:** Buffer size = 2-3x worker count is a good heuristic. For 17 workers, buffer of 50-100. Larger buffers reduce blocking but use more memory. Start with 100, profile if memory is concern.

### 3. Healthy link logging verbosity

**What we know:** User wants basic logging during crawl, broken links reported, summary at end. Healthy link verbosity is Claude's discretion.

**What's unclear:** Should every healthy link be logged, or only broken ones?

**Recommendation:** Log all checked URLs with status (user said "print each URL as it's checked"), use different formatting for broken vs healthy (e.g., color, prefix). Allows user to see progress without overwhelming output.

### 4. Package organization

**What we know:** User left flat vs package-based organization to Claude's discretion.

**What's unclear:** Expected code size and future growth.

**Recommendation:** Start with package-based structure (crawler/, urlutil/, result/). URL normalization alone is complex enough (see user constraints) to justify dedicated package. If code stays under 500 LOC, could refactor to flat. Packages make testing easier (smaller unit test surface per package).

### 5. BFS vs DFS crawl order

**What we know:** User requirements say "BFS" but don't strictly require breadth-first semantics for correctness.

**What's unclear:** Whether strict BFS order matters, or if any traversal order is acceptable.

**Recommendation:** Worker pool naturally does BFS if work queue is FIFO and new links are appended to queue. But with concurrent workers, strict BFS ordering is not guaranteed (workers complete in non-deterministic order). This is acceptable for broken link detection (order doesn't affect correctness). If strict BFS is needed for future features (e.g., depth limiting), would need coordination.

---

## Sources

### Primary (HIGH confidence)

- [net/http package - Go Packages](https://pkg.go.dev/net/http) - HTTP client, timeout, redirect handling
- [golang.org/x/net/html package - Go Packages](https://pkg.go.dev/golang.org/x/net/html) - HTML parsing API and status
- [net/url package - Go Packages](https://pkg.go.dev/net/url) - URL parsing, normalization, ResolveReference
- [context package - Go Packages](https://pkg.go.dev/context) - Context API for cancellation
- [errgroup package - golang.org/x/sync/errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) - Coordinated goroutine error handling
- [synctest package - testing/synctest](https://pkg.go.dev/testing/synctest) - Testing concurrent code (Go 1.24+)
- [Go 1.24 Release Notes](https://go.dev/doc/go1.24) - Latest stable version features
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26) - Newest version (released Feb 10, 2026)

### Secondary (MEDIUM confidence)

- [The complete guide to Go net/http timeouts - Cloudflare](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) - Timeout patterns
- [Graceful Shutdown in Go: Practical Patterns - VictoriaMetrics](https://victoriametrics.com/blog/go-graceful-shutdown/) - Signal handling
- [How to Implement Graceful Shutdown in Go - OneUpTime](https://oneuptime.com/blog/post/2026-01-23-go-graceful-shutdown/view) - Context cancellation (2026)
- [Mastering the Worker Pool Pattern in Go - Corentin GS](https://corentings.dev/blog/go-pattern-worker/) - Worker pool architecture
- [How to Implement Worker Pools in Go - OneUpTime](https://oneuptime.com/blog/post/2026-01-07-go-worker-pools/view) - Worker pool implementation (2026)
- [Understanding and Preventing Goroutine Leaks in Go - Medium](https://medium.com/@srajsonu/understanding-and-preventing-goroutine-leaks-in-go-623cac542954) - Leak prevention
- [Go sync.Map: The Right Tool for the Right Job - VictoriaMetrics](https://victoriametrics.com/blog/go-sync-map/) - sync.Map vs mutex
- [How to Choose Between sync.Map and Maps with Mutex in Go - OneUpTime](https://oneuptime.com/blog/post/2026-01-25-sync-map-vs-mutex-maps-go/view) - Concurrent map patterns (2026)
- [Testing concurrent code with testing/synctest - Go Blog](https://go.dev/blog/synctest) - Concurrent testing (Go 1.24)
- [Parallel Table-Driven Tests in Go - Medium](https://medium.com/@rosgluk/parallel-table-driven-tests-in-go-d06d53a02b1a) - Testing patterns (2026)

### Tertiary (LOW confidence, requires validation)

- [Exercise: Web Crawler - Go Tour](https://go.dev/tour/concurrency/10) - Educational example (simplified)
- [Organizing a Go module - Go.dev](https://go.dev/doc/modules/layout) - Module structure recommendations
- [Standard Go Project Layout - GitHub](https://github.com/golang-standards/project-layout) - Community project structure (controversial)

---

## Metadata

**Confidence breakdown:**

- **Standard stack:** HIGH - All official Go packages with stable APIs, verified from pkg.go.dev
- **Architecture:** HIGH - Worker pool, context, errgroup are established patterns, verified from official sources and 2026 articles
- **Pitfalls:** HIGH - Goroutine leaks, concurrent map access, infinite loops are well-documented issues with known solutions
- **Open questions:** MEDIUM - Recommendations based on best practices, but user's specific needs may differ

**Research date:** 2026-02-13

**Valid until:** ~2026-03-13 (30 days, Go ecosystem is stable, standard library changes slowly)

**Notes:**

- User's CONTEXT.md provided extensive locked decisions, significantly constraining research scope
- All locked decisions are technically sound and align with Go best practices
- Claude's discretion areas (package structure, timeout durations, buffer sizes) have reasonable defaults provided
- No conflicts found between user decisions and Go ecosystem standards
