# Project Research Summary

**Project:** zombiecrawl
**Domain:** CLI Dead Link Checker / Web Crawler
**Researched:** 2026-02-13
**Confidence:** MEDIUM-HIGH

## Executive Summary

zombiecrawl is a Go-based CLI tool for checking dead links on websites through recursive crawling. Expert implementations in this domain (lychee, hyperlink, muffet) use concurrent worker pools, proper HTTP client configuration, and politeness mechanisms (robots.txt, rate limiting) to achieve both performance and reliability. The recommended approach leverages Go's native concurrency primitives (goroutines, channels) with the Charm ecosystem (Bubble Tea, Lip Gloss) to create a beautiful, terminal-native experience that differentiates zombiecrawl from utilitarian competitors.

The critical architectural foundation centers on three core patterns: a worker pool with bounded concurrency to prevent resource exhaustion, per-host rate limiting to avoid IP bans, and the Elm architecture (Model-View-Update) for predictable TUI state management. Standard library HTTP and HTML parsing tools are sufficient - avoid heavyweight frameworks like Colly (designed for complex scraping) in favor of focused, composable components.

Key risks center on Go concurrency pitfalls that cause production failures: goroutine leaks from blocked channels, HTTP response body leaks causing file descriptor exhaustion, and unbounded map growth in the URL deduplication layer. These are preventable with upfront patterns (separate goroutine for channel cleanup, defer resp.Body.Close(), LRU caches) but require architectural discipline from Phase 1. The Bubble Tea integration is well-documented but requires strict adherence to message-passing (no direct model mutation from goroutines) to avoid race conditions.

## Key Findings

### Recommended Stack

The Go ecosystem provides production-ready tools for every layer of a dead link checker. The Charm ecosystem (Bubble Tea v2, Lip Gloss v2, Bubbles) offers purpose-built TUI components with an Elm architecture that ensures predictable state management. Standard library packages (net/http, net/url, encoding/json) handle HTTP, URL parsing, and output formats without external dependencies. For CLI structure, Cobra + Viper is the industry standard (used by kubectl, Hugo, GitHub CLI).

**Core technologies:**
- **Bubble Tea v2**: TUI framework — Elm architecture provides predictable state, de facto standard for beautiful Go CLIs
- **net/http (custom client)**: HTTP requests — Standard library sufficient with proper configuration (timeouts, connection pooling, redirect limits)
- **golang.org/x/net/html**: HTML parsing — 2x faster than goquery for simple link extraction, HTML5 compliant
- **Cobra + Viper**: CLI framework — Industry standard, handles commands/flags/config with hierarchical precedence
- **Charm Log**: Structured logging — Human-readable output designed for CLI tools, wraps Go's slog
- **testify**: Test assertions — Reduces boilerplate, expressive failures, maintained actively

**Anti-recommendations:**
- **DO NOT** use http.DefaultClient (no timeout = hanging requests)
- **DO NOT** spawn unbounded goroutines (use worker pool pattern)
- **DO NOT** use Colly framework (massive overkill for static HTML link extraction)
- **DO NOT** use Bubble Tea v1 (v2 is current major version)

### Expected Features

Dead link checkers have well-established feature expectations from tools like lychee, hyperlink, and linkchecker. Users expect recursive crawling with depth limits, comprehensive error detection (4xx/5xx, timeouts, DNS failures, redirect chains), and configurable concurrency for performance control. The differentiator for zombiecrawl is its beautiful TUI experience (Charm ecosystem) - most competitors use plain text output.

**Must have (table stakes):**
- Recursive crawling with BFS strategy and depth limiting
- HTTP status detection (4xx, 5xx, timeouts, DNS failures)
- Redirect handling (3xx detection, chain tracking, loop detection)
- Internal vs external link distinction
- Configurable concurrency (default ~100 concurrent requests)
- robots.txt compliance and custom User-Agent
- Progress indication during crawl
- Summary report with counts (total, broken, redirected)
- Exit codes for CI integration (0=success, non-zero=failures)
- Human-readable output with colors/formatting

