# Phase 4: Configuration & Output - Research

**Researched:** 2026-02-17
**Domain:** CLI flag handling, structured output formats (JSON/CSV), BFS depth limiting
**Confidence:** HIGH

## Summary

Phase 4 adds configuration flags for crawl depth limiting and machine-readable output formats (JSON/CSV) for automation and CI use cases. The Go standard library provides excellent support for all requirements: `flag` package for CLI parsing (already in use), `encoding/json` for JSON output, and `encoding/csv` for CSV output.

The existing codebase already uses the standard `flag` package and has well-structured result types (`result.Result`, `result.LinkResult`) with JSON-friendly field types. Adding depth limiting requires extending `CrawlJob` with a depth field and modifying the BFS coordinator logic to track and respect depth limits.

**Primary recommendation:** Continue using Go standard library throughout. For depth control, add `Depth` field to `CrawlJob` and track depth in the coordinator. For output, use `encoding/json.Marshal` and `encoding/csv.Writer` directly on existing struct types with appropriate JSON tags.

---

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Output format details:**
- Error representation: Separate fields in JSON/CSV - `status_code` for HTTP codes (e.g., 404), `error_type` for non-HTTP errors (e.g., "timeout", "dns")
- Empty CSV behavior: Output header row only if no broken links found

**Flag design:**
- Flag style: Both short and long forms (e.g., `-d` and `--depth`)
- Default depth: Unlimited - if `--depth` not specified, crawl everything reachable from start URL

**Depth control:**
- Depth indexing: 0-indexed - start URL is depth 0, links from it are depth 1
- Depth scope: Applies to same-domain crawling only; external links are validated but never followed regardless of depth
- Depth 1 meaning: Check start URL plus all direct links found on it (no further recursion)
- Depth display: Not shown in output - just enforced as a limit

**Output destination:**
- TUI + file: TUI always displays (beautiful output is core value); `-o/--output` writes JSON/CSV to file *in addition* to TUI
- File timing: File written at crawl completion (not streaming during crawl)
- Overwrite behavior: Silently overwrite existing files
- Quiet mode: None - always show beautiful TUI output

### Claude's Discretion

- JSON output structure (flat array vs grouped vs wrapped with metadata)
- CSV column order
- Specific short flag letters for each option
- Behavior when both `--json` and `--csv` specified

### Deferred Ideas (OUT OF SCOPE)

None - discussion stayed within phase scope

</user_constraints>

---

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CRWL-05 | User can limit crawl depth via --depth flag | Add `Depth` field to `CrawlJob`, track depth in BFS coordinator, check depth before enqueueing discovered links |
| OUTP-03 | User can get JSON output via --json flag | Use `encoding/json.Marshal` on `result.LinkResult` slice with JSON struct tags for field naming |
| OUTP-04 | User can get CSV output via --csv flag | Use `encoding/csv.Writer` with manual column ordering, implement `error_type` vs `status_code` separation |

</phase_requirements>

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| flag | stdlib | CLI flag parsing | Already in use, supports --long and -short forms, no external dependencies needed |
| encoding/json | stdlib | JSON serialization | RFC 7159 compliant, struct tags for field customization, widely battle-tested |
| encoding/csv | stdlib | CSV output | RFC 4180 compliant, handles quoting/escaping correctly, straightforward API |
| os | stdlib | File I/O for -o flag | `os.Create` for file output, simple overwrite behavior |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| io | stdlib | Multi-writer for dual output | If writing to both stdout and file simultaneously |
| fmt | stdlib | Error messages | Flag validation errors |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| stdlib flag | spf13/pflag, cobra, urfave/cli | External deps for just a few more flags is overkill; current flag usage works well |
| encoding/json | json-iterator/go, goccy/go-json | Performance not critical for output; stdlib is sufficient |
| encoding/csv | gocsv, jszwec/csvutil | Reflection-based CSV is unnecessary; manual field ordering gives full control |

**Installation:**
No new dependencies required - all functionality uses Go standard library.

---

## Architecture Patterns

### Recommended Package Structure

