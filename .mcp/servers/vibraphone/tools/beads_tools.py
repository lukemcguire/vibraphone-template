"""Beads task management tools — thin CLI wrappers around br --json."""

from __future__ import annotations

import re

from config import config, project_root
from utils import br_client, session


async def list_tasks(status_filter: str | None = None) -> dict:
    """List all Beads tasks, optionally filtered by status or label."""
    result = await br_client.br_list(status_filter)
    session.audit_log("list_tasks", {"filter": status_filter}, "ok", result)
    return result


async def next_ready() -> dict:
    """Get the next task whose dependencies are satisfied and is ready to start."""
    result = await br_client.br_ready()
    session.audit_log("next_ready", {}, "ok", result)
    return result


async def complete_task(task_id: str) -> dict:
    """Mark a task as complete and optionally sync."""
    result = await br_client.br_close(task_id)
    if config.beads.auto_sync:
        await br_client.br_sync()
    session.clear_task()
    session.audit_log("complete_task", {"task_id": task_id}, "ok", result)
    return result


async def abandon_task(task_id: str) -> dict:
    """Abandon an in-progress task — resets to open and cleans up."""
    result = await br_client.br_update(task_id, status="open")
    session.clear_task()
    session.audit_log("abandon_task", {"task_id": task_id}, "ok", result)
    return result


async def health_check() -> dict:
    """Run br doctor and analyse the task graph for cycles and orphans.

    Returns structured JSON:
        {"br_doctor": <raw doctor output>, "cycles": [...], "orphans": [...]}
    """
    doctor_result = await br_client.br_doctor()

    # Fetch task list for graph analysis
    tasks_result = await br_client.br_list()
    tasks = tasks_result if isinstance(tasks_result, list) else tasks_result.get("tasks", [])

    cycles = br_client.detect_cycles(tasks)
    orphans = br_client.detect_orphans(tasks)

    result = {
        "br_doctor": doctor_result,
        "cycles": cycles,
        "orphans": orphans,
    }
    session.audit_log("health_check", {}, "ok", result)
    return result


_PLAN_LABEL_RE = re.compile(r"plan:(\S+)")


async def get_task_context(task_id: str) -> dict:
    """Load focused context for a task: task details, originating plan, architecture, and recent commits.

    Returns a structured bundle so the agent gets only relevant context
    instead of loading entire files on every task switch.
    """
    task = await br_client.br_show(task_id)
    root = project_root()

    # Extract plan label (set by bridge_tools during import)
    plan_content: str | None = None
    labels = task.get("labels", "") or ""
    match = _PLAN_LABEL_RE.search(labels)
    if match:
        plan_id = match.group(1)  # e.g. "1-2"
        # Derive phase number from plan_id: "1-2" → phase 1
        phase_number = plan_id.split("-")[0]
        # GSD uses zero-padded directory names like "01-core-crawler-foundation"
        padded = phase_number.zfill(2)
        phases_dir = root / ".planning" / "phases"
        phase_dirs = sorted(phases_dir.glob(f"{padded}-*"))
        if phase_dirs:
            plan_path = phase_dirs[0] / f"{plan_id}-PLAN.md"
        else:
            plan_path = phases_dir / phase_number / f"{plan_id}-PLAN.md"
        if plan_path.exists():
            plan_content = plan_path.read_text()

    # Read ARCHITECTURE.md (compact, context-dense)
    arch_path = root / "docs" / "ARCHITECTURE.md"
    architecture: str | None = None
    if arch_path.exists():
        architecture = arch_path.read_text()

    # Recent commits on task branch
    recent_commits = await br_client.git_log(f"feat/{task_id}")

    result = {
        "task": task,
        "plan": plan_content,
        "architecture": architecture,
        "recent_commits": recent_commits,
    }
    session.audit_log("get_task_context", {"task_id": task_id}, "ok", result)
    return result


async def triage() -> dict:
    """Run bv graph-aware triage and return ranked recommendations.

    Returns ranked actionable items, quick wins, blockers to clear,
    and project health metrics. Use this instead of next_ready when
    you need smarter task selection — especially for parallel agents.

    Requires JSONL to be current (auto-synced on state transitions).
    """
    # Ensure JSONL is fresh before triage
    if config.beads.auto_sync:
        await br_client.br_sync()
    result = await br_client.bv_run("--robot-triage")
    session.audit_log("triage", {}, "ok", result)
    return result


async def plan_parallel() -> dict:
    """Return parallel execution tracks for current ready work.

    Returns execution tracks showing which tasks can run concurrently
    and which are sequential, with unblocks lists. Use this to divide
    work across multiple subagents.

    Requires JSONL to be current (auto-synced on state transitions).
    """
    # Ensure JSONL is fresh before planning
    if config.beads.auto_sync:
        await br_client.br_sync()
    result = await br_client.bv_run("--robot-plan")
    session.audit_log("plan_parallel", {}, "ok", result)
    return result


async def add_task(
    title: str,
    type_: str = "task",
    priority: int | None = None,
    labels: str | None = None,
    description: str | None = None,
    depends_on: list[str] | None = None,
    blocks: list[str] | None = None,
) -> dict:
    """Create a new Beads task with optional dependency wiring.

    Args:
        title: Task title.
        type_: Issue type (task, bug, feature, epic, question, docs).
        priority: Priority 0-4 (0=critical, 4=backlog).
        labels: Comma-separated labels.
        description: Task description.
        depends_on: Task IDs that must complete before this task can start.
        blocks: Task IDs that cannot start until this task completes.
    """
    result = await br_client.br_create(
        title,
        description=description,
        type_=type_,
        priority=priority,
        labels=labels,
    )
    new_id = str(result.get("id", result.get("issue_id", "")))

    deps_created: list[dict] = []

    # depends_on: new task is blocked by these
    for dep_id in depends_on or []:
        await br_client.br_dep_add(new_id, dep_id)
        deps_created.append({"blocked": new_id, "blocker": dep_id})

    # blocks: these tasks are blocked by new task
    for blocked_id in blocks or []:
        await br_client.br_dep_add(blocked_id, new_id)
        deps_created.append({"blocked": blocked_id, "blocker": new_id})

    result["dependencies_added"] = deps_created
    session.audit_log("add_task", {"title": title, "type": type_}, "ok", result)
    return result
