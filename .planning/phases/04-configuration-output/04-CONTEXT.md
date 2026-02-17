# Phase 4: Configuration & Output - Context

**Gathered:** 2026-02-17
**Status:** Ready for planning

<domain>
## Phase Boundary

CLI flags and machine-readable output formats (JSON, CSV) for automation/CI use cases. Includes depth control, output formatting, and file output. This phase does NOT add new crawling capabilities — it exposes configuration for existing behavior.

</domain>

<decisions>
## Implementation Decisions

### Output format details
- **Error representation:** Separate fields in JSON/CSV — `status_code` for HTTP codes (e.g., 404), `error_type` for non-HTTP errors (e.g., "timeout", "dns")
- **Empty CSV behavior:** Output header row only if no broken links found

### Flag design
- **Flag style:** Both short and long forms (e.g., `-d` and `--depth`)
- **Default depth:** Unlimited — if `--depth` not specified, crawl everything reachable from start URL

### Depth control
- **Depth indexing:** 0-indexed — start URL is depth 0, links from it are depth 1
- **Depth scope:** Applies to same-domain crawling only; external links are validated but never followed regardless of depth
- **Depth 1 meaning:** Check start URL plus all direct links found on it (no further recursion)
- **Depth display:** Not shown in output — just enforced as a limit

### Output destination
- **TUI + file:** TUI always displays (beautiful output is core value); `-o/--output` writes JSON/CSV to file *in addition* to TUI
- **File timing:** File written at crawl completion (not streaming during crawl)
- **Overwrite behavior:** Silently overwrite existing files
- **Quiet mode:** None — always show beautiful TUI output

### Claude's Discretion
- JSON output structure (flat array vs grouped vs wrapped with metadata)
- CSV column order
- Specific short flag letters for each option
- Behavior when both `--json` and `--csv` specified

</decisions>

<specifics>
## Specific Ideas

- "We are all about our beautiful goddamn display" — TUI always shows, file output is additive
- No quiet/silent mode — the beautiful output is the product differentiator

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 04-configuration-output*
*Context gathered: 2026-02-17*