```
src/
├── main.go                    # Add --depth, --json, --csv, --output flags
├── crawler/
│   ├── crawler.go             # Add depth tracking in coordinator loop
│   ├── worker.go              # Add Depth field to CrawlJob struct
│   └── events.go              # (no changes needed)
├── result/
│   ├── result.go              # Add JSON tags to LinkResult for desired field names
│   ├── output.go              # NEW: JSON and CSV output formatters
│   ├── printer.go             # Existing TUI summary (unchanged)
│   └── errors.go              # (no changes needed)
└── tui/
    ├── model.go               # (no changes needed - TUI always runs)
    └── styles.go              # (no changes needed)
```

### Pattern 1: Depth Tracking in BFS Coordinator

**What:** Track depth for each CrawlJob and check against limit before enqueueing discovered links.

**When to use:** Required for CRWL-05 (depth limiting).

**Example:**

```go
// Source: Based on existing crawler/crawler.go BFS pattern

// In worker.go - extend CrawlJob
type CrawlJob struct {
    URL        string
    SourcePage string
    IsExternal bool
    Depth      int    // NEW: Current depth of this job
}

// In crawler.go - extend Config
type Config struct {
    // ... existing fields
    MaxDepth int  // NEW: 0 = unlimited, >0 = limit (user decision: default unlimited)
}

// In coordinator loop (crawler.go around line 190)
// When enqueueing discovered links:
if !crawlResult.Job.IsExternal && ctx.Err() == nil {
    nextDepth := crawlResult.Job.Depth + 1

    // Check depth limit (only for same-domain)
    if c.cfg.MaxDepth > 0 && nextDepth > c.cfg.MaxDepth {
        continue // Skip - exceeded depth limit
    }

    startHost := hostFromURL(startURL)
    for _, link := range crawlResult.Links {
        // ... existing normalization and robots check ...

        isExternal := !urlutil.IsSameDomain(normalized, startHost)

        // External links don't increment depth for crawling purposes
        // (they're validated but never crawled)
        jobDepth := nextDepth
        if isExternal {
            jobDepth = 0 // External links are just validated
        }

        pendingJobs.Add(1)
        jobs <- CrawlJob{
            URL:        normalized,
            SourcePage: crawlResult.Job.URL,
            IsExternal: isExternal,
            Depth:      jobDepth,
        }
    }
}
```

**Key points:**
- Start URL is depth 0 (user decision)
- Depth only applies to same-domain crawling (user decision)
- External links are validated regardless of depth, but never followed
- Default depth of 0 means unlimited (user decision)

### Pattern 2: JSON Output with Struct Tags

**What:** Use JSON struct tags to control field names and handle `omitempty` for optional fields.

**When to use:** Required for OUTP-03.

**Example:**

```go
// Source: Go standard library encoding/json

package result

// LinkResult with JSON tags for machine-readable output
type LinkResult struct {
    URL           string        `json:"url"`
    StatusCode    int           `json:"status_code,omitempty"`
    Error         string        `json:"error,omitempty"`
    ErrorCategory ErrorCategory `json:"error_type,omitempty"` // "timeout", "dns_failure", etc.
    SourcePage    string        `json:"source_page"`
    IsExternal    bool          `json:"is_external"`
}

// For wrapped output with metadata (Claude's discretion)
type JSONOutput struct {
    StartURL string       `json:"start_url"`
    Stats    CrawlStats   `json:"stats"`
    Results  []LinkResult `json:"results"`
}

// Simple flat array output (alternative)
func WriteJSON(w io.Writer, results []LinkResult) error {
    enc := json.NewEncoder(w)
    enc.SetEscapeHTML(false) // Cleaner URLs
    enc.SetIndent("", "  ")  // Pretty print for readability
    return enc.Encode(results)
}

// In main.go after crawl completes:
if *jsonFlag {
    jsonData, err := json.MarshalIndent(brokenLinks, "", "  ")
    if err != nil {
        return fmt.Errorf("encode JSON: %w", err)
    }
    if *outputFile != "" {
        if err := os.WriteFile(*outputFile, jsonData, 0644); err != nil {
            return fmt.Errorf("write output file: %w", err)
        }
    } else {
        fmt.Println(string(jsonData))
    }
}
```

