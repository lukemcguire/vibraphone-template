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
        Container(cli, "zombiecrawl", "Go binary", "CLI entry point, flag parsing, signal handling")
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
        Component(main, "main", "Go", "CLI entry point")
        Component(crawler, "crawler", "Go", "Concurrent crawl engine with worker pool")
        Component(extract, "crawler/extract", "Go", "HTML tokenizer-based link extraction")
        Component(urlutil, "urlutil", "Go", "URL filtering, normalization, domain checks")
        Component(result, "result", "Go", "Link result types and output formatting")
    }
    Rel(main, crawler, "Starts crawl")
    Rel(main, result, "Prints results to stdout")
    Rel(crawler, extract, "Extracts links from pages")
    Rel(crawler, urlutil, "Filters and classifies URLs")
    Rel(extract, urlutil, "Normalizes discovered URLs")
    Rel(crawler, result, "Produces link results")
```

---

## Data Model

Core types and their relationships (no database — all in-memory).

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
    CrawlResult --> CrawlJob
    CrawlResult --> LinkResult
    Result --> LinkResult
    Result --> CrawlStats
```

---

## Key Sequence Diagrams

### Crawl Flow

```mermaid
sequenceDiagram
    participant U as User
    participant M as main
    participant C as Crawler
    participant W as Worker Pool
    participant E as ExtractLinks
    participant T as Target Website

    U->>M: zombiecrawl [flags] <url>
    M->>C: New(cfg)
    M->>C: Run(ctx)
    C->>W: Launch N workers
    C->>W: Seed start URL as CrawlJob

    loop BFS until queue empty
        W->>T: GET (internal) / HEAD (external)
        T-->>W: HTTP response
        alt Internal page
            W->>E: ExtractLinks(body, baseURL)
            E-->>W: []discovered links
            W-->>C: CrawlResult{Links, Result}
            C->>C: Enqueue new links (dedup via visited map)
        else External link
            W-->>C: CrawlResult{Result}
        end
    end

    C-->>M: *Result{BrokenLinks, Stats}
    M->>M: PrintResults(stdout, result)
    M-->>U: Broken link report + exit code
```
