# Feature Landscape

**Domain:** Dead Link Checker CLI Tools
**Researched:** 2026-02-13

## Table Stakes

Features users expect. Missing = product feels incomplete.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Recursive crawling | Core function - must follow links within site | Medium | BFS vs DFS strategy choice |
| HTTP status detection (4xx, 5xx) | Primary purpose - detect broken links | Low | 404, 410, 500, 502, 503, etc. |
| Timeout detection | Slow/hanging servers are effectively broken | Low | Configurable timeout threshold |
| DNS failure detection | Site doesn't resolve = dead link | Low | Network-level errors |
| Redirect detection (3xx) | Need to know if redirects are working | Low | 301, 302, 307, 308 tracking |
| Redirect chain tracking | Multi-hop redirects can hide issues | Medium | Follow up to 9+ redirects typical |
| Redirect loop detection | Prevent infinite loops | Low | Critical for robustness |
| Internal vs external link distinction | Different handling for each type | Low | Stay on domain vs leave domain |
| Base URL configuration | Start point for crawl | Low | CLI argument or config file |
| Configurable concurrency | Performance - users expect control | Low | Default ~100 concurrent requests |
| robots.txt compliance | Respect site preferences by default | Medium | Standard web crawler behavior |
| User-Agent header | Identify crawler to servers | Low | Custom user-agent string |
| Progress indication | Visual feedback during long scans | Low | Progress bar or percentage |
| Summary report | Final count of broken/working links | Low | Total, broken, redirected counts |
| Exit codes | CI integration requires non-zero on failure | Low | 0=success, 1=warnings, 2=errors |
| Human-readable output | Default output format | Low | Colored, formatted terminal output |

## Differentiators

Features that set product apart. Not expected, but valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Beautiful TUI (Charm/Bubble Tea) | Makes tool delightful vs utilitarian | Medium | zombiecrawl's main differentiator |
| Structured output (JSON, CSV) | Enables automation/integration | Low | Machine-parseable results |
| Link source tracking | Shows WHERE broken link was found | Medium | Maps errors back to source files |
| Fuzzy content matching | Find link context in Markdown files | Medium | Like hyperlink's feature |
| Retry logic with exponential backoff | Handles transient failures gracefully | Medium | Respect Retry-After headers |
| Request caching | Speed up repeated checks | Medium | Cache valid responses |
| Incremental scanning | Only check changed pages | High | Dramatically faster on large sites |
| Custom headers support | Access authenticated/protected content | Low | Bearer tokens, cookies, etc. |
| Cookie support | Maintain session state | Medium | Stateful crawling |
| Basic auth support | Protected site access | Low | Username/password |
| Pattern-based exclusions | Skip URLs matching patterns | Low | Regex or glob patterns |
| IP range exclusions | Skip private/local/loopback IPs | Low | Avoid false positives |
| Status code filtering | Customize what counts as "broken" | Low | Accept custom status codes |
| Anchor/fragment checking | Verify #anchors exist on pages | Medium | Deep link validation |
| Email address validation | Check mailto: links via SMTP | Medium | Like lychee feature |
| Sitemap support (XML) | Use sitemap.xml as input | Medium | Alternative to recursive crawl |
| Multiple input formats | Markdown, HTML, RST, plain text | Medium | File vs URL starting points |
| GitHub token integration | Higher rate limits for GitHub links | Low | Environment variable support |
| Depth limiting | Control crawl depth | Low | Prevent runaway crawls |
| Rate limiting | Respect server capacity | Medium | Requests per second throttling |
| Verbose/quiet modes | Control output detail level | Low | -v, -q flags |
| Colored output | Better readability | Low | Terminal color support |

## Anti-Features