**Recommendation for JSON structure:** Use flat array `[]LinkResult` - simpler for CI tools to parse and process. Wrapping with metadata adds complexity without clear benefit.

### Pattern 3: CSV Output with Controlled Column Order

**What:** Use `encoding/csv.Writer` with explicitly ordered columns for predictable output.

**When to use:** Required for OUTP-04.

**Example:**

```go
// Source: Go standard library encoding/csv

package result

import (
    "encoding/csv"
    "io"
)

// CSV column order (Claude's discretion)
var csvHeaders = []string{"url", "status_code", "error_type", "source_page", "is_external"}

func WriteCSV(w io.Writer, links []LinkResult) error {
    cw := csv.NewWriter(w)
    defer cw.Flush()

    // Write header row
    if err := cw.Write(csvHeaders); err != nil {
        return err
    }

    // Write data rows
    for _, link := range links {
        statusCode := ""
        if link.StatusCode > 0 {
            statusCode = fmt.Sprintf("%d", link.StatusCode)
        }

        // User decision: separate status_code and error_type
        errorType := ""
        if link.Error != "" || link.ErrorCategory != "" {
            if link.ErrorCategory != "" {
                errorType = string(link.ErrorCategory)
            } else {
                errorType = "unknown"
            }
        }

        record := []string{
            link.URL,
            statusCode,                    // Empty if no HTTP status
            errorType,                     // "timeout", "dns_failure", etc.
            link.SourcePage,
            fmt.Sprintf("%t", link.IsExternal),
        }
        if err := cw.Write(record); err != nil {
            return err
        }
    }

    return cw.Error()
}

// In main.go - empty CSV behavior (user decision)
if *csvFlag {
    var output io.Writer = os.Stdout
    if *outputFile != "" {
        f, err := os.Create(*outputFile) // Silently overwrites (user decision)
        if err != nil {
            return fmt.Errorf("create output file: %w", err)
        }
        defer f.Close()
        output = f
    }

    // User decision: header row only if no broken links
    if err := result.WriteCSV(output, brokenLinks); err != nil {
        return fmt.Errorf("write CSV: %w", err)
    }
}
```

**CSV column order recommendation:** `url,status_code,error_type,source_page,is_external` - puts most important fields (URL, status) first.

### Pattern 4: Flag Definition with Both Short and Long Forms

**What:** Define both short and long flag variants sharing the same variable.

**When to use:** Required for user's flag style decision.

**Example:**

```go
// Source: Go standard library flag package

package main

import "flag"

func main() {
    // Existing flags
    concurrency := flag.Int("concurrency", 10, "number of concurrent workers")
    rateLimit := flag.Int("rate-limit", 10, "requests per second")
    retries := flag.Int("retries", 2, "number of retries for transient errors")
    retryDelay := flag.Duration("retry-delay", time.Second, "base delay between retries")
    userAgent := flag.String("user-agent", "zombiecrawl/1.0 (+https://github.com/lukemcguire/zombiecrawl)", "user agent string")

    // NEW: Phase 4 flags
    var depth int
    flag.IntVar(&depth, "d", 0, "maximum crawl depth (0 = unlimited)")
    flag.IntVar(&depth, "depth", 0, "maximum crawl depth (0 = unlimited)")

    var outputJSON bool
    flag.BoolVar(&outputJSON, "j", false, "output results as JSON")
    flag.BoolVar(&outputJSON, "json", false, "output results as JSON")

    var outputCSV bool
    flag.BoolVar(&outputCSV, "c", false, "output results as CSV")
    flag.BoolVar(&outputCSV, "csv", false, "output results as CSV")

    var outputFile string
    flag.StringVar(&outputFile, "o", "", "write JSON/CSV output to file")
    flag.StringVar(&outputFile, "output", "", "write JSON/CSV output to file")

    flag.Parse()

    // Validate: both --json and --csv is ambiguous
    if outputJSON && outputCSV {
        fmt.Fprintln(os.Stderr, "Error: --json and --csv are mutually exclusive")
        os.Exit(1)
    }

    // ... rest of main
}
```

