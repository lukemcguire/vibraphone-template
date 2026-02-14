# Architecture Patterns

**Domain:** Go CLI dead link checker (zombiecrawl)
**Researched:** 2026-02-13
**Confidence:** MEDIUM

## Recommended Architecture

zombiecrawl follows a layered architecture with clear separation between UI, orchestration, crawling, and validation concerns. The architecture is built around Go's concurrency primitives (goroutines, channels, mutexes) with Bubble Tea providing the UI layer.

```
┌─────────────────────────────────────────────────────────┐
│                     CLI Entry Point                      │
│                  (parse flags, config)                   │
└───────────────────────┬─────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────────┐
│                   Bubble Tea TUI                         │
│        (Model-View-Update, render progress)              │
│  - Init(): start crawler command                         │
│  - Update(msg): handle crawler messages                  │
│  - View(): render current state with Lip Gloss           │
└───────────────────────┬─────────────────────────────────┘
                        │ tea.Cmd (async)
                        ▼
┌─────────────────────────────────────────────────────────┐
│                 Crawler Orchestrator                     │
│   - URL Frontier (channel-based queue)                   │
│   - Visited Set (sync.Map or map + mutex)                │
│   - Worker Pool Manager                                  │
│   - Results Aggregator                                   │
└─────────┬───────────────────────────┬───────────────────┘
          │                           │
          │ URLs to check             │ Results
          ▼                           ▼
┌──────────────────────┐    ┌──────────────────────┐
│   Worker Goroutines  │    │  Results Collector   │
│  (per-host spacing)  │    │  (dead links list)   │
└──────────┬───────────┘    └──────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────┐
│         HTTP Client + Link Validator         │
│  - Retry logic (exponential backoff)         │
│  - Timeout configuration                     │
│  - Status code checking                      │
│  - Response validation                       │
└──────────────────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────┐
│            HTML Parser (goquery)             │
│  - Extract links from HTML                   │
│  - Resolve relative URLs                     │
│  - Filter same-domain vs external            │
└──────────────────────────────────────────────┘
```

### Component Boundaries

| Component | Responsibility | Communicates With | Interface |
|-----------|---------------|-------------------|-----------|
| **CLI Entry** | Parse flags, validate config, initialize TUI | Bubble Tea TUI | Command-line args → Config struct |
| **Bubble Tea TUI** | Render progress, handle user input, orchestrate crawler lifecycle | Crawler Orchestrator, Results Collector | tea.Model interface (Init/Update/View) |
| **Crawler Orchestrator** | Manage URL queue, track visited URLs, spawn/manage workers, enforce concurrency limits | Workers, TUI, Parser | Channels for URLs and results |
| **Worker Pool** | Execute link checks with per-host rate limiting, handle retries | HTTP Client, Orchestrator | URL channel (in), Result channel (out) |
| **HTTP Client** | Make HTTP requests, handle timeouts/retries, validate responses | Workers | Request → Response + Error |
| **HTML Parser** | Extract links, resolve relative URLs, classify same-domain vs external | Orchestrator | HTML string → []URL |
| **Results Collector** | Aggregate dead links, generate report | TUI, Orchestrator | Results channel → Report struct |
| **Visited Set** | Deduplicate URLs to prevent loops | Orchestrator | URL → bool (visited check) |

### Data Flow

```
1. User invokes CLI with starting URL
   ↓
2. CLI validates config, starts Bubble Tea program
   ↓
3. TUI Init() returns tea.Cmd to start crawler
   ↓
4. Crawler Orchestrator:
   a. Adds starting URL to frontier queue
   b. Spawns N workers (from config)
   c. Worker pulls URL from queue
   ↓
5. Worker (per URL):
   a. Checks visited set (skip if seen)
   b. Marks as visited
   c. Sends HTTP request via client
   d. If success + HTML + same-domain:
      - Parse HTML for links
      - Send new URLs to queue
   e. If error/4xx/5xx:
      - Send to results channel (dead link)
   f. Sends progress message to TUI
   ↓
6. TUI Update() receives messages:
   - Progress update → render new state
   - Dead link found → add to results list
   - Crawler complete → final report
   ↓
7. TUI View() renders:
   - Spinner/progress bar
   - URLs checked count
   - Dead links count
   - Current URL being checked
   ↓
8. On completion:
   - Final report displayed
   - Exit or offer to save results
```

