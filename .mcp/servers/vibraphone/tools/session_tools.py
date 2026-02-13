"""Session recovery tool — detects and handles stale agent sessions."""

from __future__ import annotations

from datetime import UTC, datetime
from pathlib import Path

from utils import br_client, session

STALE_SESSION_MINUTES = 30


async def recover_session() -> dict:
    """Check for stale sessions and resume or clean up.

    Returns structured JSON indicating what action was taken:
    - ``{"status": "clean", "action": "none"}`` — no active session
    - ``{"status": "active", "action": "resume", ...}`` — fresh session, resume
    - ``{"status": "stale", "action": "resume", ...}`` — stale but resumable
    - ``{"status": "stale", "action": "cleaned_up", ...}`` — stale and cleaned
    """
    state = session.load_session()

    if state is None or state.active_task is None:
        result: dict = {"status": "clean", "action": "none"}
        session.audit_log("recover_session", {}, "ok", result)
        return result

    task_id = state.active_task
    worktree = state.worktree
    last_action_time = state.last_action_time

    # Determine staleness
    is_stale = True
    if last_action_time:
        try:
            last_time = datetime.fromisoformat(last_action_time)
            elapsed = (datetime.now(UTC) - last_time).total_seconds() / 60
            is_stale = elapsed >= STALE_SESSION_MINUTES
        except (ValueError, TypeError):
            is_stale = True

    # Check worktree existence
    worktree_exists = worktree is not None and Path(worktree).exists()

    if not is_stale:
        # Fresh session — resume
        result = {
            "status": "active",
            "action": "resume",
            "task_id": task_id,
            "worktree": worktree,
            "last_action": state.last_action,
            "attempt_counts": {
                "test_attempts": state.test_attempts,
                "review_attempts": state.review_attempts,
            },
        }
        session.audit_log("recover_session", {"task_id": task_id}, "ok", result)
        return result

    # Stale session — check task status in Beads
    try:
        task_info = await br_client.br_show(task_id)
        task_status = task_info.get("status", "unknown")
    except br_client.BrError:
        task_status = "unknown"

    if task_status != "in_progress":
        # Task already closed/blocked — clean up session
        session.clear_task()
        result = {
            "status": "stale",
            "action": "cleaned_up",
            "task_id": task_id,
            "reason": "task no longer in_progress",
        }
        session.audit_log("recover_session", {"task_id": task_id}, "ok", result)
        return result

    if not worktree_exists:
        # Worktree gone — reset task to open and clean up
        await br_client.br_update(task_id, status="open")
        session.clear_task()
        result = {
            "status": "stale",
            "action": "cleaned_up",
            "task_id": task_id,
            "reason": "worktree missing",
        }
        session.audit_log("recover_session", {"task_id": task_id}, "ok", result)
        return result

    # Stale but worktree exists and task still in_progress — agent decides
    result = {
        "status": "stale",
        "action": "resume",
        "task_id": task_id,
        "worktree": worktree,
        "last_action": state.last_action,
        "attempt_counts": {
            "test_attempts": state.test_attempts,
            "review_attempts": state.review_attempts,
        },
    }
    session.audit_log("recover_session", {"task_id": task_id}, "ok", result)
    return result