**Short flag letter recommendations:**
- `-d` / `--depth` - intuitive
- `-j` / `--json` - j for JSON
- `-c` / `--csv` - c for CSV
- `-o` / `--output` - standard convention

**Behavior when both --json and --csv specified:** Error and exit - mutually exclusive (prevents ambiguity in output format).

### Pattern 5: File Output After Crawl Completion

**What:** Write to file only after TUI completes, using the final result.

**When to use:** Required for user's "file written at crawl completion" decision.

**Example:**

```go
// Source: Based on existing main.go flow

func main() {
    // ... flag parsing ...

    // Run TUI (always shown - user decision)
    finalModel, err := program.Run()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }

    finalTUIModel := finalModel.(tui.Model)

    // Write file output AFTER TUI completes
    if finalTUIModel.result != nil && (*outputJSON || *outputCSV || *outputFile != "") {
        if err := writeOutputFile(finalTUIModel.result, *outputJSON, *outputCSV, *outputFile); err != nil {
            fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
            os.Exit(1)
        }
    }

    // Exit code based on broken links
    if finalTUIModel.HasBrokenLinks() {
        os.Exit(1)
    }
}

func writeOutputFile(res *result.Result, asJSON, asCSV bool, filename string) error {
    var output io.Writer = os.Stdout
    if filename != "" {
        f, err := os.Create(filename) // Truncates if exists (user decision: silent overwrite)
        if err != nil {
            return err
        }
        defer f.Close()
        output = f
    }

    if asJSON {
        return result.WriteJSON(output, res.BrokenLinks)
    }
    if asCSV {
        return result.WriteCSV(output, res.BrokenLinks)
    }

    // Default: if -o specified without --json or --csv, default to JSON
    return result.WriteJSON(output, res.BrokenLinks)
}
```

### Anti-Patterns to Avoid

- **Using fmt.Fprintf for JSON output**: Manual JSON construction is error-prone; use `encoding/json`
- **Streaming output during crawl**: User decision says write at completion; streaming adds complexity
- **Depth on external links**: External links are validated, not crawled; depth doesn't apply to them
- **CSV without header**: Always write header row (even if empty) - user decision
- **Over-complicated JSON structure**: Flat array is simpler than wrapped metadata for CI tools

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JSON serialization | Manual string formatting | encoding/json | Handles escaping, types, nested structs correctly |
| CSV output | Manual comma-separated strings | encoding/csv | Handles quoting, escaping, newlines in fields |
| CLI flags | Manual os.Args parsing | flag package | Handles --long, -short, value parsing, help generation |
| File writing | Custom file handling | os.Create + io.Writer | Standard patterns, error handling |

**Key insight:** Go standard library is perfectly suited for this phase. All requirements can be met with stdlib alone, keeping the dependency footprint minimal.

---

## Common Pitfalls

### Pitfall 1: Depth Off-by-One Errors

**What goes wrong:** Confusing "depth 1" to mean different things.

**Why it happens:** Depth semantics vary across tools.

**How to avoid:** Per user decision: depth 0 = start URL, depth 1 = links from start URL. Implement exactly as specified.

**Warning signs:** `--depth 1` crawls more than expected; tests failing for depth boundary conditions.

### Pitfall 2: JSON Field Name Inconsistency

**What goes wrong:** JSON field names don't match what CI tools expect (e.g., `StatusCode` instead of `status_code`).

**Why it happens:** Go default uses struct field names directly.

**How to avoid:** Add explicit JSON tags to struct fields: `json:"status_code,omitempty"`.

**Warning signs:** Output shows `StatusCode` instead of `status_code`.

### Pitfall 3: CSV Without Header for Empty Results

**What goes wrong:** No output at all when no broken links found.

**Why it happens:** Skipping output generation entirely when results are empty.