## Patterns to Follow

### Pattern 1: Per-Host Worker Assignment
**What:** Assign each host to a dedicated worker goroutine with rate limiting per host
**When:** Prevents overwhelming servers with parallel requests
**Why:** HTTP clients can send many requests to same host in parallel, causing server overload or IP bans
**Example:**
```go
type HostWorker struct {
    host     string
    urlQueue chan *url.URL
    interval time.Duration
    client   *http.Client
}

func (hw *HostWorker) Start(ctx context.Context, results chan<- Result) {
    ticker := time.NewTicker(hw.interval)
    defer ticker.Done()

    for {
        select {
        case <-ctx.Done():
            return
        case url := <-hw.urlQueue:
            <-ticker.C // Rate limit
            resp, err := hw.client.Get(url.String())
            results <- Result{URL: url, Response: resp, Error: err}
        }
    }
}
```
**Source:** [How the Go-based link checker works](https://sekika.github.io/2025/11/21/go-linkchecker/)

### Pattern 2: Bubble Tea Command Pattern for Async Operations
**What:** Wrap all I/O operations (HTTP requests, parsing) as tea.Cmd functions
**When:** Any blocking operation (HTTP, file I/O, parsing)
**Why:** Keeps UI responsive, prevents lockups
**Example:**
```go
type crawlCompleteMsg struct {
    results []DeadLink
    err     error
}

func crawlWebsiteCmd(startURL string, config Config) tea.Cmd {
    return func() tea.Msg {
        results, err := runCrawler(startURL, config)
        return crawlCompleteMsg{results: results, err: err}
    }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case crawlCompleteMsg:
        m.complete = true
        m.results = msg.results
        return m, tea.Quit
    }
    return m, nil
}
```
**Source:** [Commands in Bubble Tea](https://charm.land/blog/commands-in-bubbletea/), [HTTP and Async Operations | charmbracelet/bubbletea](https://deepwiki.com/charmbracelet/bubbletea/6.4-step-by-step-tutorials)

### Pattern 3: URL Frontier with Visited Set
**What:** Channel-based queue + concurrent-safe visited map
**When:** Managing URLs to crawl and preventing duplicates
**Why:** Prevents infinite loops, ensures each URL checked exactly once
**Example:**
```go
type URLFrontier struct {
    queue   chan *url.URL
    visited sync.Map // or map[string]bool + sync.RWMutex
}

func (uf *URLFrontier) Add(u *url.URL) bool {
    normalized := normalizeURL(u)
    if _, loaded := uf.visited.LoadOrStore(normalized, true); loaded {
        return false // Already visited
    }
    uf.queue <- u
    return true
}
```
**Source:** [Use Go Channels to Build a Crawler](https://medium.com/@jorinvo/use-go-channels-to-build-a-crawler-1e71cdb5f11), [Go Web Crawler: Build one step-by-step](https://roundproxies.com/blog/go-web-crawler/)

### Pattern 4: Worker Pool with Bounded Concurrency
**What:** Fixed number of worker goroutines, shared URL queue
**When:** Need to limit concurrent HTTP requests
**Why:** Prevents resource exhaustion, controls memory usage
**Example:**
```go
func startWorkerPool(numWorkers int, urls <-chan *url.URL, results chan<- Result) {
    var wg sync.WaitGroup

    for i := 0; i < numWorkers; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for url := range urls {
                result := checkLink(url)
                results <- result
            }
        }(i)
    }

    wg.Wait()
    close(results)
}
```
**Source:** [The Worker Pool Pattern in Go: A Practical Implementation](https://elsyarifx.medium.com/the-worker-pool-pattern-in-go-a-practical-implementation-fdd9b81de5ea), [Mastering the Worker Pool Pattern in Go](https://corentings.dev/blog/go-pattern-worker/)

### Pattern 5: Retryable HTTP Client with Exponential Backoff
**What:** Wrap net/http.Client with automatic retry logic
**When:** Network requests that may fail transiently
**Why:** Improves reliability for temporary failures (503, timeouts, connection resets)
**Example:**
```go
import "github.com/hashicorp/go-retryablehttp"

func newRetryableClient(maxRetries int) *http.Client {
    retryClient := retryablehttp.NewClient()
    retryClient.RetryMax = maxRetries
    retryClient.RetryWaitMin = 1 * time.Second
    retryClient.RetryWaitMax = 10 * time.Second
    retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
        // Retry on 5xx, connection errors
        if err != nil || resp.StatusCode >= 500 {
            return true, nil
        }
        return false, nil
    }
    return retryClient.StandardClient()
}
```
**Source:** [hashicorp/go-retryablehttp](https://github.com/hashicorp/go-retryablehttp), [How I improved consistency and performance in a Go crawler](https://blog.maxgio.me/posts/improving-consistency-performance-go-crawler-retry-logics-http-client-tuning/)

### Pattern 6: Elm Architecture (Model-View-Update)
**What:** State in Model struct, Update handles messages, View renders
**When:** Building any Bubble Tea TUI
**Why:** Predictable state management, clear separation of concerns
**Example:**
```go
type model struct {
    urlsChecked int
    deadLinks   []DeadLink
    currentURL  string
    spinner     spinner.Model
}

func (m model) Init() tea.Cmd {
    return tea.Batch(
        m.spinner.Tick,
        crawlWebsiteCmd(m.startURL, m.config),
    )
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case progressMsg:
        m.urlsChecked++
        m.currentURL = msg.url
    case deadLinkMsg:
        m.deadLinks = append(m.deadLinks, msg.link)
    }
    return m, nil
}

func (m model) View() string {
    return lipgloss.JoinVertical(lipgloss.Left,
        fmt.Sprintf("%s Checking: %s", m.spinner.View(), m.currentURL),
        fmt.Sprintf("URLs checked: %d", m.urlsChecked),
        fmt.Sprintf("Dead links: %d", len(m.deadLinks)),
    )
}
```
**Source:** [Bubble Tea documentation](https://github.com/charmbracelet/bubbletea), [Tips for building Bubble Tea programs](https://leg100.github.io/en/posts/building-bubbletea-programs/)

## Anti-Patterns to Avoid

### Anti-Pattern 1: Accessing Model from Command Goroutines
**What:** Directly reading/modifying model state inside tea.Cmd goroutines
**Why bad:** Race conditions, data corruption (model is not goroutine-safe)
**Instead:** Commands return messages, Update modifies model based on messages
```go
// BAD: Command modifies model directly
func badCmd(m *model) tea.Cmd {
    return func() tea.Msg {
        m.counter++ // RACE CONDITION
        return nil
    }
}

// GOOD: Command returns message, Update modifies model
type incrementMsg struct{}

func goodCmd() tea.Cmd {
    return func() tea.Msg {
        return incrementMsg{}
    }
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg.(type) {
    case incrementMsg:
        m.counter++ // Safe
    }
    return m, nil
}
```
**Source:** [charmbracelet/bubbletea](https://deepwiki.com/charmbracelet/bubbletea)

### Anti-Pattern 2: Unbounded Goroutine Spawning
**What:** Spawning a goroutine for every URL without limit
**Why bad:** Resource exhaustion, memory bloat, OS limits on goroutines
**Instead:** Use worker pool with bounded concurrency
```go
// BAD: Spawn unlimited goroutines
for _, url := range urls {
    go checkLink(url) // Thousands of goroutines
}

// GOOD: Worker pool
startWorkerPool(numWorkers, urlQueue, results)
```
**Source:** [The Worker Pool Pattern in Go](https://elsyarifx.medium.com/the-worker-pool-pattern-in-go-a-practical-implementation-fdd9b81de5ea)

### Anti-Pattern 3: Shared Map Without Synchronization
**What:** Multiple goroutines reading/writing map without mutex
**Why bad:** Concurrent map read/write panics, data races
**Instead:** Use sync.Map or map + sync.RWMutex
```go
// BAD: Concurrent map access
var visited = make(map[string]bool)
go func() { visited[url] = true }() // PANIC

// GOOD: sync.Map
var visited sync.Map
visited.Store(url, true)

// GOOD: map + mutex
var (
    visited = make(map[string]bool)
    mu      sync.RWMutex
)
mu.Lock()
visited[url] = true
mu.Unlock()
```
**Source:** [Use Go Channels to Build a Crawler](https://medium.com/@jorinvo/use-go-channels-to-build-a-crawler-1e71cdb5f11)

### Anti-Pattern 4: No Rate Limiting per Host
**What:** Sending many parallel requests to same host
**Why bad:** Server overload, IP bans, 429 errors, bad netizen behavior
**Instead:** Per-host worker with time.Ticker rate limiting
```go
// BAD: Parallel requests to same host
for _, url := range urls {
    go http.Get(url.String()) // Hammers server
}

// GOOD: Per-host rate limiting
ticker := time.NewTicker(100 * time.Millisecond)
for url := range hostURLs {
    <-ticker.C
    http.Get(url.String())
}
```
**Source:** [How the Go-based link checker works](https://sekika.github.io/2025/11/21/go-linkchecker/)

### Anti-Pattern 5: Blocking I/O in Bubble Tea Update
**What:** Calling http.Get(), file I/O, or parsing inside Update()
**Why bad:** Freezes UI, makes app unresponsive
**Instead:** All I/O in tea.Cmd, return messages to Update
```go
// BAD: Blocking I/O in Update
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    resp, _ := http.Get(m.url) // BLOCKS UI
    return m, nil
}

// GOOD: I/O in command
func fetchCmd(url string) tea.Cmd {
    return func() tea.Msg {
        resp, err := http.Get(url)
        return fetchCompleteMsg{resp, err}
    }
}
```
**Source:** [Commands in Bubble Tea](https://charm.land/blog/commands-in-bubbletea/)

### Anti-Pattern 6: Ignoring Context Cancellation
**What:** Long-running operations that don't check ctx.Done()
**Why bad:** Can't gracefully shutdown, goroutines leak
**Instead:** Always select on ctx.Done() in loops
```go
// BAD: No cancellation
for url := range urls {
    checkLink(url)
}

// GOOD: Context-aware
for {
    select {
    case <-ctx.Done():
        return
    case url := <-urls:
        checkLink(url)
    }
}
```

## Scalability Considerations

| Concern | Small Site (100 pages) | Medium Site (10K pages) | Large Site (1M pages) |
|---------|------------------------|-------------------------|----------------------|
| **Worker Count** | 5-10 workers | 50-100 workers | 100-500 workers |
| **Visited Set** | map + mutex (KB) | map + mutex (MB) | sync.Map or external store (Redis) |
| **Memory** | In-memory queue | In-memory with periodic GC | Disk-backed queue (BoltDB, SQLite) |
| **Rate Limiting** | Simple time.Ticker per host | Token bucket per domain | Distributed rate limiter |
| **Results Storage** | In-memory slice | In-memory + batch writes | Streaming to file/DB |
| **HTTP Client** | Default http.Client | Connection pooling, keep-alive | Custom Transport with tuned MaxIdleConns |

**Notes:**
- For zombiecrawl (MVP), small-medium site approach is sufficient
- sync.Map scales better than map+mutex for high-concurrency reads
- At 1M+ URLs, consider streaming results to disk rather than holding in memory

## Build Order Implications

Based on component dependencies, suggested build order:

### Phase 1: Core Crawler (No TUI)
**Build:** HTTP Client → URL Parser → Visited Set → Worker Pool → Orchestrator
**Why:** Establishes foundation, testable without UI complexity
**Dependencies:** HTTP client used by workers, parser feeds orchestrator, visited set prevents loops

### Phase 2: Basic TUI
**Build:** Bubble Tea skeleton → Progress display → Integration with crawler
**Why:** Adds user interface to working crawler
**Dependencies:** Requires Phase 1 crawler, wraps it with tea.Cmd pattern

### Phase 3: Advanced Features
**Build:** Per-host rate limiting → Retry logic → Results output
**Why:** Enhances reliability and professionalism
**Dependencies:** Builds on Phase 1 worker pool, Phase 2 TUI for progress

### Key Dependencies:
```
HTTP Client ──> Worker Pool ──> Orchestrator ──> Bubble Tea TUI
                     ▲                ▲
                     │                │
                URL Parser      Visited Set
```

**Critical Path:** Can't build workers without HTTP client, can't build orchestrator without workers/visited set, can't integrate TUI without working orchestrator.

**Parallelization Opportunity:** URL Parser and HTTP Client can be built in parallel (no interdependency).

## Confidence Assessment

| Aspect | Confidence | Rationale |
|--------|-----------|-----------|
| Overall Architecture | MEDIUM | Well-established patterns from WebSearch, verified across multiple sources |
| Worker Pool Pattern | HIGH | Go idiom, well-documented in multiple sources including official Go by Example |
| Bubble Tea Integration | MEDIUM | Official documentation and tutorials available, but no zombiecrawl-specific examples |
| Per-Host Rate Limiting | MEDIUM | Pattern described in recent Go link checker implementation, makes architectural sense |
| HTTP Retry Logic | HIGH | hashicorp/go-retryablehttp is production-proven library |
| Scalability Numbers | LOW | Estimates based on general Go performance, not zombiecrawl-specific benchmarks |

**Gaps:**
- No official Context7 documentation available for Bubble Tea or Colly
- Scalability numbers are estimates, not tested with zombiecrawl workload
- Per-host rate limiting pattern described conceptually, no production reference implementation examined

## Sources

### Architecture & Patterns
- [Use Go Channels to Build a Crawler](https://medium.com/@jorinvo/use-go-channels-to-build-a-crawler-1e71cdb5f11)
- [How to Build a Web Crawler With Go - ZenRows](https://www.zenrows.com/blog/golang-web-crawler)
- [Go Web Crawler: Build one step-by-step](https://roundproxies.com/blog/go-web-crawler/)
- [Web Crawler System Design - EnjoyAlgorithms](https://www.enjoyalgorithms.com/blog/web-crawler/)

### Worker Pool Pattern
- [The Worker Pool Pattern in Go: A Practical Implementation](https://elsyarifx.medium.com/the-worker-pool-pattern-in-go-a-practical-implementation-fdd9b81de5ea)
- [Mastering the Worker Pool Pattern in Go](https://corentings.dev/blog/go-pattern-worker/)
- [Go by Example: Worker Pools](https://gobyexample.com/worker-pools)
- [Implementing Worker Pool Pattern in Go](https://wrizkia.medium.com/implementing-worker-pool-pattern-in-go-fc6ad7e376ab)

### Bubble Tea Architecture
- [charmbracelet/bubbletea - GitHub](https://github.com/charmbracelet/bubbletea)
- [Commands in Bubble Tea](https://charm.land/blog/commands-in-bubbletea/)
- [Tips for building Bubble Tea programs](https://leg100.github.io/en/posts/building-bubbletea-programs/)
- [How to Build Command Line Tools with Bubbletea in Go](https://oneuptime.com/blog/post/2026-01-30-how-to-build-command-line-tools-with-bubbletea-in-go/view)
- [HTTP and Async Operations | charmbracelet/bubbletea](https://deepwiki.com/charmbracelet/bubbletea/6.4-step-by-step-tutorials)

### HTTP Client & Retry Logic
- [hashicorp/go-retryablehttp - GitHub](https://github.com/hashicorp/go-retryablehttp)
- [How I improved consistency and performance in a Go crawler](https://blog.maxgio.me/posts/improving-consistency-performance-go-crawler-retry-logics-http-client-tuning/)
- [HTTP Retries in Go](https://medium.com/@nitishkr88/http-retries-in-go-e622e51d249f)

### Link Checker Implementations
- [How the Go-based link checker works](https://sekika.github.io/2025/11/21/go-linkchecker/)
- [raviqqe/muffet - Fast website link checker in Go](https://github.com/raviqqe/muffet)
- [coderme/gocheck - Advanced concurrent HTTP links checker](https://github.com/coderme/gocheck)

### Colly Framework
- [gocolly/colly - GitHub](https://github.com/gocolly/colly)
- [Scraping the Web in Golang with Colly and Goquery](https://benjamincongdon.me/blog/2018/03/01/Scraping-the-Web-in-Golang-with-Colly-and-Goquery/)