**Should have (competitive):**
- Beautiful TUI with Bubble Tea (zombiecrawl's main differentiator)
- Structured output formats (JSON, CSV) for automation
- Link source tracking (show WHERE broken link was found)
- Retry logic with exponential backoff for transient failures
- Pattern-based URL exclusions (regex or glob)
- Rate limiting per host (politeness)
- Verbose/quiet modes
- Custom headers for authenticated content

**Defer (v2+):**
- Incremental scanning (only check changed pages)
- Request caching for repeated checks
- Anchor/fragment validation (#anchor existence)
- Email address validation (mailto: links via SMTP)
- Sitemap.xml support as alternative input
- JavaScript rendering (headless browser, major complexity increase)
- Multiple input formats (Markdown, RST files)

### Architecture Approach

zombiecrawl follows a layered architecture separating UI (Bubble Tea), orchestration (URL frontier + worker pool manager), crawling (HTTP client + HTML parser), and validation (status code checking). The architecture centers on Go's concurrency primitives: a buffered channel acts as the URL queue, sync.Map or map+mutex tracks visited URLs to prevent loops, and a fixed worker pool (5-100 goroutines) provides bounded concurrency with backpressure. Bubble Tea wraps the crawler, using the Elm architecture (Model-View-Update) where all I/O happens in tea.Cmd functions that return messages to Update().

**Major components:**
1. **Crawler Orchestrator** — Manages URL frontier (channel-based queue), visited set (sync.Map), worker pool lifecycle, results aggregation
2. **Worker Pool** — Fixed goroutines pull URLs from queue, execute HTTP requests with per-host rate limiting via time.Ticker, handle retries with exponential backoff
3. **HTTP Client + Validator** — Custom http.Client with timeouts/connection pooling/redirect limits, validates status codes, drains response bodies for connection reuse
4. **HTML Parser** — Uses golang.org/x/net/html to extract links, resolves relative URLs with url.ResolveReference, filters same-domain vs external
5. **Bubble Tea TUI** — Renders progress (spinner, URLs checked count, dead links count), handles user input, orchestrates crawler lifecycle via tea.Cmd pattern
6. **Results Collector** — Aggregates dead links from results channel, generates reports (human-readable table, JSON, CSV)

**Critical patterns:**
- **Worker pool with bounded concurrency**: Fixed goroutines prevent resource exhaustion
- **Per-host rate limiting**: Dedicated worker per host with time.Ticker prevents server overload/IP bans
- **Bubble Tea command pattern**: All I/O in tea.Cmd, messages to Update(), never mutate model from goroutines
- **URL frontier with visited set**: Channel queue + concurrent-safe map prevents infinite loops
- **Retryable HTTP client**: Exponential backoff with jitter, classify retryable vs non-retryable errors

### Critical Pitfalls

The research identified 14 pitfalls across critical, moderate, and minor severity. The top 5 require architectural patterns from Phase 1 to avoid rewrites.

1. **Goroutine leaks from blocked channels** — Workers writing to full results channel block indefinitely while main goroutine waits on sync.WaitGroup. Prevention: Use separate goroutine to close results channel after WaitGroup completes. Detection: runtime.NumGoroutine() grows unbounded, memory leak.

2. **HTTP response body not closed** — Failing to defer resp.Body.Close() leaks file descriptors and prevents connection reuse. Prevention: Always defer close immediately after error check, drain body with io.Copy(io.Discard, resp.Body) for HTTP/1.x keep-alive. Detection: "too many open files" errors, performance degradation.

3. **Unbounded visited URL map growth** — Go maps don't reclaim memory after delete(), causing multi-gigabyte usage on large crawls. Prevention: Use LRU cache with bounded size or periodic map reinitialization. Detection: Memory usage correlates with total URLs discovered.

4. **Missing context timeout and cancellation** — Requests without context hang forever on dead servers, user can't cancel crawl. Prevention: Propagate context.Context through all goroutines, use context.WithTimeout for requests, select on ctx.Done() in loops. Detection: Crawler never completes, Ctrl+C doesn't stop.

5. **Poor HTTP client configuration** — DefaultClient uses only 2 connections per host and has no redirect limit. Prevention: Custom http.Client with MaxIdleConnsPerHost=20, Timeout=10s, CheckRedirect limiting to 10 hops. Detection: Crawl 10x slower than expected, infinite redirect loops.

**Additional moderate pitfalls:**
- **Incorrect relative URL resolution**: Use url.Parse + ResolveReference, not string concatenation
- **Naive retry logic**: Implement exponential backoff with jitter, classify retryable errors (5xx, timeouts) vs non-retryable (404, DNS, SSL)
- **URL deduplication doesn't normalize**: Sort query parameters, strip fragments, handle www prefix
- **HTML parsing with regex**: Use golang.org/x/net/html or goquery, never regex for HTML
- **Ignoring robots.txt**: Use robotstxt library, respect Crawl-delay directive

## Implications for Roadmap

Based on research, the suggested phase structure prioritizes foundational patterns (worker pool, HTTP client) before TUI integration, then adds polish (retry logic, output formats) after core functionality is proven.

### Phase 1: Core Crawler Foundation
**Rationale:** Establish concurrency patterns, HTTP client configuration, and URL handling before adding UI complexity. These patterns are hard to retrofit if done wrong initially (goroutine leaks, response body leaks require architectural rewrites).

**Delivers:** Working crawler that can recursively crawl a website, detect dead links (4xx/5xx), track visited URLs, and output results to terminal (basic text output, no TUI yet).

**Addresses:**
- Recursive crawling (table stakes)
- HTTP status detection (table stakes)
- Internal vs external link distinction (table stakes)
- Base URL configuration (table stakes)

**Implements:**
- Worker pool with bounded concurrency (avoids Pitfall 1, 4)
- Custom HTTP client with timeouts/connection pooling (avoids Pitfall 2, 5, 6)
- URL frontier + visited set with normalization (avoids Pitfall 3, 8)
- HTML parser using golang.org/x/net/html (avoids Pitfall 9)
- Context propagation for cancellation (avoids Pitfall 4)

**Avoids:**
- Pitfall 1 (goroutine leaks): Use separate goroutine for channel cleanup
- Pitfall 2 (body leaks): defer resp.Body.Close() + drain body
- Pitfall 3 (map growth): Start with simple map, document where to add LRU later
- Pitfall 4 (no timeouts): Custom http.Client with Timeout, context.WithTimeout
- Pitfall 5 (poor config): Set MaxIdleConnsPerHost=20, CheckRedirect limit

**Testing strategy:** Use net/http/httptest to mock HTTP servers, verify worker pool bounds, test URL normalization edge cases (www, fragments, query params).

### Phase 2: Bubble Tea TUI Integration
**Rationale:** With working crawler (Phase 1), add beautiful TUI to differentiate from competitors. Bubble Tea requires strict message-passing discipline (Pitfall 11) but is well-documented. This phase is isolated - TUI wraps crawler without modifying core logic.

**Delivers:** Real-time progress display (spinner, URLs checked count, dead links count, current URL), beautiful terminal output styled with Lip Gloss, graceful shutdown on Ctrl+C.

**Addresses:**
- Beautiful TUI (differentiator)
- Progress indication (table stakes)
- Colored output (should-have)

**Implements:**
- Bubble Tea Model-View-Update pattern (follows Pattern 6 from ARCHITECTURE.md)
- tea.Cmd wrapping for crawler (async I/O, follows Pattern 2)
- Message-passing for progress updates (avoids Pitfall 11)
- Spinner and progress components from Bubbles
- Lip Gloss styling for terminal output

**Avoids:**
- Pitfall 11 (state mutation from goroutines): Strict message-passing, use p.Send() for updates
- Pitfall 4 (no cancellation): Integrate context cancellation with Bubble Tea quit

**Dependencies:** Requires Phase 1 crawler to be working and exposing progress via channels/callbacks that can be converted to tea.Msg.

### Phase 3: Politeness and Reliability
**Rationale:** Before production use, implement web etiquette (robots.txt, rate limiting) and improve reliability (retry logic). These features prevent IP bans and handle transient failures gracefully.

**Delivers:** robots.txt compliance, per-host rate limiting with configurable delay, retry logic with exponential backoff, custom User-Agent with contact info.

**Addresses:**
- robots.txt compliance (table stakes)
- User-Agent header (table stakes)
- Retry logic (should-have differentiator)
- Rate limiting (should-have)

**Implements:**
- Per-host worker assignment with time.Ticker (Pattern 1 from ARCHITECTURE.md)
- robotstxt library integration
- Exponential backoff with jitter (Pattern 5)
- Retryable error classification (5xx, timeouts vs 404, DNS)

**Avoids:**
- Pitfall 10 (ignoring robots.txt): Use temoto/robotstxt or jimsmart/grobotstxt
- Pitfall 7 (naive retry): Classify errors, exponential backoff, don't retry 404/DNS
- Pitfall 12 (default User-Agent): Custom UA with project URL/contact
- Anti-Pattern 4 (no rate limiting): Per-host time.Ticker

**Dependencies:** Builds on Phase 1 worker pool, requires architectural refactor to assign hosts to workers.

### Phase 4: Output Formats and CI Integration
**Rationale:** With core functionality stable, add machine-readable output for automation and CI use cases. This phase is low-risk (uses standard library encoding packages).

**Delivers:** JSON and CSV output flags (--json, --csv), exit codes based on broken link count (0=success, 1=warnings, 2=errors), human-readable table output using tabwriter.

**Addresses:**
- Structured output (should-have differentiator)
- Exit codes (table stakes for CI)
- Human-readable summary (table stakes)

**Implements:**
- encoding/json for --json flag
- encoding/csv for --csv flag
- text/tabwriter for aligned terminal tables
- Exit code logic (0 if no broken links, 1 if warnings, 2 if errors)

**Avoids:**
- No specific pitfalls, straightforward implementation using standard library

**Dependencies:** Requires Phase 1 results collection, Phase 2 TUI for human-readable default.

### Phase 5: Advanced Features (Optional)
**Rationale:** Polish features that enhance usability but aren't critical for v1.0. These can be split into separate releases (v1.1, v1.2) based on user demand.

**Delivers:** Redirect chain tracking, redirect loop detection, depth limiting, pattern-based URL exclusions, verbose/quiet modes, link source tracking.

**Addresses:**
- Redirect chain tracking (table stakes)
- Redirect loop detection (table stakes)
- Depth limiting (table stakes)
- Pattern exclusions (should-have)
- Verbose/quiet modes (should-have)
- Link source tracking (should-have differentiator)

**Avoids:**
- Pitfall 3 (map growth): Redirect chain tracking adds to memory, may need LRU here

**Dependencies:** Builds on all previous phases, mostly additive features.

### Phase Ordering Rationale

- **Phase 1 first**: Concurrency pitfalls (1, 2, 3, 4, 5) are architectural - getting these wrong initially requires rewrites. Worker pool, HTTP client, and URL handling patterns must be correct from the start.
- **Phase 2 after Phase 1**: Bubble Tea TUI is well-documented and isolated (wraps crawler without modifying it). Easier to build TUI around working crawler than debug concurrency issues through a UI layer.
- **Phase 3 before production**: robots.txt and rate limiting are ethical requirements and prevent IP bans. Retry logic significantly improves reliability. These should be in place before public use.
- **Phase 4 for CI**: Output formats and exit codes enable automation/CI use cases. Low complexity (standard library), high value for certain users.
- **Phase 5 as polish**: Advanced features improve UX but aren't critical for launch. Can be deferred to v1.x releases based on feedback.

**Dependency chain:**
```
Phase 1 (core crawler) → Phase 2 (TUI) → Phase 3 (politeness) → Phase 4 (output) → Phase 5 (polish)
                ↓
         All subsequent phases depend on Phase 1 patterns
```

### Research Flags

Phases with standard patterns (skip research-phase during planning):
- **Phase 1**: Well-documented Go patterns (worker pool, HTTP client). Official docs + multiple authoritative sources confirm approach.
- **Phase 2**: Bubble Tea has official docs, tutorials, and active community. Pattern is established (Elm architecture).
- **Phase 4**: Standard library usage (encoding/json, encoding/csv), no research needed.

Phases that may benefit from targeted research:
- **Phase 3**: Per-host rate limiting pattern is described conceptually but lacks production reference implementations. May need research-phase for optimal implementation (worker assignment strategies, memory overhead of per-host workers).
- **Phase 5**: Link source tracking (fuzzy content matching in Markdown files) is mentioned in hyperlink docs but implementation unclear. May need research-phase if this feature is prioritized.

No phases require deep research - the project research covered all major areas comprehensively.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Official pkg.go.dev verified for all libraries, Charm ecosystem actively developed (Oct 2025 - Feb 2026 releases), versions confirmed as current major versions |
| Features | HIGH | Multiple production tools (lychee, hyperlink, linkchecker, muffet) show consensus on table stakes features, differentiators identified from competitive analysis |
| Architecture | MEDIUM | Worker pool and HTTP client patterns well-documented in official Go sources, Bubble Tea patterns from official docs + community tutorials, per-host rate limiting described conceptually but fewer production examples |
| Pitfalls | HIGH | Critical pitfalls (goroutine leaks, body leaks, context timeouts) documented in official Go docs and authoritative sources, moderate pitfalls verified across multiple implementations |

**Overall confidence:** MEDIUM-HIGH

The stack and features have high confidence (official sources, multiple verification points). Architecture is medium confidence for per-host rate limiting (conceptual pattern, needs validation during implementation) but high for other patterns. Pitfalls are high confidence (well-documented failure modes with clear prevention strategies).

### Gaps to Address

- **Scalability numbers are estimates**: Worker pool size recommendations (5-100 workers) and performance targets (100+ pages/second) are based on general Go performance characteristics, not zombiecrawl-specific benchmarks. Validate during Phase 1 implementation with realistic workloads.

- **Per-host rate limiting implementation details**: Pattern is clear (dedicated worker per host with time.Ticker) but worker assignment strategy needs design work. Options: (a) dynamic worker creation per host (simple, potential unbounded workers), (b) hash-based assignment to fixed pool (complex, bounded), (c) single queue with host-aware scheduling. Research suggests (a) for MVP, validate during Phase 3.

- **Bubble Tea version stability**: Bubble Tea v2.0.0-beta.6 and Lip Gloss v2.0.0-beta1 are beta releases (Oct 2025, Mar 2025). For a learning project this is acceptable, but production use should verify stable v2 releases exist. Run `go get -u` during initialization to fetch latest versions (likely stable by Feb 2026).

- **LRU cache library selection**: Research mentions using LRU cache for visited URLs (Pitfall 3) but doesn't specify library. Options: hashicorp/golang-lru (production-proven), groupcache/lru (minimal), DIY (educational). Defer decision to Phase 1 implementation - start with simple map, profile memory usage, add LRU if needed.

- **Visited set data structure trade-offs**: Research suggests sync.Map for high-concurrency reads vs map+sync.RWMutex. Actual choice depends on read/write ratio. Visited set is write-heavy during initial frontier expansion, then read-heavy. Profile during Phase 1 to validate sync.Map advantage.

## Sources

### Primary (HIGH confidence)
- **Official Go documentation**: net/http package docs (connection pooling, timeouts), context package (cancellation patterns), sync package (concurrency primitives)
- **pkg.go.dev**: Verified versions and APIs for Bubble Tea, Lip Gloss, Bubbles, Cobra, Viper, testify (Jan-Feb 2026)
- **Go by Example**: Worker pools pattern, goroutine patterns
- **Charm ecosystem GitHub repos**: Bubble Tea, Lip Gloss, Bubbles (official docs, examples, recent commits)

### Secondary (MEDIUM confidence)
- **Production crawler implementations**: muffet (Go link checker), gocheck (concurrent HTTP checker), sekika Go linkchecker blog post (Nov 2025)
- **Go blog posts (2025-2026)**: Worker pool implementations (elsyarifx, corentings), HTTP client tuning (maxgio crawler blog), Bubble Tea tutorials (sngeth, packagemain)
- **Competitive analysis**: lychee (Rust, async), hyperlink (fast CI), linkchecker (mature Python), markdown-link-check (focused)
- **Web crawling guides**: ZenRows Go crawler guide (2026), Enjoy Algorithms crawler system design

### Tertiary (LOW confidence - needs validation)
- **Performance claims**: net/html is 2x faster than goquery (from 2014 benchmark, needs current verification)
- **Version warnings**: Several libraries showed "not latest" on pkg.go.dev (Cobra v1.10.2 Dec 2025, Viper v1.21.0 Sep 2025) - newer versions likely exist, run go get -u
- **Scalability estimates**: Worker count recommendations (100 concurrent), page speed targets (100+ pages/second) are general estimates, not zombiecrawl-specific

**Note on sources:** No Context7 libraries were available for this research (Bubble Tea, Colly, etc. not in knowledge base). All research used WebSearch with verification across multiple sources. Official documentation and Go standard library docs provide high confidence for critical areas (concurrency, HTTP client). Bubble Tea patterns verified through official GitHub repo and recent community tutorials (2025-2026).

---
*Research completed: 2026-02-13*
*Ready for roadmap: yes*