**How to avoid:** Per user decision: output header row only if no broken links found. Still write something.

**Warning signs:** Empty file instead of header-only file when crawl finds nothing.

### Pitfall 4: Both --json and --csv Specified

**What goes wrong:** Ambiguous which format to output.

**Why it happens:** User accidentally specifies both flags.

**How to avoid:** Validate at startup: if both flags are true, print error and exit.

**Warning signs:** Output is garbled or tool crashes trying to output both formats.

---

## Code Examples

### Complete JSON Output Function

```go
// Source: Go standard library encoding/json pattern

package result

import (
    "encoding/json"
    "io"
)

// LinkResult with JSON tags matching user's field name decisions
type LinkResult struct {
    URL           string        `json:"url"`
    StatusCode    int           `json:"status_code,omitempty"` // HTTP codes only
    Error         string        `json:"error,omitempty"`
    ErrorCategory ErrorCategory `json:"error_type,omitempty"` // "timeout", "dns_failure", etc.
    SourcePage    string        `json:"source_page"`
    IsExternal    bool          `json:"is_external"`
}

func WriteJSON(w io.Writer, links []LinkResult) error {
    enc := json.NewEncoder(w)
    enc.SetEscapeHTML(false) // Don't escape < > & in URLs
    enc.SetIndent("", "  ")  // Pretty print
    return enc.Encode(links)
}
```

### Complete CSV Output Function

```go
// Source: Go standard library encoding/csv pattern

package result

import (
    "encoding/csv"
    "fmt"
    "io"
)

func WriteCSV(w io.Writer, links []LinkResult) error {
    cw := csv.NewWriter(w)
    defer cw.Flush()

    // Header row (always written - user decision)
    headers := []string{"url", "status_code", "error_type", "source_page", "is_external"}
    if err := cw.Write(headers); err != nil {
        return err
    }

    // Data rows
    for _, link := range links {
        // User decision: separate status_code and error_type
        statusCode := ""
        if link.StatusCode > 0 {
            statusCode = fmt.Sprintf("%d", link.StatusCode)
        }

        errorType := ""
        if link.ErrorCategory != "" {
            errorType = string(link.ErrorCategory)
        } else if link.Error != "" {
            errorType = "unknown"
        }

        record := []string{
            link.URL,
            statusCode,
            errorType,
            link.SourcePage,
            fmt.Sprintf("%t", link.IsExternal),
        }
        if err := cw.Write(record); err != nil {
            return err
        }
    }

    return cw.Error()
}
```

---

## Open Questions

1. **JSON structure: flat array vs wrapped with metadata?**
   - What we know: User left this to Claude's discretion
   - Recommendation: Use flat array `[]LinkResult` - simpler for CI tools to parse, jq-friendly, no nesting to navigate

2. **CSV column order?**
   - What we know: User left this to Claude's discretion
   - Recommendation: `url,status_code,error_type,source_page,is_external` - most actionable info first

3. **Behavior when both --json and --csv specified?**
   - What we know: User left this to Claude's discretion
   - Recommendation: Error and exit - mutually exclusive to prevent output ambiguity

---

## Sources

### Primary (HIGH confidence)
- [Go flag package documentation](https://pkg.go.dev/flag) - CLI flag parsing, short/long form patterns
- [Go encoding/json documentation](https://pkg.go.dev/encoding/json) - JSON marshaling, struct tags, omitempty
- [Go encoding/csv documentation](https://pkg.go.dev/encoding/csv) - CSV writing, quoting, headers

### Secondary (MEDIUM confidence)
- Existing codebase analysis of `src/crawler/crawler.go`, `src/crawler/worker.go`, `src/result/result.go`

### Tertiary (LOW confidence)
- None - all recommendations based on official documentation

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - Go stdlib is mature and well-documented
- Architecture: HIGH - Based on analysis of existing well-structured codebase
- Pitfalls: HIGH - Common Go patterns with known solutions

**Research date:** 2026-02-17
**Valid until:** 90 days - Go stdlib is stable; patterns won't change significantly
