# AGENTS.md — Agent Behavioral Contract

## Philosophy

You are a builder, not an architect. You do not freestyle. You use tools. Never
run raw `git commit` — use `attempt_commit`. Never skip code review — use
`request_code_review`. Never guess at task priority — use `next_ready`.

Planning happens through GSD slash commands. Execution happens through
Vibraphone MCP tools. The tools enforce the rules — you follow the tools.

---

## Available Tools

### Planning Bridge Tools

| Tool              | Inputs              | Returns                                                                          | Notes                                     |
| ----------------- | ------------------- | -------------------------------------------------------------------------------- | ----------------------------------------- |
| `import_gsd_plan` | `phase_number: int` | `{"tasks_created": [...], "dependencies": [...], "diagram_update_needed": bool}` | Parses GSD PLAN.md XML, creates br issues |

### Task Management Tools (thin br wrappers)

| Tool            | Inputs         | Returns                                                 | Notes                                             |
| --------------- | -------------- | ------------------------------------------------------- | ------------------------------------------------- |
| `list_tasks`    | `filter?: str` | JSON from `br list --json`                              | Passthrough with optional status filter           |
| `next_ready`    | —              | JSON from `br ready --json`                             | If `use_bv: true`, uses `bv --robot-next` instead |
| `complete_task` | `task_id: str` | `{"completed": id, "unblocked": [...]}`                 | Runs `br close` + `br sync`                       |
| `abandon_task`  | `task_id: str` | `{"abandoned": id, "status": "ready"}`                  | Resets task, removes worktree                     |
| `health_check`  | —              | `{"br_doctor": ..., "cycles": [...], "orphans": [...]}` | Runs `br doctor` + cycle detection                |

### Worktree / Git Tools

| Tool          | Inputs         | Returns                               | Notes                                                                       |
| ------------- | -------------- | ------------------------------------- | --------------------------------------------------------------------------- |
| `start_task`  | `task_id: str` | `{"worktree": path, "branch": name}`  | `br update --status in_progress` + `just start-task` + updates session.json |
| `finish_task` | `task_id: str` | `{"pushed": branch, "cleaned": bool}` | `just finish-task` + optional worktree cleanup                              |

### Stack Configuration Tools

| Tool              | Inputs                                             | Returns                                                        | Notes                                                       |
| ----------------- | -------------------------------------------------- | -------------------------------------------------------------- | ----------------------------------------------------------- |
| `configure_stack` | `components: dict`, `preview: bool = True`         | `{"status": "preview"\|"configured", ...}`                     | Generates per-component Justfile recipes + vibraphone.yaml   |

### Quality Gate Tools

| Tool                  | Inputs                           | Returns                                                                          | Circuit Breaker                  |
| --------------------- | -------------------------------- | -------------------------------------------------------------------------------- | -------------------------------- |
| `run_tests`           | `component?: str`, `scope?: str` | `{"status": "pass"\|"fail"\|"ESCALATED", "output": ..., "attempt": n}`           | `max_test_attempts`              |
| `run_lint`            | `component?: str`                | `{"status": "pass"\|"fail", "issues": [...]}`                                    | —                                |
| `run_format`          | `component?: str`                | `{"status": "formatted"\|"error", "output": ...}`                                | —                                |
| `request_code_review` | — (reviews staged changes)       | `{"status": "APPROVED"\|"REJECTED"\|"ESCALATED", "issues": [...], "attempt": n}` | `max_review_attempts`            |
| `attempt_commit`      | `message: str`                   | `{"status": "committed"\|"rejected", "reason?": ...}`                            | Requires prior `APPROVED` review |

---

## Workflow State Machine

```mermaid
stateDiagram-v2
    [*] --> PLAN_IMPORTED: import_gsd_plan
    PLAN_IMPORTED --> TASK_CLAIMED: start_task
    TASK_CLAIMED --> TDD_LOOP: write test
    TDD_LOOP --> TDD_LOOP: run_tests (fail)
    TDD_LOOP --> REVIEW: run_tests (pass) + run_lint (pass)
    REVIEW --> TDD_LOOP: request_code_review (REJECTED)
    REVIEW --> COMMITTED: attempt_commit (APPROVED)
    COMMITTED --> TASK_COMPLETE: complete_task
    TASK_COMPLETE --> TASK_CLAIMED: next_ready + start_task
    TDD_LOOP --> ESCALATED: circuit breaker (max attempts)
    REVIEW --> ESCALATED: circuit breaker (max attempts)
    ESCALATED --> TASK_CLAIMED: next_ready (skip to different task)
```

