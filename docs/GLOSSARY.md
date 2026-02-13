# GLOSSARY.md — Domain Terminology

Flat, authoritative list of domain terms. Prevents the LLM from inventing its
own names for concepts. When adding new terms, maintain alphabetical order.

---

| Term                | Definition                                                                                                                                            |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| **ADR**             | Architecture Decision Record. A short document in `docs/DECISIONS.md` capturing a significant technical decision and its rationale.                   |
| **Beads**           | Task state manager (`br` CLI). Stores tasks in SQLite with a git-friendly JSONL export. Tracks status, dependencies, and blocking relationships.      |
| **Bridge**          | The `import_gsd_plan` tool that converts GSD planning output (PLAN.md XML blocks) into Beads tasks with dependency relationships.                     |
| **Circuit breaker** | A safety mechanism that stops retrying after a configured number of failures (`max_test_attempts`, `max_review_attempts`) and returns `ESCALATED`.    |
| **Constitution**    | `docs/CONSTITUTION.md`. The machine-readable coding rules the reviewer checks against. Each rule has an ID referenced in review output.               |
| **Escalation**      | When a circuit breaker fires, the task is marked `blocked` in Beads and the agent moves to a different task. Requires human intervention to unblock.  |
| **GSD**             | "Get Shit Done" — the planning layer. Handles project interviews, research, requirements, roadmaps, and phase planning via `/gsd:*` slash commands.   |
| **MCP**             | Model Context Protocol. The interface through which the agent calls Vibraphone tools. All tool inputs and outputs are structured JSON.                |
| **Quality gate**    | The sequence of checks before a commit is allowed: tests pass, lint passes, code review approved. Enforced by `attempt_commit`.                       |
| **Session**         | Runtime state stored in `.vibraphone/session.json`. Tracks the active task, attempt counters, and last action for crash recovery.                     |
| **Stitch**          | Google Stitch. An optional MCP sidecar for UI generation. When enabled, the agent uses Stitch tools to generate screens before entering the TDD loop. |
| **TDD loop**        | Test-Driven Development cycle: write failing test, write code, run tests, run lint, request review, attempt commit.                                   |
| **Vibraphone**      | The execution framework. Provides MCP tools for task management, worktrees, testing, linting, code review, and commits.                               |
| **Worktree**        | A git worktree created per task in `./worktrees/<task-id>`. Isolates work on each task to its own directory and branch.                               |
