"""Git worktree lifecycle tools — start and finish tasks."""

from __future__ import annotations

import asyncio

from config import config, project_root
from tools.beads_tools import get_task_context
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

    result: dict = {"worktree": worktree_path, "branch": branch}

    # Load task context bundle (error-resilient)
    try:
        context = await get_task_context(task_id)
        result["task"] = context.get("task")
        result["plan"] = context.get("plan")
        result["architecture"] = context.get("architecture")
        result["recent_commits"] = context.get("recent_commits")
    except Exception as exc:
        result["context_error"] = str(exc)

    result["next_steps"] = [
        "1. Read the plan and identify the first deliverable",
        "2. Write a failing test for it",
        "3. Run run_tests to confirm it fails",
        "4. Write the minimal implementation to pass",
        "5. Run run_tests + run_lint to confirm green",
        "6. git_stage → request_code_review → attempt_commit",
    ]

    session.audit_log("start_task", {"task_id": task_id}, "ok", result)
    return result


async def merge_to_main(task_id: str) -> dict:
    """Merge a completed task branch into main, clean up worktree and branch.

    Performs a local --no-ff merge. Does not push to any remote.
    On conflict or dirty worktree, returns a structured error without
    attempting auto-resolution.
    """
    branch = f"{config.worktree.prefix}{task_id}"
    base = config.worktree.base_branch
    root = str(project_root())
    worktree_path = str((project_root() / "worktrees" / task_id).resolve())

    async def _git(*args: str, cwd: str = root) -> tuple[int, str]:
        proc = await asyncio.create_subprocess_exec(
            "git", *args,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.STDOUT,
            cwd=cwd,
        )
        stdout, _ = await proc.communicate()
        return proc.returncode or 0, stdout.decode().strip()

    # 1. Verify worktree has no uncommitted changes
    rc, output = await _git("status", "--porcelain", cwd=worktree_path)
    if rc != 0:
        return {"status": "error", "reason": f"Cannot read worktree status: {output}"}
    if output:
        return {"status": "error", "reason": "Worktree has uncommitted changes", "files": output}

    # 2. Verify main worktree HEAD is on base branch
    rc, current_branch = await _git("rev-parse", "--abbrev-ref", "HEAD")
    if rc != 0 or current_branch != base:
        return {
            "status": "error",
            "reason": f"Main worktree is on '{current_branch}', expected '{base}'",
        }

    # 3. Merge with --no-ff
    rc, output = await _git("merge", "--no-ff", branch, "-m", f"Merge {branch} into {base}")
    if rc != 0:
        # Abort the failed merge to leave repo clean
        await _git("merge", "--abort")
        return {"status": "error", "reason": f"Merge conflict: {output}"}

    # 4. Remove worktree
    await _git("worktree", "remove", worktree_path, "--force")

    # 5. Delete feature branch
    # git branch -d fails when the branch isn't pushed to the remote tracking
    # branch, even if it's merged locally. Since merge_to_main is local-only,
    # verify the branch is an ancestor of HEAD ourselves, then delete safely.
    rc_check, _ = await _git("merge-base", "--is-ancestor", branch, "HEAD")
    if rc_check == 0:
        rc, branch_output = await _git("branch", "-D", branch)
    else:
        rc, branch_output = 1, f"Branch '{branch}' is not an ancestor of HEAD after merge"

    # 6. Clear session
    session.clear_task()

    warnings = [
        f"Worktree at worktrees/{task_id} has been removed. "
        "If your shell CWD was inside that worktree, it is now invalid. "
        f"Run: cd {root}"
    ]
    if rc != 0:
        warnings.append(
            f"Branch '{branch}' was NOT deleted: {branch_output}. "
            "This may indicate the merge did not fully integrate the branch. "
            "Investigate before manually removing."
        )

    result = {
        "status": "merged",
        "branch": branch,
        "merged_into": base,
        "branch_deleted": rc == 0,
        "warning": " | ".join(warnings),
    }
    session.audit_log("merge_to_main", {"task_id": task_id}, "ok", result)
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
