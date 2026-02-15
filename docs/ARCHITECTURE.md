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
    title System Context Diagram
    %% Replace with actual system context after project planning
    Person(user, "User", "End user of the system")
    System(system, "System", "The system being built")
    Rel(user, system, "Uses")
```

---

## Container Diagram (C4 Level 2)

Services, processes, and communication.

```mermaid
C4Container
    title Container Diagram
    %% Replace with actual containers after project planning
    Person(user, "User", "End user")
    System_Boundary(sb, "System") {
        Container(app, "Application", "Tech TBD", "Main application")
        ContainerDb(db, "Database", "Tech TBD", "Primary data store")
    }
    Rel(user, app, "Uses")
    Rel(app, db, "Reads/Writes")
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
        Component(result, "result", "Go", "Link result types")
    }
    Rel(main, crawler, "Starts crawl")
    Rel(crawler, extract, "Extracts links from pages")
    Rel(crawler, urlutil, "Filters and classifies URLs")
    Rel(extract, urlutil, "Normalizes discovered URLs")
    Rel(crawler, result, "Produces link results")
```

---

## Data Model

ER diagram of database schema.

```mermaid
erDiagram
    %% Replace with actual data model after project planning
    ENTITY {
        string id PK
        string name
        datetime created_at
    }
```

---

## Key Sequence Diagrams

Critical flows (auth, data pipeline, etc.).

```mermaid
sequenceDiagram
    %% Replace with actual sequence diagrams after project planning
    participant U as User
    participant A as Application
    participant D as Database
    U->>A: Request
    A->>D: Query
    D-->>A: Result
    A-->>U: Response
```
