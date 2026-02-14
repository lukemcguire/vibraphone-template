# Technology Stack

**Project:** zombiecrawl
**Researched:** 2026-02-13

## Recommended Stack

### Core Framework
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go | 1.25+ | Language runtime | Industry standard for CLI tools, excellent concurrency primitives, compiled binaries |
| Go Modules | Latest | Dependency management | Official standard, reproducible builds, semantic versioning |

### TUI (Terminal User Interface)
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Bubble Tea | v2.0.0-beta.6+ | TUI framework | De facto standard for Go TUIs, Elm architecture pattern, active development (Charm ecosystem) |
| Lip Gloss | v2.0.0-beta1+ | Terminal styling | Companion to Bubble Tea, CSS-like declarative styling, automatic color profile detection |
| Bubbles | v2.0.0+ | TUI components | Pre-built components (spinner, progress, table), maintained by Charm team |
| Charm Log | v0.4.2+ | Structured logging | Beautiful human-readable output, implements slog.Handler, designed for CLI tools |

**Rationale:** The Charm ecosystem is purpose-built for beautiful CLI applications. Bubble Tea's Elm architecture provides predictable state management. v2 represents the current major version with active development. Beta status is acceptable for a learning project and allows migration path to stable v2.

**Confidence:** HIGH - Official Charm ecosystem documentation and pkg.go.dev verified, published Oct 2025 - Feb 2026.

### HTML Parsing
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| golang.org/x/net/html | Latest | HTML parsing | Standard library extension, HTML5 compliant, lower-level control for link extraction |
| PuerkitoBio/goquery | v1.11.0+ | Optional helper | jQuery-like API if complex selectors needed, built on net/html, stable API guarantee |

**Rationale:** Use `net/html` directly for performance - it's ~2x faster than goquery for simple link extraction. For a dead link checker, you only need to extract `href` attributes, not complex DOM traversal. Goquery adds convenience but unnecessary overhead. Include as optional dependency if roadmap adds complex HTML analysis.

**Confidence:** MEDIUM - Based on official Go package documentation and 2026 web scraping tutorials. Performance claim from 2014 benchmark, needs validation for current versions.

### HTTP Client
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| net/http | Standard library | HTTP requests | Production-ready, sufficient for dead link checking, no external dependencies |
| net/url | Standard library | URL parsing/validation | RFC 3986 compliant, handles encoding automatically |

**Rationale:** Standard library is sufficient. Avoid frameworks like Colly (designed for full scraping, too heavyweight). Configure custom `http.Client` with timeouts, connection pooling, and context cancellation. Never use `http.DefaultClient` in production (no timeout = requests hang indefinitely).

**Best practices:**
- Set `Client.Timeout` (e.g., 30s total including redirects)
- Configure `Transport` for granular control (DialTimeout, TLSHandshakeTimeout, ResponseHeaderTimeout)
- Use context for per-request cancellation
- Reuse singleton client for connection pooling
- Handle timeout errors with exponential backoff

**Confidence:** HIGH - Official Go documentation and multiple 2026 best practice guides confirm this approach.

### Concurrency
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go goroutines | Built-in | Concurrent crawling | Native Go feature, lightweight, efficient |
| sync package | Standard library | Worker pool synchronization | WaitGroups, Mutexes for coordination |
| context package | Standard library | Cancellation/timeouts | Graceful shutdown, timeout propagation |

**Rationale:** Implement worker pool pattern to limit concurrent requests. Unbounded goroutines cause memory spikes and unpredictable performance. Worker pool provides backpressure, rate limiting, and controlled parallelism. Use buffered channels as task queue.

**Pattern:**
```go
// Fixed worker pool (e.g., 10 workers)
// Buffered channel for URL queue
// Use context for cancellation
// Implement graceful shutdown
```

**Confidence:** HIGH - Worker pool is standard pattern for Go crawlers, documented in 2025-2026 concurrency guides.

### CLI Argument Parsing
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| spf13/cobra | v1.10.2+ | Command structure | De facto standard for Go CLIs (used by kubectl, Hugo, GitHub CLI) |
| spf13/viper | v1.21.0+ | Configuration | Companion to Cobra, handles config files + env vars + flags with precedence |

**Rationale:** Cobra + Viper is the industry standard. Cobra handles command structure, flags, help generation, and shell completions. Viper provides hierarchical config (flags override env vars override config file override defaults). Use `viper.BindPFlag` to connect them.

**Confidence:** MEDIUM - Versions from Sep-Dec 2025 verified via pkg.go.dev, but pkg.go.dev showed "not latest" warnings. Latest versions likely exist by Feb 2026 but not accessible in research. Use `go get -u` to fetch current versions.