Features to explicitly NOT build.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| JavaScript rendering (v1) | Complexity explosion, slow, heavy deps | Document as future enhancement, static HTML only for v1 |
| Headless browser integration (v1) | Resource-intensive, slow, complex | Use lightweight HTTP client for v1 |
| GUI interface | Out of scope for CLI tool | Focus on excellent TUI instead |
| Web dashboard | Scope creep, maintenance burden | Provide JSON output for external tools |
| Database persistence | Over-engineered for v1 | Use file-based cache if needed |
| Plugin system | YAGNI for v1 | Keep it simple, add if demand exists |
| Auto-fix broken links | Dangerous, could corrupt sites | Report only, let users fix manually |
| Spell checking content | Out of scope | Focus on link validation only |
| SEO scoring | Feature creep | Stay focused on link checking |
| Performance monitoring | Different domain | Report response times, not full perf analysis |
| Link building suggestions | Marketing tool, not validation | Stay technical |
| Bulk URL operations without crawling | Different use case | Focus on site crawling, not list checking |

## Feature Dependencies

```
Recursive crawling → robots.txt compliance
Recursive crawling → Depth limiting
Redirect detection → Redirect chain tracking
Redirect detection → Redirect loop detection
Request caching → Incremental scanning
Custom headers → Cookie support
Custom headers → Basic auth support
Anchor checking → HTML parsing (deeper than link extraction)
Sitemap support → XML parsing
Structured output → Exit codes (for CI use)
Progress indication → TUI framework (Charm/Bubble Tea)
Beautiful output → TUI framework (Charm/Bubble Tea)
```

## MVP Recommendation

Prioritize:
1. **Recursive crawling** (BFS, depth limit, robots.txt)
2. **Core link detection** (4xx, 5xx, timeouts, DNS failures)
3. **Redirect handling** (3xx detection, chains, loops)
4. **Configurable concurrency** (performance)
5. **Basic TUI** (progress bar, colored output via Charm)
6. **Human-readable report** (summary with counts)
7. **JSON output** (--json flag for scripting)
8. **Exit codes** (CI integration)

Defer to v1.1+:
- **Incremental scanning**: High complexity, optimize later
- **Email validation**: Nice-to-have, not core
- **Anchor checking**: Adds parsing complexity
- **Sitemap support**: Alternative input method, not critical
- **Advanced auth** (cookies, custom headers): Wait for user demand
- **Request caching**: Optimization, add when needed

Defer to v2.0+:
- **JavaScript rendering**: Major complexity, different tool category
- **Headless browser**: Resource-intensive, changes performance profile
- **Multiple input formats**: Focus on URLs first, add file parsing later

## Feature Confidence Assessment

| Category | Confidence | Source Quality |
|----------|------------|----------------|
| Table stakes | HIGH | Multiple CLI tools (lychee, hyperlink, linkchecker) all implement these |
| Differentiators | MEDIUM | Features observed in leading tools, but prioritization based on zombiecrawl's goals |
| Anti-features | MEDIUM | Based on scope creep patterns in similar tools and v1 constraints |
| Dependencies | HIGH | Technical dependencies are clear from implementation requirements |

## Competitive Feature Analysis

### High-Performance Tools (lychee, hyperlink)
- **Strengths**: Speed (Rust), async/parallel, caching, GitHub integration
- **zombiecrawl adoption**: Concurrency, structured output, GitHub token support

### Traditional Tools (LinkChecker)
- **Strengths**: Mature, comprehensive, many output formats
- **zombiecrawl adoption**: Multiple output formats, robots.txt compliance

### Modern CLI Tools (markdown-link-check)
- **Strengths**: Focused, good progress output, CI-friendly
- **zombiecrawl adoption**: Exit codes, quiet/verbose modes

### zombiecrawl's Niche
- **Beautiful TUI** (Charm ecosystem) - most link checkers use plain text
- **Go implementation** (learning goal) - leverage Go's concurrency
- **Human-friendly first** - most tools optimize for machines
- **Static HTML focus** (v1) - clear scope, room to expand

## Research Notes

### Performance Expectations
- **hyperlink**: 700 HTML pages in 220ms (default), 850ms (with anchor checking)
- **lychee**: Async, stream-based, handles large sites efficiently
- **linkinator**: Default 100 concurrent connections

**zombiecrawl target**: Aim for 100+ pages/second on typical sites with configurable concurrency

