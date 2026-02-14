# Domain Pitfalls: Go Web Crawler / Dead Link Checker

**Domain:** Go CLI dead link checker with concurrent crawling
**Researched:** 2026-02-13
**Overall Confidence:** HIGH

## Critical Pitfalls

Mistakes that cause rewrites, crashes, or major architectural problems.

### Pitfall 1: Goroutine Leaks from Blocked Channels

**What goes wrong:** Goroutines writing to channels without readers block indefinitely, consuming memory. In crawlers that spawn a goroutine per URL or per worker, blocked channel writers accumulate until memory exhaustion crashes the application.

**Why it happens:**
- Unbuffered channels with no reader
- Buffered channels that fill up while the main goroutine waits on `wg.Wait()`
- Missing channel close operations
- Parent goroutines blocked before consuming child output

**Consequences:**
- Progressive memory leak (stack space + heap allocations)
- Eventual OOM crash
- CPU exhaustion from thousands of blocked goroutines
- Particularly insidious in long-running crawls (degrades over hours)

**Prevention:**
```go
// BAD: Workers block when results buffer fills and main is stuck on wg.Wait()
results := make(chan Result, 10)
for i := 0; i < workers; i++ {
    wg.Add(1)
    go worker(jobs, results, &wg)
}
wg.Wait()  // DEADLOCK: workers stuck sending to full buffer
close(results)

// GOOD: Separate goroutine handles closing after workers finish
results := make(chan Result, 10)
for i := 0; i < workers; i++ {
    wg.Add(1)
    go worker(jobs, results, &wg)
}
go func() {
    wg.Wait()
    close(results)
}()
// Main can now drain results without blocking
```

**Detection:**
- Memory usage grows linearly with URLs crawled
- `runtime.NumGoroutine()` increases without bound
- pprof goroutine profile shows accumulation
- Go 1.24+ synctest package / Go 1.26+ goroutineleak profile

**Phase mapping:** Phase 1 (worker pool setup) - get this right initially or face rewrites