### Output Formats
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| encoding/json | Standard library | JSON output | Built-in, no dependencies, sufficient for structured output |
| encoding/csv | Standard library | CSV output | Built-in, handles escaping and quoting |
| text/tabwriter | Standard library | Human-readable tables | Aligned columns for terminal output |

**Rationale:** Standard library covers all requirements. Avoid third-party CSV/JSON libraries (unnecessary complexity). For `--json` flag, marshal report struct directly. For `--csv`, use csv.Writer with proper headers. For human-readable (default), combine Lip Gloss styling with tabwriter for aligned output.

**Confidence:** HIGH - Standard library features verified in official Go documentation.

### Testing
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| testing | Standard library | Test runner | Official framework, sufficient for most cases |
| testify/assert | v1.10.0+ | Assertions | Expressive assertions, clear error messages, reduces boilerplate |
| testify/require | v1.10.0+ | Fatal assertions | Same as assert but stops test on failure |
| net/http/httptest | Standard library | HTTP mocking | Mock HTTP servers for testing crawl logic |

**Rationale:** Standard library `testing` package is the foundation. Add testify for better assertions (reduces boilerplate, clearer failures). Use `httptest.NewServer` to mock websites during testing. Avoid heavy mocking frameworks like gomock (adds complexity without benefit for this project). testify is maintained, active (Aug 2025 release), and stable at v1.

**Alternatives considered:** gomock (no longer maintained, forked by Uber), mockio (too new, unproven).

**Confidence:** HIGH - Testify documentation and 2026 testing framework comparisons confirm this approach.

### Optional/Nice-to-Have Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| jimsmart/grobotstxt | Latest | Parse robots.txt | If roadmap includes respecting robots.txt (ethical crawling) |
| oxffaa/gopher-parse-sitemap | Latest | Parse sitemap.xml | If roadmap includes sitemap-first crawling for efficiency |
| gammazero/workerpool | Latest | Worker pool implementation | If you want proven worker pool vs rolling your own |

**Rationale:** These are optional enhancements. grobotstxt is Google's official robots.txt parser ported to Go (production code used by Googlebot). gopher-parse-sitemap handles large sitemaps with low memory usage. workerpool is a battle-tested implementation if you want to avoid custom worker pool code.

**Confidence:** MEDIUM - Libraries exist and are documented, but feasibility for zombiecrawl depends on roadmap scope decisions.

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| TUI Framework | Bubble Tea | tview, termui | Bubble Tea has better docs, active development, Elm architecture advantages |
| HTML Parsing | net/html | Colly | Colly is full scraping framework (too heavyweight), includes headless browser support (unnecessary) |
| HTTP Client | net/http (custom) | Colly, go-resty | Standard library sufficient, external deps add complexity without clear benefit |
| Logging | Charm Log | zerolog, zap, slog | zerolog/zap optimize for JSON/machines (not CLI UX), slog lacks color (Charm Log wraps slog) |
| CLI Framework | Cobra/Viper | flag, urfave/cli | Cobra is industry standard, better help generation, wider ecosystem |
| Testing | testify | gomock, ginkgo | gomock unmaintained, ginkgo adds BDD syntax (unnecessary complexity for learning project) |

## Anti-Recommendations

**DO NOT USE:**
- `http.DefaultClient` - No timeout, requests hang indefinitely in production
- `http.Get()` shorthand - Same issue, creates DefaultClient internally
- Unbounded goroutines - Spawn goroutine per URL = memory explosion, use worker pool
- Colly framework - Designed for complex scraping (JS rendering, anti-bot bypass), massive overkill for dead links
- Bubble Tea v1 - v2 is current major version, start with v2 to avoid migration later
- gomock - No longer maintained (original), fork exists but testify is more popular

**WHY:**
- DefaultClient: Production showstopper, requests can hang forever
- Unbounded concurrency: Kills performance under load, unpredictable memory usage
- Colly: You're parsing static HTML for `<a href>`, not scraping dynamic content
- v1 libraries: Start with current major versions to avoid breaking changes

## Installation

```bash
# Initialize module
go mod init github.com/yourname/zombiecrawl

# Core dependencies
go get github.com/charmbracelet/bubbletea/v2@latest
go get github.com/charmbracelet/lipgloss/v2@latest
go get github.com/charmbracelet/bubbles/v2@latest
go get github.com/charmbracelet/log@latest
go get github.com/spf13/cobra@latest
go get github.com/spf13/viper@latest
go get golang.org/x/net/html@latest

# Testing
go get github.com/stretchr/testify@latest

# Optional (add as needed)
go get github.com/jimsmart/grobotstxt@latest
go get github.com/oxffaa/gopher-parse-sitemap@latest
go get github.com/PuerkitoBio/goquery@latest
```

## Project Structure Recommendation

