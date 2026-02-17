# Roadmap: zombiecrawl

## Overview

zombiecrawl delivers a beautiful CLI dead link checker in five phases. We start with core crawler foundations (worker pool, HTTP client, URL handling) to establish concurrency patterns that prevent architectural rewrites. Next comes Bubble Tea TUI integration for differentiated user experience. Then we add politeness and reliability (robots.txt, retry logic). Configuration and structured output enable CLI flexibility and CI integration. Finally, polish and edge cases prepare for production use.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Core Crawler Foundation** - Worker pool, HTTP client, basic crawling and link detection
- [ ] **Phase 2: Bubble Tea TUI** - Live progress display and beautiful terminal output
- [ ] **Phase 3: Politeness & Reliability** - robots.txt, retry logic, comprehensive error handling
- [ ] **Phase 4: Configuration & Output** - CLI flags, depth control, structured formats
- [ ] **Phase 5: Production Polish** - Edge case handling, performance optimization, final refinements

## Phase Details

### Phase 1: Core Crawler Foundation
**Goal**: Working recursive crawler that detects dead links with proper concurrency patterns
**Depends on**: Nothing (first phase)
**Requirements**: CRWL-01, CRWL-02, CRWL-03, CRWL-04, CRWL-06, DETC-01, DETC-02, DETC-05
**Success Criteria** (what must be TRUE):
  1. User can run `zombiecrawl <url>` and see basic text output listing broken links
  2. Tool recursively follows all same-domain anchor tags without infinite loops
  3. External links are validated but not crawled further
  4. Tool detects 4xx and 5xx status codes as broken links
  5. Tool treats 3xx redirects as valid (not broken)
  6. Concurrent requests execute without goroutine leaks or resource exhaustion
**Plans**: 3 plans

Plans:
- [ ] 01-01-PLAN.md — URL utilities (normalize, filter, resolve) via TDD
- [ ] 01-02-PLAN.md — Crawler core (worker pool, HTTP client, link extraction)
- [ ] 01-03-PLAN.md — CLI entry point, output formatting, end-to-end integration

### Phase 2: Bubble Tea TUI
**Goal**: Beautiful real-time progress display that differentiates zombiecrawl from competitors
**Depends on**: Phase 1
**Requirements**: OUTP-01, OUTP-02, OUTP-05, OUTP-06
**Success Criteria** (what must be TRUE):
  1. User sees live TUI with spinner, URLs checked count, broken links count during crawl
  2. Tool displays pretty formatted summary table after crawl completes (Lip Gloss)
  3. User can gracefully stop crawl with Ctrl+C
  4. Tool exits with code 0 when no broken links found
  5. Tool exits with non-zero code when broken links found
**Plans**: 2 plans

Plans:
- [ ] 02-01-PLAN.md — Refactor crawler to emit events via channel (remove stdout writes)
- [ ] 02-02-PLAN.md — Bubble Tea TUI model, Lip Gloss styles, main.go integration

### Phase 3: Politeness & Reliability
**Goal**: Web-crawler etiquette and resilient error handling for production use
**Depends on**: Phase 2
**Requirements**: CRWL-07, DETC-03, DETC-04, DETC-06, DETC-07, DETC-08
**Success Criteria** (what must be TRUE):
  1. Tool respects robots.txt directives and skips disallowed URLs
  2. Tool retries failed requests with exponential backoff before marking broken
  3. Tool detects and reports timeout failures as broken links
  4. Tool detects and reports DNS resolution failures as broken links
  5. Tool detects and reports redirect loops as broken
  6. Each broken link report shows the source page where it was found
**Plans**: 4 plans

Plans:
- [ ] 03-01-PLAN.md — Error classification types and rate limiting foundation
- [ ] 03-02-PLAN.md — robots.txt compliance with caching
- [ ] 03-03-PLAN.md — Retry logic and redirect loop detection
- [ ] 03-04-PLAN.md — CLI flags and grouped error display

### Phase 4: Configuration & Output
**Goal**: CLI flexibility and machine-readable output for automation/CI use cases
**Depends on**: Phase 3
**Requirements**: CRWL-05, OUTP-03, OUTP-04
**Success Criteria** (what must be TRUE):
  1. User can limit crawl depth with --depth flag
  2. User can get JSON output via --json flag
  3. User can get CSV output via --csv flag
  4. Structured output includes all link details (URL, status, source page, error type)
**Plans**: TBD

Plans:
- [ ] 04-01: TBD
- [ ] 04-02: TBD

### Phase 5: Production Polish
**Goal**: Edge case handling and refinements for reliable production use
**Depends on**: Phase 4
**Requirements**: None (all v1 requirements covered in Phases 1-4)
**Success Criteria** (what must be TRUE):
  1. Tool handles edge cases gracefully (malformed HTML, invalid URLs, connection errors)
  2. Memory usage remains bounded on large crawls (10,000+ pages)
  3. Tool includes helpful error messages for common user mistakes
  4. Performance meets target of crawling 50+ pages/second on typical sites
**Plans**: TBD

Plans:
- [ ] 05-01: TBD
- [ ] 05-02: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Core Crawler Foundation | 0/3 | Planned | - |
| 2. Bubble Tea TUI | 0/2 | Planned | - |
| 3. Politeness & Reliability | 0/4 | Planned | - |
| 4. Configuration & Output | 0/2 | Not started | - |
| 5. Production Polish | 0/2 | Not started | - |
