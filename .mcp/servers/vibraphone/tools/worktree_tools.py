"""Git worktree lifecycle tools — start and finish tasks."""

from __future__ import annotations

import asyncio

from config import config
from utils import br_client, session


async def _run_just(*args: str) -> str:
    """Run a just recipe and return stdout."""
    proc = await asyncio.create_subprocess_exec(
        "just",
        *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
    )
    stdout, stderr = await proc.communicate()
    if proc.returncode != 0:
        msg = f"just {' '.join(args)} failed (rc={proc.returncode}): {stderr.decode().strip()}"
        raise RuntimeError(msg)
    return stdout.decode().strip()


async def start_task(task_id: str) -> dict:
    """Create a worktree and branch for a task, mark it in-progress."""
    await br_client.br_update(task_id, status="in_progress")
    # Sync so other agents (and bv) see this task is claimed
    if config.beads.auto_sync:
        await br_client.br_sync()
    await _run_just("start-task", task_id)

    worktree_path = f"./worktrees/{task_id}"
    branch = f"{config.worktree.prefix}{task_id}"

    state = session.load_session() or session.SessionState()
    state.active_task = task_id
    state.worktree = worktree_path
    state.phase = "working"
    session.save_session(state)

    result = {"worktree": worktree_path, "branch": branch}
    session.audit_log("start_task", {"task_id": task_id}, "ok", result)
    return result


async def finish_task(task_id: str) -> dict:
    """Push the task branch and optionally clean up the worktree."""
    await _run_just("finish-task", task_id)

    branch = f"{config.worktree.prefix}{task_id}"
    cleaned = False

    if config.worktree.auto_cleanup:
        await _run_just("cleanup-task", task_id)
        cleaned = True

    result = {"pushed": branch, "cleaned": cleaned}
    session.audit_log("finish_task", {"task_id": task_id}, "ok", result)
    return result