```
zombiecrawl/
├── cmd/
│   └── zombiecrawl/
│       └── main.go              # Entry point, Cobra root command
├── internal/
│   ├── crawler/
│   │   ├── crawler.go           # Core crawl logic
│   │   ├── worker_pool.go       # Worker pool implementation
│   │   └── link_extractor.go   # HTML parsing for links
│   ├── checker/
│   │   └── http_checker.go     # HTTP status code validation
│   ├── reporter/
│   │   ├── json.go              # JSON output
│   │   ├── csv.go               # CSV output
│   │   └── table.go             # Human-readable table
│   └── tui/
│       ├── model.go             # Bubble Tea model
│       ├── update.go            # Bubble Tea update
│       └── view.go              # Bubble Tea view (with Lip Gloss)
├── tests/
│   └── integration_test.go
├── go.mod
├── go.sum
└── README.md
```

**Rationale:**
- `cmd/` for binaries (Go convention)
- `internal/` prevents external imports (encapsulation)
- Separate concerns: crawling, checking, reporting, TUI
- `tests/` for integration tests (unit tests alongside code in `_test.go` files)

## Version Notes

**IMPORTANT:** Versions listed reflect research as of 2026-02-13. Several libraries showed "not latest" warnings on pkg.go.dev:

- Bubble Tea: v2.0.0-beta.6 published Oct 2025 (beta, but v2 is current major version)
- Lip Gloss: v2.0.0-beta1 published Mar 2025 (beta, but v2 is current major version)
- Bubbles: v2.0.0 mentioned but v1.0.0 shown on pkg.go.dev (published Feb 9, 2026)
- Cobra: v1.10.2 published Dec 2025 (may have newer version)
- Viper: v1.21.0 published Sep 2025 (may have newer version)
- goquery: v1.11.0 published Nov 2025 (stable)

**Action:** Run `go get -u` to fetch latest versions when initializing project. The listed versions are minimums; newer versions likely exist by Feb 2026.

## Sources

**Charm Ecosystem:**
- [Bubble Tea GitHub](https://github.com/charmbracelet/bubbletea)
- [Lip Gloss Documentation](https://pkg.go.dev/github.com/charmbracelet/lipgloss)
- [Bubbles Components](https://github.com/charmbracelet/bubbles)
- [Building Terminal UI with Bubble Tea (2025)](https://sngeth.com/go/terminal/ui/bubble-tea/2025/08/17/building-terminal-ui-with-bubble-tea/)

**HTML Parsing:**
- [Golang Web Scraping 2025 Guide](https://www.zyte.com/learn/golang-web-scraping-in-2025-tools-techniques-and-best-practices/)
- [How to Parse HTML in Golang 2026](https://www.zenrows.com/blog/golang-html-parser)
- [goquery Documentation](https://github.com/PuerkitoBio/goquery)

**HTTP Client & Concurrency:**
- [Go HTTP Client Timeouts Best Practices 2026](https://oneuptime.com/blog/post/2026-01-23-go-http-timeouts/view)
- [Don't Use Go's Default HTTP Client](https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779)
- [7 Powerful Golang Concurrency Patterns 2025](https://cristiancurteanu.com/7-powerful-golang-concurrency-patterns-that-will-transform-your-code-in-2025/)
- [Worker Pool Pattern Deep Dive](https://rksurwase.medium.com/efficient-concurrency-in-go-a-deep-dive-into-the-worker-pool-pattern-for-batch-processing-73cac5a5bdca)

**CLI Tools:**
- [How to Build CLI with Cobra (Feb 2026)](https://oneuptime.com/blog/post/2026-02-03-go-cobra-cli/view)
- [Building CLI Apps with Cobra & Viper (Nov 2025)](https://www.glukhov.org/post/2025/11/go-cli-applications-with-cobra-and-viper/)

**Testing:**
- [Top Golang Testing Frameworks 2026](https://reliasoftware.com/blog/golang-testing-framework)
- [testify GitHub](https://github.com/stretchr/testify)

**Dependency Management:**
- [Go Modules Best Practices 2026](https://oneuptime.com/blog/post/2026-01-23-go-modules-dependency/view)
- [Mastering go.mod](https://medium.com/@moksh.9/mastering-go-mod-dependency-management-the-right-way-in-go-918226a69d58)

**Logging:**
- [Top Golang Logging Libraries 2026](https://reliasoftware.com/blog/golang-logging-libraries)
- [Charmbracelet Log GitHub](https://github.com/charmbracelet/log)
- [High-Performance Logging with slog and zerolog](https://leapcell.io/blog/high-performance-structured-logging-in-go-with-slog-and-zerolog)

**Optional Libraries:**
- [grobotstxt - Google robots.txt parser](https://github.com/jimsmart/grobotstxt)
- [gopher-parse-sitemap](https://github.com/oxffaa/gopher-parse-sitemap)
