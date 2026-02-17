# ARCHITECTURE.md — Visual Source of Truth

Mermaid diagrams that give the agent a compressed understanding of the system
without reading every file. Diagrams are generated incrementally as the project
grows.

---

## When Diagrams Are Created and Updated

| Trigger                     | Action                                                                                |
| --------------------------- | ------------------------------------------------------------------------------------- |
| `gsd:new-project` completes | Agent generates initial system context + data model diagrams                          |
| `import_gsd_plan` runs      | Bridge tool checks if phase introduces new components; flags diagram update needed    |
| `complete_task` runs        | Agent updates diagrams if task changed architecture (enforced by AGENTS.md directive) |
| `request_code_review` runs  | Reviewer checks for missing diagram updates per CONSTITUTION.md rules                 |

---

## System Context (C4 Level 1)

External systems and boundaries.

```mermaid
C4Context
    title System Context — ZombieCrawl
    Person(user, "User", "Runs CLI to find broken links on a website")
    System(zc, "ZombieCrawl", "CLI tool that crawls a website and reports broken links")
    System_Ext(target, "Target Website", "The website being crawled for broken links")
    Rel(user, zc, "Runs with target URL")
    Rel(zc, target, "HTTP GET/HEAD requests")
```

---

## Container Diagram (C4 Level 2)

Services, processes, and communication.

```mermaid
C4Container
    title Container Diagram — ZombieCrawl
    Person(user, "User", "Runs CLI from terminal")
    System_Boundary(sb, "ZombieCrawl") {
        Container(cli, "zombiecrawl", "Go binary", "CLI entry point, Bubble Tea TUI with live progress")
    }
    System_Ext(target, "Target Website", "Serves HTML pages over HTTP/HTTPS")
    Rel(user, cli, "Provides URL and flags")
    Rel(cli, target, "HTTP GET/HEAD requests")
    Rel(cli, user, "Prints broken link report to stdout")
```

---

## Component Diagram (C4 Level 3)

Internal structure per container.

```mermaid
C4Component
    title Component Diagram — ZombieCrawl
    Container_Boundary(app, "ZombieCrawl CLI") {
        Component(main, "main", "Go", "CLI entry point, wires TUI to crawler")
        Component(tuipkg, "tui", "Go", "Bubble Tea model with live progress and Lip Gloss summary")
        Component(crawler, "crawler", "Go", "Concurrent crawl engine with worker pool")
        Component(robots, "crawler/robots", "Go", "RobotsChecker with 1-hour cache for robots.txt compliance")
        Component(visited, "crawler/visited", "Go", "Disk-backed bloom filter for URL deduplication")
        Component(memory, "crawler/memory", "Go", "Memory pressure monitoring with SetMemoryLimit")
        Component(events, "crawler/events", "Go", "Progress event types for TUI integration")
        Component(extract, "crawler/extract", "Go", "HTML tokenizer-based link extraction")
        Component(urlutil, "urlutil", "Go", "URL filtering, normalization, domain checks")
        Component(result, "result", "Go", "Link result types and output formatting")
    }
    System_Ext(target, "Target Website", "Serves HTML pages and robots.txt over HTTP/HTTPS")
    System_Ext(disk, "OS Temp Directory", "Stores bloom filter mmap files")
    Rel(main, tuipkg, "Creates Model, runs tea.Program")
    Rel(tuipkg, crawler, "Starts crawl via tea.Cmd")
    Rel(tuipkg, events, "Reads CrawlEvent from progress channel")
    Rel(tuipkg, result, "Renders styled summary from Result")
    Rel(crawler, events, "Emits progress events")
    Rel(crawler, robots, "Checks URL allowability before enqueueing")
    Rel(crawler, visited, "Uses VisitedTracker for URL dedup")
    Rel(crawler, memory, "Checks memory pressure during crawl")
    Rel(crawler, extract, "Extracts links from pages")
    Rel(crawler, urlutil, "Filters and classifies URLs")
    Rel(robots, target, "Fetches robots.txt per host")
    Rel(extract, urlutil, "Normalizes discovered URLs")
    Rel(crawler, result, "Produces link results")
    Rel(visited, disk, "Creates temp file for bloom filter")
```

---

## Data Model

Core types and their relationships (no database — all in-memory except bloom filter temp file).

