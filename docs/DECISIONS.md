# DECISIONS.md — Architecture Decision Records

Manually authored. The agent may draft entries; a human approves. This is not
an audit log — it captures significant architectural decisions and their
rationale.

---

## Format

Each ADR follows this structure:

```markdown
## ADR-NNN: [Title]

- **Date:** YYYY-MM-DD
- **Status:** Accepted | Superseded by ADR-XXX
- **Context:** Why was this decision needed?
- **Decision:** What was decided
- **Consequences:** Trade-offs accepted
```

---

## ADR-001: Use Vibraphone Framework for Agent-Driven Development

- **Date:** 2026-02-12
- **Status:** Accepted
- **Context:** The project needs a structured approach to AI-assisted
  development that enforces code quality through tooling rather than relying
  on agent compliance with prompt instructions.
- **Decision:** Adopt the Vibraphone framework with MCP tools for execution,
  GSD for planning, and Beads for task management.
- **Consequences:** Agents must use MCP tools for all git, test, and review
  operations. Direct shell commands for these operations are prohibited. This
  adds overhead but guarantees quality gate enforcement.

## ADR-003: Use Charm Ecosystem for Terminal UI

- **Date:** 2026-02-15
- **Status:** Accepted
- **Context:** Phase 2 requires a live terminal UI with spinner, progress
  counters, and styled summary tables. The main options are: (1) raw ANSI escape
  codes, (2) the Charm ecosystem (Bubble Tea + Lip Gloss + Bubbles), or
  (3) other TUI frameworks like tview or termui.
- **Decision:** Use charmbracelet/bubbletea for the Elm-architecture TUI loop,
  charmbracelet/lipgloss for styling and table rendering, and
  charmbracelet/bubbles for the spinner component.
- **Consequences:** Adds three dependencies from the Charm ecosystem. Bubble Tea's
  Elm architecture (Init/Update/View) provides a clean separation of concerns and
  testable state transitions. Lip Gloss gives styled output without manual ANSI
  codes. The trade-off is a larger dependency tree compared to raw escape codes,
  but the Charm libraries are widely adopted, well-maintained, and composable.

## ADR-002: Use golang.org/x/net for HTML Parsing

- **Date:** 2026-02-15
- **Status:** Accepted
- **Context:** The crawler needs to extract links from HTML pages. Go's standard
  library does not include an HTML tokenizer/parser. The two main options are
  golang.org/x/net/html (official Go sub-repository) and third-party libraries
  like goquery (which itself wraps x/net/html).
- **Decision:** Use golang.org/x/net/html directly for HTML tokenization and
  link extraction.
- **Consequences:** Minimal dependency footprint — x/net is maintained by the Go
  team with the same compatibility guarantees as the standard library. Using the
  tokenizer directly gives fine-grained control over parsing without the overhead
  of a full DOM tree.

## ADR-004: Use golang.org/x/time/rate for Rate Limiting

- **Date:** 2026-02-17
- **Status:** Accepted
- **Context:** The crawler needs to limit request rate to avoid overwhelming
  target servers. The two main options are: (1) implement a custom token bucket,
  or (2) use golang.org/x/time/rate which provides a well-tested implementation.
- **Decision:** Use golang.org/x/time/rate.Limiter for request rate limiting.
- **Consequences:** Adds one dependency from the official Go x repository. The
  rate.Limiter provides a token bucket implementation with context-aware waiting,
  burst support, and proper cancellation handling. This is a minimal dependency
  with the same stability guarantees as the standard library.

## ADR-005: Use golang.org/x/sync/errgroup for Goroutine Lifecycle

- **Date:** 2026-02-17
- **Status:** Accepted
- **Context:** The crawler's worker pool needs structured goroutine management
  with proper cancellation propagation and error handling. Options include: (1)
  raw goroutines with manual WaitGroup management, or (2) golang.org/x/sync/errgroup
  which provides structured concurrency with built-in context cancellation.
- **Decision:** Use golang.org/x/sync/errgroup for managing worker goroutines.
- **Consequences:** Adds one dependency from the official Go x repository. errgroup
  ensures all workers terminate on context cancellation and provides a single
  point to wait for all goroutines to complete. The trade-off is slightly more
  complex coordination with the existing WaitGroup for job tracking, but it
  guarantees deterministic goroutine lifecycle management.

## ADR-006: Use github.com/temoto/robotstxt for robots.txt Compliance

- **Date:** 2026-02-17
- **Status:** Accepted
- **Context:** The crawler needs to respect robots.txt directives for ethical
  crawling. Options include: (1) implement a custom parser, (2) use the
  temoto/robotstxt library. A custom parser would need to handle the full
  robots.txt specification including user-agent matching, allow/disallow rules,
  crawl-delay, and sitemap directives.
- **Decision:** Use github.com/temoto/robotstxt for robots.txt parsing and
  compliance checking.
- **Consequences:** Adds one third-party dependency. The library handles the
  complexity of the robots.txt specification including edge cases like
  case-insensitive matching, pattern wildcards, and multiple user-agent groups.
  The trade-off is external dependency maintenance risk, but the library is
  widely used (3000+ GitHub stars), actively maintained, and handles corner
  cases that a custom implementation might miss. We wrap the library with our
  own RobotsChecker type to provide caching and fail-open behavior.

## ADR-007: Disk-Backed Bloom Filter for URL Deduplication

- **Date:** 2026-02-17
- **Status:** Accepted
- **Context:** The crawler needs to track visited URLs to avoid revisiting pages
  during BFS crawling. The previous implementation used sync.Map for in-memory
  tracking, which has unbounded memory growth proportional to unique URLs. For
  production-scale crawls targeting 100,000+ pages, this can cause OOM errors.
- **Decision:** Use a disk-backed bloom filter with memory-mapped file I/O for
  URL deduplication. Dependencies: github.com/bits-and-blooms/bloom/v3 for the
  bloom filter implementation and github.com/edsrzf/mmap-go for memory-mapped
  file I/O.
- **Consequences:** O(1) space and time complexity regardless of URL count.
  Configured for 100K URLs with 0.1% false positive rate (~1 in 1000 URLs may
  be incorrectly skipped). Memory-mapped file provides constant memory footprint
  with OS-managed paging. Temp files are created in OS temp directory and
  cleaned up on Close(). The trade-off is acceptable false positives for bounded
  memory, and the bloom filter library handles optimal parameter calculation.

## ADR-008: Memory Pressure Monitoring with SetMemoryLimit

- **Date:** 2026-02-17
- **Status:** Accepted
- **Context:** For long-running crawls, the process may consume increasing memory.
  Without monitoring, the OS may kill the process with OOM, losing all progress.
- **Decision:** Implement memory pressure monitoring using runtime/debug.SetMemoryLimit
  (Go 1.19+) and runtime.ReadMemStats for current memory statistics.
- **Consequences:** No external dependencies required - uses native Go runtime
  support. Provides warning level at 75% and critical level at 90% of memory
  limit. Callback mechanism allows crawlers to adapt behavior. Default limit of
  1GB for CLI tool (configurable). The soft limit means GC works harder before
  OOM, but the process isn't killed by the OS.
