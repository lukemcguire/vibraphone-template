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

| Tool               | Inputs                                                                                                                             | Returns                                                                                    | Notes                                             |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ | ------------------------------------------------- |
| `list_tasks`       | `filter?: str`                                                                                                                     | JSON from `br list --json`                                                                 | Passthrough with optional status filter           |
| `next_ready`       | —                                                                                                                                  | JSON from `br ready --json`                                                                | If `use_bv: true`, uses `bv --robot-next` instead |
| `complete_task`    | `task_id: str`                                                                                                                     | `{"completed": id, "unblocked": [...]}`                                                    | Runs `br close` + `br sync`                       |
| `abandon_task`     | `task_id: str`                                                                                                                     | `{"abandoned": id, "status": "ready"}`                                                     | Resets task, removes worktree                     |
| `health_check`     | —                                                                                                                                  | `{"br_doctor": ..., "cycles": [...], "orphans": [...]}`                                    | Runs `br doctor` + cycle detection                |
| `get_task_context` | `task_id: str`                                                                                                                     | `{"task": ..., "plan": str\|null, "architecture": str\|null, "recent_commits": str\|null}` | Loads focused context bundle for a task           |
| `add_task`         | `title: str`, `type_?: str`, `priority?: int`, `labels?: str`, `description?: str`, `depends_on?: list[str]`, `blocks?: list[str]` | `{"id": ..., "dependencies_added": [...]}`                                                 | Creates task with optional dependency wiring      |

### Session Recovery Tools

| Tool              | Inputs | Returns                                                                            | Notes                                                      |
| ----------------- | ------ | ---------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `recover_session` | —      | `{"status": "clean"\|"active"\|"stale", "action": "none"\|"resume"\|"cleaned_up"}` | Detects stale sessions; resumes or cleans up automatically |

### Worktree / Git Tools

| Tool          | Inputs         | Returns                               | Notes                                                                       |
| ------------- | -------------- | ------------------------------------- | --------------------------------------------------------------------------- |
| `start_task`  | `task_id: str` | `{"worktree": path, "branch": name}`  | `br update --status in_progress` + `just start-task` + updates session.json |
| `finish_task` | `task_id: str` | `{"pushed": branch, "cleaned": bool}` | `just finish-task` + optional worktree cleanup                              |

### Stack Configuration Tools

| Tool              | Inputs                                     | Returns                                    | Notes                                                      |
| ----------------- | ------------------------------------------ | ------------------------------------------ | ---------------------------------------------------------- |
| `configure_stack` | `components: dict`, `preview: bool = True` | `{"status": "preview"\|"configured", ...}` | Generates per-component Justfile recipes + vibraphone.yaml |

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
2. Call `recover_session` tool.
   - If `action: "resume"` → continue working on the returned task_id.
   - If `action: "cleaned_up"` → stale state was cleared, proceed to
     `next_ready`.
   - If `action: "none"` → no active task, proceed normally.
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
- **Context reset:** Call `recover_session` on startup. It checks session
  staleness, verifies worktree and task status, and resumes or cleans up.

