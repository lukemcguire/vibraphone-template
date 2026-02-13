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