### Output Format Patterns
- **Standard**: Text with colors/emoji for human reading
- **CI/Automation**: JSON, CSV, XML for parsing
- **Advanced**: Sitemap graphs, HTML reports (defer to v2)

**zombiecrawl approach**: Human-readable default (Charm TUI) + --json flag for automation

### Error Detection Philosophy
- **Strict**: Any non-2xx is an error (may have false positives)
- **Lenient**: Only 4xx/5xx are errors (may miss redirect issues)
- **Configurable**: Let users define acceptable status codes

**zombiecrawl recommendation**: Start strict (non-2xx = warning/error), add --accept-status flag later

## Sources

### Tool Documentation & Repositories
- [Lychee - Fast, async link checker in Rust](https://github.com/lycheeverse/lychee)
- [Hyperlink - Very fast link checker for CI](https://github.com/untitaker/hyperlink)
- [LinkChecker - Check websites for broken links](https://wummel.github.io/linkchecker/)
- [Linkinator - Broken Link Checker & Website Crawler](https://jbeckwith.com/projects/linkinator)
- [markdown-link-check - Check hyperlinks in markdown](https://github.com/tcort/markdown-link-check)

### Feature Research
- [Top 10 Broken Link Checker Tools 2026](https://www.softwaretestinghelp.com/broken-link-checker/)
- [21 Open-source tools for broken links](https://medevel.com/os-broken-link-checkers-to-improve-your-seo/)
- [Best Open-Source Web Crawlers 2026](https://www.firecrawl.dev/blog/best-open-source-web-crawler)
- [Top Web Crawler Tools 2026](https://scrapfly.io/blog/posts/top-web-crawler-tools)
- [Screaming Frog SEO Spider](https://www.screamingfrog.co.uk/seo-spider/)

### Technical Implementation
- [Linkinator Configuration](https://www.tkcnn.com/github/JustinBeckwith/linkinator.html)
- [LinkChecker Man Page](https://linux.die.net/man/1/linkchecker)
- [W3C Link Checker Documentation](https://dev.w3.org/perl/modules/W3C/LinkChecker/docs/checklink)
- [Lychee Documentation](https://lychee.cli.rs/overview/)

### CI Integration
- [Lychee GitHub Action](https://github.com/lycheeverse/lychee-action)
- [Hyperlink Link Checker Action](https://github.com/marketplace/actions/hyperlink-link-checker)
- [Check links with linkcheck Action](https://github.com/marketplace/actions/check-links-with-linkcheck)
- [URLChecker Action](https://github.com/marketplace/actions/urlchecker-action)

### Crawling Strategies
- [Web Crawling using BFS at specified depth](https://www.geeksforgeeks.org/python/web-crawling-using-breadth-first-search-at-a-specified-depth/)
- [Deep Crawling - Crawl4AI](https://docs.crawl4ai.com/core/deep-crawling/)
- [Dealing with Rate Limiting Using Exponential Backoff](https://substack.thewebscraping.club/p/rate-limit-scraping-exponential-backoff)
- [How to Fix API 429 Rate Limit Error](https://www.aifreeapi.com/en/posts/claude-api-429-error-fix)

### TUI Development
- [Building TUI Using Go, Bubble Tea, Lip Gloss](https://www.grootan.com/blogs/building-an-awesome-terminal-user-interface-using-go-bubble-tea-and-lip-gloss/)
- [Bubble Tea - TUI framework](https://github.com/charmbracelet/bubbletea)
- [Rapidly building interactive CLIs with Bubbletea](https://www.inngest.com/blog/interactive-clis-with-bubbletea)
- [Developing terminal UI in Go with Bubble Tea](https://packagemain.tech/p/terminal-ui-bubble-tea)

### Configuration & Patterns
- [W3C Link Checker and Robots.txt](https://codecharismatic.com/w3c-link-checker-and-robots-txt-exclusion/)
- [LinkChecker FAQ - Ignore URLs](https://wummel.github.io/linkchecker/faq.html)
- [Broken Links: Common Causes and Fixes](https://www.semrush.com/blog/broken-link/)
- [Headless Browser vs Static HTML](https://www.nimbleway.com/blog/headless-browser-scraping-guide)