```mermaid
classDiagram
    class Config {
        string StartURL
        int Concurrency
        Duration RequestTimeout
    }
    class CrawlJob {
        string URL
        string SourcePage
        bool IsExternal
    }
    class CrawlResult {
        CrawlJob Job
        []string Links
        *LinkResult Result
        error Err
    }
    class CrawlEvent {
        string URL
        int StatusCode
        string Error
        int Checked
        int Broken
        bool IsExternal
    }
    class LinkResult {
        string URL
        int StatusCode
        string Error
        string SourcePage
        bool IsExternal
    }
    class CrawlStats {
        int TotalChecked
        int BrokenCount
        Duration Duration
    }
    class Result {
        []LinkResult BrokenLinks
        CrawlStats Stats
    }
    class RobotsChecker {
        *http.Client client
        sync.Map cache
        Duration cacheTTL
    }
    class cachedRobots {
        *RobotsData data
        Time fetchedAt
    }
    class VisitedTracker {
        sync.Mutex mu
        *BloomFilter filter
        *os.File file
        MMap mmap
        string tmpPath
        uint64 count
        uint64 syncEvery
    }
    class MemoryWatcher {
        sync.RWMutex mu
        int64 limitBytes
        func callback
        ThrottleLevel lastLevel
    }
    class ThrottleLevel {
        <<enumeration>>
        ThrottleNormal
        ThrottleWarning
        ThrottleCritical
    }
    CrawlResult --> CrawlJob
    CrawlResult --> LinkResult
    Result --> LinkResult
    Result --> CrawlStats
    RobotsChecker --> cachedRobots
    MemoryWatcher --> ThrottleLevel
```

---

## Key Sequence Diagrams

### Crawl Flow

```mermaid
sequenceDiagram
    participant U as User
    participant M as main
    participant TUI as tui.Model
    participant C as Crawler
    participant W as Worker Pool
    participant E as ExtractLinks
    participant T as Target Website

    U->>M: zombiecrawl [flags] <url>
    M->>TUI: NewModel(ctx, cancel, crawler, progressCh)
    M->>TUI: tea.NewProgram(model).Run()
    TUI->>TUI: Init() → Batch(spinner.Tick, startCrawl, waitForProgress)
    TUI->>C: startCrawl → crawler.Run(ctx)
    C->>W: Launch N workers
    C->>W: Seed start URL as CrawlJob

    loop BFS until queue empty
        W->>T: GET (internal) / HEAD (external)
        T-->>W: HTTP response
        alt Internal page
            W->>E: ExtractLinks(body, baseURL)
            E-->>W: []discovered links
            W-->>C: CrawlResult{Links, Result}
            C->>C: Enqueue new links (dedup via VisitedTracker bloom filter)
        else External link
            W-->>C: CrawlResult{Result}
        end
        C->>TUI: CrawlEvent via progressCh
        TUI->>TUI: Update(CrawlProgressMsg) → re-render spinner + counters
    end

    C-->>TUI: CrawlDoneMsg{Result}
    TUI->>TUI: View() → RenderSummary (Lip Gloss styled table)
    TUI-->>M: finalModel via p.Run()
    M-->>U: Styled report + exit code
```

### Robots.txt Check Flow

```mermaid
sequenceDiagram
    participant C as Crawler
    participant R as RobotsChecker
    participant Cache as sync.Map Cache
    participant T as Target Website

    C->>R: Allowed(ctx, url, userAgent)
    R->>R: Parse URL, extract host
    R->>Cache: Load(host)

    alt Cache hit and valid TTL
        Cache-->>R: cachedRobots{data, fetchedAt}
        alt data is nil
            R-->>C: true (allow all)
        else data exists
            R->>R: data.TestAgent(path, userAgent)
            R-->>C: allowed/disallowed
        end
    else Cache miss or expired
        R->>T: GET {scheme}://{host}/robots.txt
        alt 404 or 5xx response
            T-->>R: 404/5xx status
            R->>Cache: Store(host, nil entry)
            R-->>C: true (allow all)
        else Network error
            T--xR: timeout/connection error
            R->>Cache: Store(host, nil entry)
            R-->>C: true, error (fail-open)
        else 2xx response
            T-->>R: 200 OK with robots.txt body
            R->>R: robotstxt.FromStatusAndBytes()
            R->>Cache: Store(host, parsed data)
            R->>R: robots.TestAgent(path, userAgent)
            R-->>C: allowed/disallowed
        end
    end
```