---

## Prohibited Actions

1. Never commit directly with `git commit`. Always use `attempt_commit`.
2. Never skip code review before committing.
3. Never modify files outside your active worktree.
4. Never work on a task that isn't `in_progress` in Beads.

---

## Mermaid Diagram Maintenance

- After project planning is complete, generate initial Mermaid diagrams in
  `ARCHITECTURE.md` (system context, data model, key flows).
- When a task changes the architecture (new service, new data model, new API
  endpoint, new flow), update the corresponding Mermaid diagram in
  `ARCHITECTURE.md` before calling `attempt_commit`.
- The reviewer may flag missing diagram updates as a `warning`.

---

## Stitch Integration (when enabled)

- For UI tasks, use Stitch MCP tools to generate initial screen designs before
  writing component code.
- Call `extract_design_context` to capture fonts, colors, and layout tokens.
  Ensure they're consistent with the project's design system.
- Refactor Stitch output to match CONSTITUTION.md conventions before entering
  the TDD loop.

---

## Escalation Rules

- If any tool returns `ESCALATED`, stop working on the current task immediately.
- Set the task to `blocked` in Beads with a comment explaining the failure.
- Call `next_ready` to move to a different task.
- Never attempt to fix an escalated issue without human approval.

---

## Context Loading on Startup

1. Read `AGENTS.md` (this file).
2. Read `.vibraphone/session.json` — if a task is active, resume it.
3. Read `docs/ARCHITECTURE.md` for system context.
4. Read `docs/CONSTITUTION.md` for coding rules.
5. Read the relevant `.planning/phases/` spec for the current phase.
6. If the Justfile has quality stubs (check/test/lint exit with
   "configure_stack" message), run `configure_stack` with the project's
   component definitions before entering the execution loop.

---

## Error Recovery

- **Test failures:** Fix and retry. After `max_test_attempts`, the tool
  escalates automatically.
- **Review rejections:** Fix issues listed in JSON response. After
  `max_review_attempts`, the tool escalates automatically.
- **Merge conflicts:** Call `abandon_task`, then `start_task` again (fresh
  worktree from latest main).
- **Context reset:** Read `session.json` on startup to resume where you left
  off.

<!-- bv-agent-instructions-v1 -->

---

## Beads Workflow Integration

This project uses [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) for issue tracking. Issues are stored in `.beads/` and tracked in git.

### Essential Commands

```bash
# View issues (launches TUI - avoid in automated sessions)
bv

# CLI commands for agents (use these instead)
bd ready              # Show issues ready to work (no blockers)
bd list --status=open # All open issues
bd show <id>          # Full issue details with dependencies
bd create --title="..." --type=task --priority=2
bd update <id> --status=in_progress
bd close <id> --reason="Completed"
bd close <id1> <id2>  # Close multiple issues at once
bd sync               # Commit and push changes
```

### Workflow Pattern

1. **Start**: Run `bd ready` to find actionable work
2. **Claim**: Use `bd update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `bd close <id>`
5. **Sync**: Always run `bd sync` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `bd ready` shows only unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers, not words)
- **Types**: task, bug, feature, epic, question, docs
- **Blocking**: `bd dep add <issue> <depends-on>` to add dependencies

### Session Protocol

**Before ending any session, run this checklist:**

```bash
git status              # Check what changed
git add <files>         # Stage code changes
bd sync                 # Commit beads changes
git commit -m "..."     # Commit code
bd sync                 # Commit any new beads changes
git push                # Push to remote
```

### Best Practices

- Check `bd ready` at session start to find available work
- Update status as you work (in_progress → closed)
- Create new issues with `bd create` when you discover tasks
- Use descriptive titles and set appropriate priority/type
- Always `bd sync` before ending session

<!-- end-bv-agent-instructions -->
