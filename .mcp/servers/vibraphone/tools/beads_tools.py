"""Beads task management tools — thin CLI wrappers around br --json."""

from __future__ import annotations

from config import config
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