**Sources:**
- [Understanding and Debugging Goroutine Leaks in Go Web Servers](https://leapcell.io/blog/understanding-and-debugging-goroutine-leaks-in-go-web-servers)
- [Goroutine Leaks - The Forgotten Sender](https://www.ardanlabs.com/blog/2018/11/goroutine-leaks-the-forgotten-sender.html)
- [Mastering the Worker Pool Pattern in Go](https://corentings.dev/blog/go-pattern-worker/)

---

### Pitfall 2: HTTP Response Body Not Closed

**What goes wrong:** Failing to close `response.Body` leaks file descriptors and prevents HTTP connection reuse. Connections remain in CLOSE_WAIT state indefinitely until descriptor limit is reached.

**Why it happens:**
- Non-obvious Go HTTP client requirement (must close even if body not read)
- Early returns on error before `defer resp.Body.Close()`
- Assumption that connection pool handles cleanup
- Common in error paths where response validity isn't checked

**Consequences:**
- File descriptor exhaustion (process can't open new connections)
- HTTP connection pool unable to reuse keep-alive connections
- New TCP connection created for every request (massive overhead)
- Crawl slows to a crawl or fails entirely after ~1024 URLs (typical FD limit)

**Prevention:**
```go
// BAD: Body leaked if request fails or response ignored
resp, err := client.Do(req)
if resp.StatusCode != 200 {
    return fmt.Errorf("bad status")  // LEAK: body not closed
}

// GOOD: Always close, even on error
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()  // Close ASAP regardless of status

// Also drain body for connection reuse
io.Copy(io.Discard, resp.Body)  // Required for HTTP/1.x keep-alive
```

**Detection:**
- `lsof | grep CLOSE_WAIT` shows stuck connections
- Crawler performance degrades over time
- "too many open files" errors
- Static analysis with `bodyclose` linter

**Phase mapping:** Phase 1 (HTTP client setup) - must be correct from start

**Sources:**
- [TIL: Go Response Body MUST be closed, even if you don't read it](https://manishrjain.com/must-close-golang-http-response)
- [Always close the response body! Running out of file descriptors in Golang](https://www.j4mcs.dev/posts/golang-response-body/)
- [Go HTTP client connection pooling](https://davidbacisin.com/writing/golang-http-connection-pools-1)

---

### Pitfall 3: Unbounded Visited URL Map Growth

**What goes wrong:** Using a simple `map[string]bool` to track visited URLs grows without bound. Large crawls (10K+ pages) consume gigabytes of memory, and deleting entries doesn't free memory (Go map internals retain allocated buckets).

**Why it happens:**
- Natural pattern for deduplication (use map to track visited)
- Assumption that delete() frees memory
- Go map implementation optimizes for throughput, not memory reclamation
- Most crawlers never clear the visited map

**Consequences:**
- Multi-gigabyte memory usage for large sites
- OOM kills on memory-constrained environments
- Map iteration becomes slower as map grows
- Memory not reclaimed even when crawl completes

**Prevention:**
```go
// BAD: Unbounded growth
visited := make(map[string]bool)
// ... after crawling 100K URLs, map holds gigabytes

// OPTION 1: LRU cache with bounded size
cache := lru.New(10000)  // Limit to 10K most recent URLs

// OPTION 2: Periodic map reinitialization
if len(visited) > threshold {
    // Save essential entries, recreate map
    visited = make(map[string]bool)
}

// OPTION 3: Bloom filter for large crawls (allows false positives)
filter := bloom.New(1000000, 5)  // Fixed memory footprint
```

**Detection:**
- Memory usage correlates with total URLs discovered
- pprof heap profile shows map as top allocator
- `len(visited)` grows indefinitely
- Slow map operations as size increases

**Phase mapping:** Phase 1 (URL deduplication) - choose right data structure initially

**Sources:**
- [Understanding and Preventing Memory Leak in Go Maps](https://medium.com/@tedious/go-map-memory-leaks-why-deleting-elements-doesnt-always-free-memory-670a81ad3be9)
- [How to avoid crawling duplicate URLs at Google scale](https://blog.bytebytego.com/p/how-to-avoid-crawling-duplicate-urls)

---

### Pitfall 4: Missing Context Timeout and Cancellation

**What goes wrong:** HTTP requests without context timeouts hang indefinitely on slow/unresponsive servers. Crawler continues spawning new workers while old ones block forever on dead connections.

**Why it happens:**
- Using default http.Client (no timeout)
- Not propagating context through worker goroutines
- Assuming server timeouts are sufficient
- No global crawl timeout mechanism

**Consequences:**
- Goroutines accumulate waiting for responses that never complete
- Crawl never finishes (hangs on last few dead links)
- User cannot gracefully cancel crawl (Ctrl+C leaves goroutines running)
- Resource exhaustion from unbounded waiting goroutines

**Prevention:**
```go
// BAD: No timeout, hangs forever on dead servers
resp, err := http.Get(url)

// GOOD: Per-request timeout
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)

// BETTER: Propagate context from main, allow graceful shutdown
func crawler(ctx context.Context, ...) {
    for url := range urls {
        select {
        case <-ctx.Done():
            return  // Cancelled, cleanup and exit
        default:
            req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
            // ...
        }
    }
}
```

**Detection:**
- Crawler never completes
- `runtime.NumGoroutine()` grows unbounded
- pprof shows goroutines stuck in network read
- Ctrl+C doesn't stop crawl

**Phase mapping:** Phase 1 (HTTP client + worker pool) - critical for crawl stability

**Sources:**
- [Golang Context - Cancellation, Timeout and Propagation](https://golangbot.com/context-timeout-cancellation/)
- [Mastering Go Contexts: A Deep Dive Into Cancellation, Timeouts, and Request-Scoped Values](https://medium.com/@harshithgowdakt/mastering-go-contexts-a-deep-dive-into-cancellation-timeouts-and-request-scoped-values-392122ad0a47)

---

## Moderate Pitfalls

Issues that degrade quality or require refactoring but don't cause catastrophic failure.

### Pitfall 5: Incorrect Relative URL Resolution

**What goes wrong:** Relative URLs (e.g., `../page.html`, `/about`) are not resolved against the correct base URL, causing incorrect absolute URLs that result in 404s or crawling the wrong domain.

**Why it happens:**
- Using string concatenation instead of `url.Parse` + `url.ResolveReference`
- Not extracting base URL from current page
- Ignoring `<base href="">` tags in HTML
- Assuming all hrefs are absolute

**Consequences:**
- Missing valid links (false positives for "dead links")
- Attempting to crawl external domains unintentionally
- Incorrect deduplication (same page seen as different URLs)
- Users lose trust in accuracy

**Prevention:**
```go
// BAD: String concatenation fails on relative URLs
absoluteURL := baseURL + href  // WRONG for "../page.html"

// GOOD: Proper URL resolution
base, _ := url.Parse(currentPageURL)
ref, _ := url.Parse(href)
absolute := base.ResolveReference(ref).String()

// BETTER: Handle <base href=""> tags from HTML
// Parse HTML, extract base tag if present, use that as baseURL
```

**Detection:**
- 404s on URLs that work in browser
- Crawling external domains unintentionally
- Deduplication failing (same page crawled multiple times)

**Phase mapping:** Phase 2 (link extraction) - test thoroughly with relative URLs

**Sources:**
- [Resolving relative references to a URL](https://developer.mozilla.org/en-US/docs/Web/API/URL_API/Resolving_relative_references)
- [How to Build a Web Crawler With Go](https://www.zenrows.com/blog/golang-web-crawler)

---

### Pitfall 6: Poor HTTP Client Configuration for Crawling

**What goes wrong:** Using default `http.DefaultClient` results in poor performance (only 2 connections per host) and unbounded redirects, leading to slow crawls and infinite redirect loops.

**Why it happens:**
- Not knowing `DefaultMaxIdleConnsPerHost = 2`
- Assuming defaults are optimized for crawling
- Not setting redirect limits
- Missing custom User-Agent

**Consequences:**
- Only 2 concurrent requests per domain (10x slower than necessary)
- Infinite redirect loops crash crawler
- Blocked by servers (default User-Agent often blocked)
- Connection pool exhaustion on multi-domain crawls

**Prevention:**
```go
// BAD: Default client
client := &http.Client{}  // Only 2 conns/host, no redirect limit

// GOOD: Optimized for crawling
client := &http.Client{
    Timeout: 10 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,  // Total pool size
        MaxIdleConnsPerHost: 20,   // Per-domain connections
        IdleConnTimeout:     90 * time.Second,
    },
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        if len(via) >= 10 {
            return errors.New("too many redirects")
        }
        return nil
    },
}
// Set custom User-Agent per request
req.Header.Set("User-Agent", "zombiecrawl/1.0 (contact@example.com)")
```

**Detection:**
- Crawl much slower than expected
- Only 2 requests at a time per domain
- Crawler hangs on redirect loops
- 403 Forbidden from servers

**Phase mapping:** Phase 1 (HTTP client setup) - optimize early

**Sources:**
- [Tuning the Go HTTP client settings for load testing](http://tleyden.github.io/blog/2016/11/21/tuning-the-go-http-client-library-for-load-testing/)
- [How to Use the HTTP Client in GO To Enhance Performance](https://www.loginradius.com/blog/engineering/tune-the-go-http-client-for-high-performance/)

---

### Pitfall 7: Naive Retry Logic Without Backoff

**What goes wrong:** Fixed retry count with no delay hammers failing servers, gets IP banned, and wastes time on persistent failures (e.g., DNS failures, certificate errors).

**Why it happens:**
- Implementing simple `for i := 0; i < 3; i++` retry loop
- Not distinguishing retryable vs non-retryable errors
- Assuming all failures are transient
- Not implementing exponential backoff

**Consequences:**
- IP banned for hammering servers (429 rate limits → 403 ban)
- Wasting time retrying non-retryable errors (DNS, 404, SSL)
- Multiple clients retrying simultaneously (thundering herd)
- Crawl takes 10x longer due to unnecessary retries

**Prevention:**
```go
// BAD: Fixed retry, no delay, all errors
for i := 0; i < 3; i++ {
    resp, err := client.Get(url)
    if err == nil { break }
}

// GOOD: Exponential backoff, error classification
retryableStatusCodes := []int{429, 502, 503, 504}
backoff := 1 * time.Second
for attempt := 0; attempt < 3; attempt++ {
    resp, err := client.Get(url)
    if err == nil && !isRetryable(resp.StatusCode) {
        return resp, nil  // Success or non-retryable error
    }
    time.Sleep(backoff + time.Duration(rand.Intn(1000))*time.Millisecond)  // Jitter
    backoff *= 2  // Exponential
}

// Use library like hashicorp/go-retryablehttp
```

**Detection:**
- Many 429 "Too Many Requests" responses
- IP bans (403 Forbidden)
- Crawl takes hours on small sites
- Logs show retrying 404s and DNS failures

**Phase mapping:** Phase 2 (error handling) - add before large-scale testing

**Sources:**
- [How I improved consistency and performance in a Go crawler with retry logics](https://blog.maxgio.me/posts/improving-consistency-performance-go-crawler-retry-logics-http-client-tuning/)
- [How to Implement Retry Logic in Go with Exponential Backoff](https://oneuptime.com/blog/post/2026-01-07-go-retry-exponential-backoff/view)

---

### Pitfall 8: URL Deduplication Doesn't Normalize

**What goes wrong:** URLs with different string representations but same resource (e.g., `?a=1&b=2` vs `?b=2&a=1`, trailing slashes, fragments) are treated as separate, causing duplicate crawling.

**Why it happens:**
- Using raw URL string as map key
- Not sorting query parameters
- Not handling fragments consistently
- Not normalizing trailing slashes

**Consequences:**
- Same page crawled multiple times (slower, more requests)
- Deduplication map larger than necessary (memory waste)
- Difficult to track "what we've actually crawled"
- Poor user experience (progress bar inaccurate)

**Prevention:**
```go
// BAD: Raw URL as key
visited[rawURL] = true  // "/page?a=1&b=2" != "/page?b=2&a=1"

// GOOD: Normalize before deduplication
func normalizeURL(rawURL string) string {
    u, _ := url.Parse(rawURL)
    // Remove fragment (usually doesn't change server response)
    u.Fragment = ""
    // Sort query parameters
    q := u.Query()
    u.RawQuery = q.Encode()  // Encodes in sorted order
    // Normalize trailing slash (site-dependent)
    u.Path = strings.TrimSuffix(u.Path, "/")
    return u.String()
}
visited[normalizeURL(rawURL)] = true
```

**Detection:**
- Same page appears multiple times in results
- `visited` map much larger than actual page count
- URLs differ only in parameter order or fragment

**Phase mapping:** Phase 1 (deduplication) - implement during URL handling

**Sources:**
- [How to avoid crawling duplicate URLs at Google scale](https://blog.bytebytego.com/p/how-to-avoid-crawling-duplicate-urls)
- [Go URL parsing edge cases](https://pkg.go.dev/net/url)

---

### Pitfall 9: HTML Parsing with Regex Instead of Parser

**What goes wrong:** Using regex to extract `<a href="">` from HTML breaks on nested tags, attributes in different order, malformed HTML, and multi-line tags.

**Why it happens:**
- Regex seems simpler than learning HTML parser
- Works on basic examples
- Not aware of `golang.org/x/net/html` or goquery
- Underestimating HTML complexity

**Consequences:**
- Missing links (false negatives)
- Extracting garbage as links (false positives)
- Crashes on malformed HTML
- Can't handle nested links, base tags, etc.

**Prevention:**
```go
// BAD: Regex nightmare
re := regexp.MustCompile(`<a\s+href="([^"]+)"`)
matches := re.FindAllStringSubmatch(html, -1)

// GOOD: Use golang.org/x/net/html
doc, _ := html.Parse(resp.Body)
var links []string
var extract func(*html.Node)
extract = func(n *html.Node) {
    if n.Type == html.ElementNode && n.Data == "a" {
        for _, attr := range n.Attr {
            if attr.Key == "href" {
                links = append(links, attr.Val)
            }
        }
    }
    for c := n.FirstChild; c != nil; c = c.NextSibling {
        extract(c)
    }
}
extract(doc)

// BETTER: Use goquery (jQuery-like API)
doc, _ := goquery.NewDocumentFromReader(resp.Body)
doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
    href, _ := s.Attr("href")
    links = append(links, href)
})
```

**Detection:**
- Missing obvious links when tested manually
- Extracting text that isn't a link
- Crashes on real-world websites
- Not handling relative URLs correctly

**Phase mapping:** Phase 2 (link extraction) - use proper parser from start

**Sources:**
- [Web Crawler - Go Concurrency](https://medium.com/@wu.victor.95/web-crawler-go-concurrency-196e9c0fec8)
- [What is the best way to parse HTML in Go?](https://webscraping.ai/faq/go/what-is-the-best-way-to-parse-html-in-go)

---

### Pitfall 10: Ignoring Robots.txt and Crawl-Delay

**What goes wrong:** Not respecting `robots.txt` or `Crawl-delay` directives causes servers to rate-limit or ban the crawler IP, and violates web etiquette.

**Why it happens:**
- Learning project, seems unnecessary
- Not aware of robots.txt importance
- Assuming small crawls don't matter
- Wanting to maximize speed

**Consequences:**
- IP banned (403 Forbidden)
- Ethical issues (ignoring site owner requests)
- Server overload on small sites
- Legal risk for aggressive crawling
- Poor reputation for tool

**Prevention:**
```go
// Use library like temoto/robotstxt
robots, _ := robotstxt.FromURL("https://example.com/robots.txt")
if robots.TestAgent("/path", "zombiecrawl") {
    // Allowed to crawl
    delay := robots.CrawlDelay("zombiecrawl")
    time.Sleep(delay)  // Respect crawl-delay
}

// General politeness: 10-15 second delay between requests to same domain
// Even if robots.txt doesn't specify crawl-delay
```

**Detection:**
- 403 Forbidden from sites
- 429 Too Many Requests
- Crawl blocked after initial pages
- Complaints from site owners

**Phase mapping:** Phase 2 (politeness) - implement before production use

**Sources:**
- [Respecting Robots Exclusion Protocol or robots.txt at Scale](https://medium.com/gumgum-tech/respecting-robots-exclusion-protocol-or-robots-txt-at-scale-60ee57dc1295)
- [What is polite crawling?](https://www.firecrawl.dev/glossary/web-crawling-apis/what-is-polite-crawling)

---

## Minor Pitfalls

Small issues that reduce quality or polish but are easy to fix.

### Pitfall 11: Bubble Tea State Updates Outside Event Loop

**What goes wrong:** Modifying Bubble Tea model state directly from goroutines (instead of sending messages) causes race conditions and inconsistent UI rendering.

**Why it happens:**
- Background goroutines seem like natural place to update progress
- Not understanding Elm Architecture's message-passing model
- Trying to use shared state instead of messages

**Consequences:**
- Race conditions (data races detected by `-race`)
- UI shows inconsistent state
- Progress bar jumps or freezes
- Crashes from concurrent map access

**Prevention:**
```go
// BAD: Direct model mutation from goroutine
go func() {
    result := fetchURL(url)
    model.results[url] = result  // RACE CONDITION
}()

// GOOD: Send message to event loop
go func() {
    result := fetchURL(url)
    p.Send(ResultMsg{url: url, result: result})
}()

// In Update():
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case ResultMsg:
        m.results[msg.url] = msg.result  // Safe, sequential
    }
}
```

**Detection:**
- `go run -race` reports data races
- Inconsistent UI state
- Occasional panics on map access

**Phase mapping:** Phase 3 (TUI integration) - critical for Bubble Tea

**Sources:**
- [Tips for building Bubble Tea programs](https://leg100.github.io/en/posts/building-bubbletea-programs/)
- [Developing a terminal UI in Go with Bubble Tea](https://packagemain.tech/p/terminal-ui-bubble-tea)

---

### Pitfall 12: Not Setting Custom User-Agent

**What goes wrong:** Default Go User-Agent (`Go-http-client/1.1`) gets blocked by many servers as it's associated with malicious bots.

**Why it happens:**
- Not aware that User-Agent matters
- Using default http.Client
- Thinking it's just metadata

**Consequences:**
- 403 Forbidden from many sites
- False positives (legitimate pages marked as dead)
- Cloudflare challenges
- Site admins can't contact you to report issues

**Prevention:**
```go
req.Header.Set("User-Agent", "zombiecrawl/1.0 (+https://github.com/user/zombiecrawl)")
// Include contact info so site admins can reach you
```

**Detection:**
- Many 403 errors
- Cloudflare challenge pages
- Links work in browser but fail in crawler

**Phase mapping:** Phase 1 (HTTP client) - one-line fix, do early

**Sources:**
- [User-Agent Go-http-client/1.1](https://github.com/mitchellkrogza/nginx-ultimate-bad-bot-blocker/issues/273)
- [How the Go-based link checker works](https://sekika.github.io/2025/11/21/go-linkchecker/)

---

### Pitfall 13: Not Handling Same-Domain Detection Edge Cases

**What goes wrong:** Subdomain differences, www vs non-www, and port differences cause same-domain detection to fail, resulting in missing or over-crawling.

**Why it happens:**
- Simple string comparison: `startURL.Host == discoveredURL.Host`
- Not considering subdomain policies
- Not normalizing URLs

**Consequences:**
- Missing `www.example.com` when starting from `example.com`
- Crawling subdomains unintentionally (or vice versa)
- Inconsistent behavior

**Prevention:**
```go
// BASIC: Exact match (restrictive)
startHost := startURL.Host
if discoveredURL.Host != startHost { /* external */ }

// BETTER: Handle www prefix
func isSameDomain(host1, host2 string) bool {
    h1 := strings.TrimPrefix(host1, "www.")
    h2 := strings.TrimPrefix(host2, "www.")
    return h1 == h2
}

// BEST: Use --same-domain flag with configurable logic
// Let user decide if subdomains count as same domain
```

**Detection:**
- Not crawling www variant
- Unexpectedly crawling/skipping subdomains
- User reports missing links

**Phase mapping:** Phase 1 (domain filtering) - define policy early

**Sources:**
- [Go crawler domain detection](https://pkg.go.dev/github.com/jmwri/web-crawler)

---

### Pitfall 14: URL Fragment Not Stripped for Deduplication

**What goes wrong:** URL fragments (e.g., `#section`) don't change server response but are included in deduplication, causing same page to be crawled multiple times.

**Why it happens:**
- Not knowing fragments are client-side only
- Using raw URL as deduplication key
- Wanting to preserve full URL for display

**Consequences:**
- Same page fetched multiple times (different fragments)
- Inflated crawl count
- Slower crawl

**Prevention:**
```go
// Strip fragment for deduplication, keep for display
u, _ := url.Parse(rawURL)
dedupKey := u.Scheme + "://" + u.Host + u.Path + "?" + u.RawQuery
displayURL := rawURL  // Keep fragment for user
```

**Detection:**
- Same page crawled with `#intro`, `#contact`, etc.
- Duplicate entries in results

**Phase mapping:** Phase 1 (URL normalization) - handle during parsing

**Sources:**
- [Go URL parsing components](https://pkg.go.dev/net/url)

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| HTTP Client Setup | Response body not closed | Use defer resp.Body.Close() + io.Copy(io.Discard, resp.Body) |
| HTTP Client Setup | Poor connection pool config | Set MaxIdleConnsPerHost=20, add redirect limit |
| Worker Pool | Goroutine leaks from blocked channels | Use separate goroutine for wg.Wait() + close(results) |
| Worker Pool | Missing context cancellation | Propagate context.Context through all goroutines |
| URL Deduplication | Unbounded map growth | Use LRU cache or bloom filter for large crawls |
| URL Parsing | Relative URLs not resolved | Use url.Parse + ResolveReference, not string concat |
| Link Extraction | HTML parsing with regex | Use golang.org/x/net/html or goquery |
| Error Handling | No retry logic or naive retry | Use exponential backoff with jitter, classify errors |
| Politeness | Ignoring robots.txt | Use robotstxt library, implement crawl-delay |
| TUI Integration | Direct state mutation from goroutines | Use message passing (tea.Cmd, p.Send) exclusively |
| Domain Filtering | www vs non-www inconsistency | Normalize hosts, make subdomain policy configurable |

---

## Research Confidence Assessment

| Area | Confidence | Basis |
|------|------------|-------|
| Concurrency pitfalls | HIGH | Official Go docs, multiple authoritative sources |
| HTTP client issues | HIGH | Official net/http docs, production experience reports |
| URL handling | HIGH | Go net/url package docs, web standards (MDN) |
| Bubble Tea patterns | MEDIUM | Official examples, community blog posts (recent but limited) |
| Robots.txt | HIGH | Standards docs, crawler framework implementations |
| Memory leaks | HIGH | Go issue tracker, production debugging guides |

---

## Sources

**Goroutine Leaks:**
- [Understanding and Debugging Goroutine Leaks in Go Web Servers](https://leapcell.io/blog/understanding-and-debugging-goroutine-leaks-in-go-web-servers)
- [Goroutine Leaks - The Forgotten Sender](https://www.ardanlabs.com/blog/2018/11/goroutine-leaks-the-forgotten-sender.html)
- [Understanding and Preventing Goroutine Leaks in Go](https://medium.com/@srajsonu/understanding-and-preventing-goroutine-leaks-in-go-623cac542954)

**HTTP Client:**
- [TIL: Go Response Body MUST be closed](https://manishrjain.com/must-close-golang-http-response)
- [Always close the response body! Running out of file descriptors in Golang](https://www.j4mcs.dev/posts/golang-response-body/)
- [HTTP Connection Pooling in Go](https://davidbacisin.com/writing/golang-http-connection-pools-1)
- [Tuning the Go HTTP client settings for load testing](http://tleyden.github.io/blog/2016/11/21/tuning-the-go-http-client-library-for-load-testing/)
- [How to Use the HTTP Client in GO To Enhance Performance](https://www.loginradius.com/blog/engineering/tune-the-go-http-client-for-high-performance/)

**Context and Cancellation:**
- [Golang Context - Cancellation, Timeout and Propagation](https://golangbot.com/context-timeout-cancellation/)
- [Mastering Go Contexts: A Deep Dive Into Cancellation, Timeouts, and Request-Scoped Values](https://medium.com/@harshithgowdakt/mastering-go-contexts-a-deep-dive-into-cancellation-timeouts-and-request-scoped-values-392122ad0a47)
- [Context in Go: Managing Timeouts and Cancellations](https://abubakardev0.medium.com/context-in-go-managing-timeouts-and-cancellations-5a7291a59d0f)

**Worker Pool:**
- [Mastering the Worker Pool Pattern in Go](https://corentings.dev/blog/go-pattern-worker/)
- [The Case For A Go Worker Pool](https://brandur.org/go-worker-pool)
- [Buffered Channels and Worker Pools in Go](https://golangbot.com/buffered-channels-worker-pools/)

**Memory Leaks:**
- [Understanding and Preventing Memory Leak in Go Maps](https://medium.com/@tedious/go-map-memory-leaks-why-deleting-elements-doesnt-always-free-memory-670a81ad3be9)
- [Unseen Drains: The Subtle Art of Memory Leaks in Go](https://medium.com/@krisguttenbergovitz/unseen-drains-the-subtle-art-of-memory-leaks-in-go-da89f82e22da)

**URL Handling:**
- [Resolving relative references to a URL](https://developer.mozilla.org/en-US/docs/Web/API/URL_API/Resolving_relative_references)
- [Go URL parsing](https://pkg.go.dev/net/url)
- [How to avoid crawling duplicate URLs at Google scale](https://blog.bytebytego.com/p/how-to-avoid-crawling-duplicate-urls)

**Retry Logic:**
- [How I improved consistency and performance in a Go crawler with retry logics](https://blog.maxgio.me/posts/improving-consistency-performance-go-crawler-retry-logics-http-client-tuning/)
- [How to Implement Retry Logic in Go with Exponential Backoff](https://oneuptime.com/blog/post/2026-01-07-go-retry-exponential-backoff/view)
- [Don't Let it Fail: Retry Pattern in GO](https://medium.com/@fredrsf/dont-let-it-fail-retry-pattern-in-go-fb4decf4df3c)

**HTML Parsing:**
- [Web Crawler - Go Concurrency](https://medium.com/@wu.victor.95/web-crawler-go-concurrency-196e9c0fec8)
- [What is the best way to parse HTML in Go?](https://webscraping.ai/faq/go/what-is-the-best-way-to-parse-html-in-go)
- [Web Scraping in Go with goquery](https://zetcode.com/golang/goquery/)

**Robots.txt:**
- [Respecting Robots Exclusion Protocol or robots.txt at Scale](https://medium.com/gumgum-tech/respecting-robots-exclusion-protocol-or-robots-txt-at-scale-60ee57dc1295)
- [What is polite crawling?](https://www.firecrawl.dev/glossary/web-crawling-apis/what-is-polite-crawling)
- [How to Find and Read robots.txt for Crawling and Scraping](https://scrape.do/blog/robots-txt/)

**Bubble Tea:**
- [Tips for building Bubble Tea programs](https://leg100.github.io/en/posts/building-bubbletea-programs/)
- [Developing a terminal UI in Go with Bubble Tea](https://packagemain.tech/p/terminal-ui-bubble-tea)
- [Building a Terminal IRC Client with Bubble Tea](https://sngeth.com/go/terminal/ui/bubble-tea/2025/08/17/building-terminal-ui-with-bubble-tea/)

**General Crawling:**
- [How to Build a Web Crawler With Go](https://www.zenrows.com/blog/golang-web-crawler)
- [Web Scraping in Golang: 2026 Complete Guide](https://www.zenrows.com/blog/web-scraping-golang)
