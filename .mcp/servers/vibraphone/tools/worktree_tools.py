"""Git worktree lifecycle tools — start, merge, and cleanup tasks."""

from __future__ import annotations

import asyncio
import re

from config import config, project_root
from tools.beads_tools import get_task_context
from utils import br_client, session


async def _run_just(*args: str, cwd: str | None = None) -> str:
    """Run a just recipe and return stdout."""
    proc = await asyncio.create_subprocess_exec(
        "just",
        *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=cwd,
    )
    stdout, stderr = await proc.communicate()
    if proc.returncode != 0:
        msg = f"just {' '.join(args)} failed (rc={proc.returncode}): {stderr.decode().strip()}"
        raise RuntimeError(msg)
    return stdout.decode().strip()


async def _git(*args: str, cwd: str | None = None) -> tuple[int, str]:
    """Run a git command and return (returncode, output)."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        *args,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
        cwd=cwd,
    )
    stdout, _ = await proc.communicate()
    return proc.returncode or 0, stdout.decode().strip()


def _parse_conflict_files(output: str) -> list[str]:
    """Extract conflicted file paths from git rebase output."""
    # Git outputs conflicts like:
    # CONFLICT (content): Merge conflict in path/to/file.py
    pattern = r"CONFLICT.*?: .*? in (.+)"
    return list(set(re.findall(pattern, output)))


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

    result: dict = {"worktree": worktree_path, "branch": branch}

    # Load task context bundle (error-resilient)
    try:
        context = await get_task_context(task_id)
        result["task"] = context.get("task")
        result["plan"] = context.get("plan")
        result["architecture"] = context.get("architecture")
        result["recent_commits"] = context.get("recent_commits")
    except (OSError, br_client.BrError) as exc:
        result["context_error"] = str(exc)

    result["next_steps"] = [
        "1. Read the plan and identify the first deliverable",
        "2. Write a failing test for it",
        "3. Run run_tests to confirm it fails",
        "4. Write the minimal implementation to pass",
        "5. Run run_tests + run_lint to confirm green",
        "6. request_code_review(stage_all=True) → attempt_commit",
        "7. merge_task → cleanup_task → complete_task",
    ]

    session.audit_log("start_task", {"task_id": task_id}, "ok", result)
    return result


async def merge_task(task_id: str) -> dict:
    """Merge a completed task branch into main with rebase.

    Rebases the feature branch onto main, then merges with --no-ff.
    On conflict, aborts rebase and returns structured error.
    On success, returns next_steps instructing agent to leave worktree.
    """
    branch = f"{config.worktree.prefix}{task_id}"
    base = config.worktree.base_branch
    root = str(project_root())
    worktree_path = str((project_root() / "worktrees" / task_id).resolve())

    # 1. Verify worktree has no uncommitted changes
    rc, output = await _git("status", "--porcelain", cwd=worktree_path)
    if rc != 0:
        return {"status": "error", "reason": f"Cannot read worktree status: {output}"}
    if output:
        return {"status": "error", "reason": "Worktree has uncommitted changes", "files": output}

    # 2. Verify main worktree HEAD is on base branch
    rc, current_branch = await _git("rev-parse", "--abbrev-ref", "HEAD", cwd=root)
    if rc != 0 or current_branch != base:
        return {
            "status": "error",
            "reason": f"Main worktree is on '{current_branch}', expected '{base}'",
        }

    # 3. Rebase feature branch onto main
    rc, rebase_output = await _git("rebase", base, branch, cwd=root)
    if rc != 0:
        # Abort the failed rebase to leave repo clean
        await _git("rebase", "--abort", cwd=root)
        conflicted_files = _parse_conflict_files(rebase_output)
        result = {
            "status": "conflict",
            "reason": "Rebase conflict - manual resolution required",
            "conflicted_files": conflicted_files,
            "raw_output": rebase_output,
            "next_steps": [
                f"1. Resolve conflicts in: {', '.join(conflicted_files)}",
                f"2. cd {worktree_path}",
                "3. git add <resolved-files>",
                "4. git rebase --continue",
                f"5. cd {root}",
                f"6. merge_task('{task_id}') to retry",
            ],
        }
        session.audit_log("merge_task", {"task_id": task_id}, "conflict", result)
        return result

    # 4. Merge with --no-ff
    rc, merge_output = await _git("merge", "--no-ff", branch, "-m", f"Merge {branch} into {base}", cwd=root)
    if rc != 0:
        # Abort the failed merge to leave repo clean
        await _git("merge", "--abort", cwd=root)
        return {"status": "error", "reason": f"Merge failed: {merge_output}"}

    # 5. Update session phase
    state = session.load_session()
    if state:
        state.phase = "merged"
        session.save_session(state)

    result = {
        "status": "merged",
        "branch": branch,
        "merged_into": base,
        "next_steps": [
            f"1. cd {root}  # Leave the worktree before cleanup",
            f"2. cleanup_task('{task_id}')  # Remove worktree and branch",
            f"3. complete_task('{task_id}')  # Mark done in Beads",
            "4. next_ready()  # Get next task",
        ],
    }
    session.audit_log("merge_task", {"task_id": task_id}, "ok", result)
    return result


async def cleanup_task(task_id: str) -> dict:
    """Remove worktree and delete feature branch.

    Should only be called after merge_task succeeds and agent has
    changed CWD to project root.
    """
    root = str(project_root())

    await _run_just("cleanup-task", task_id, cwd=root)

    # Clear session
    session.clear_task()

    result = {
        "status": "cleaned",
        "worktree_removed": f"./worktrees/{task_id}",
        "branch_deleted": f"feat/{task_id}",
        "next_steps": [
            f"1. complete_task('{task_id}')  # Mark done in Beads",
            "2. next_ready()  # Get next task",
        ],
    }
    session.audit_log("cleanup_task", {"task_id": task_id}, "ok", result)
    return result
