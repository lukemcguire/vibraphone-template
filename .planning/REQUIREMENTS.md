# Requirements: zombiecrawl

**Defined:** 2026-02-13
**Core Value:** Reliably find every dead link on a website by crawling all same-domain pages and checking every outbound link — with output that's prettier than it has any right to be.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Crawling

- [ ] **CRWL-01**: User can provide a URL to crawl as a CLI argument
- [ ] **CRWL-02**: Tool recursively follows all anchor tag hrefs on same-domain pages (BFS)
- [ ] **CRWL-03**: External links are checked for reachability but not crawled further
- [ ] **CRWL-04**: Each discovered URL is checked exactly once (deduplication)
- [ ] **CRWL-05**: User can limit crawl depth via --depth flag
- [ ] **CRWL-06**: User can set concurrent request count via --concurrency flag (default 10)
- [ ] **CRWL-07**: Tool respects robots.txt directives

### Detection

- [ ] **DETC-01**: Tool reports links returning 4xx status codes as broken
- [ ] **DETC-02**: Tool reports links returning 5xx status codes as broken
- [ ] **DETC-03**: Tool reports links that timeout as broken
- [ ] **DETC-04**: Tool reports links with DNS resolution failures as broken
- [ ] **DETC-05**: Tool treats 3xx redirects as valid (not broken)
- [ ] **DETC-06**: Tool detects and reports redirect loops
- [ ] **DETC-07**: Tool retries failed requests with exponential backoff before marking broken
- [ ] **DETC-08**: Each broken link report includes the source page where it was found

### Output

- [ ] **OUTP-01**: Tool displays live TUI progress during crawl (pages checked, links found, errors)
- [ ] **OUTP-02**: Tool displays pretty formatted summary after crawl completes
- [ ] **OUTP-03**: User can get JSON output via --json flag
- [ ] **OUTP-04**: User can get CSV output via --csv flag
- [ ] **OUTP-05**: Tool exits with code 0 when no broken links found
- [ ] **OUTP-06**: Tool exits with non-zero code when broken links are found

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### JavaScript Rendering

- **JSRN-01**: Tool can check links on JavaScript-rendered pages
- **JSRN-02**: Tool uses headless browser for JS execution

### Advanced Features

- **ADVN-01**: User can exclude URLs matching patterns (--exclude flag)
- **ADVN-02**: User can provide custom HTTP headers
- **ADVN-03**: Tool checks anchor/fragment references (#) exist on target pages
- **ADVN-04**: User can check links in local files (Markdown, HTML)
- **ADVN-05**: Tool supports sitemap.xml as input source
- **ADVN-06**: User can configure verbose/quiet output modes

## Out of Scope

| Feature | Reason |
|---------|--------|
| JavaScript rendering (v1) | Complexity explosion, deferred to v2 |
| GUI / web dashboard | CLI tool — TUI is the interface |
| Auto-fix broken links | Dangerous, report-only tool |
| SEO scoring / analysis | Different domain, stay focused on link checking |
| Email (mailto:) validation | Out of scope for URL link checking |
| Database persistence | Over-engineered for a CLI tool |
| Plugin system | YAGNI — keep it simple |
| Performance monitoring | Report link status, not response times |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| CRWL-01 | Phase 1 | Pending |
| CRWL-02 | Phase 1 | Pending |
| CRWL-03 | Phase 1 | Pending |
| CRWL-04 | Phase 1 | Pending |
| CRWL-05 | Phase 4 | Pending |
| CRWL-06 | Phase 1 | Pending |
| CRWL-07 | Phase 3 | Pending |
| DETC-01 | Phase 1 | Pending |
| DETC-02 | Phase 1 | Pending |
| DETC-03 | Phase 3 | Pending |
| DETC-04 | Phase 3 | Pending |
| DETC-05 | Phase 1 | Pending |
| DETC-06 | Phase 3 | Pending |
| DETC-07 | Phase 3 | Pending |
| DETC-08 | Phase 3 | Pending |
| OUTP-01 | Phase 2 | Pending |
| OUTP-02 | Phase 2 | Pending |
| OUTP-03 | Phase 4 | Pending |
| OUTP-04 | Phase 4 | Pending |
| OUTP-05 | Phase 2 | Pending |
| OUTP-06 | Phase 2 | Pending |

**Coverage:**
- v1 requirements: 21 total
- Mapped to phases: 21
- Unmapped: 0 ✓

---
*Requirements defined: 2026-02-13*
*Last updated: 2026-02-13 after roadmap creation*
