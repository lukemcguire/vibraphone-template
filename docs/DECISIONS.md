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
