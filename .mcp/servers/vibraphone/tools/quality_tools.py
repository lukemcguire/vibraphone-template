"""Quality gate tools — test, lint, format, commit with circuit breakers."""

from __future__ import annotations

import asyncio
import hashlib

from pathlib import PurePosixPath

from config import config, project_root
from utils import br_client, session

# ---------------------------------------------------------------------------
# Dangerous-file detection for git_stage
# ---------------------------------------------------------------------------

_DANGEROUS_FILENAMES = {
    ".env",
    ".env.local",
    ".env.production",
    ".env.staging",
    "credentials.json",
    "service-account.json",
    "secrets.json",
    "id_rsa",
    "id_ed25519",
}

_DANGEROUS_EXTENSIONS = {
    ".pem",
    ".key",
    ".p12",
    ".pfx",
    ".jks",
    ".keystore",
}

_DANGEROUS_PATH_PARTS = {
    ".ssh",
    ".gnupg",
}


def _is_dangerous(path: str) -> bool:
    """Check if a file path matches known sensitive file patterns."""
    p = PurePosixPath(path)
    if p.name in _DANGEROUS_FILENAMES:
        return True
    if p.suffix in _DANGEROUS_EXTENSIONS:
        return True
    if _DANGEROUS_PATH_PARTS & set(p.parts):
        return True
    return False


def _working_dir() -> str | None:
    """Resolve the working directory for shell commands.

    If a worktree is active in the session, return its absolute path so that
    Justfile recipes (``cd ./src && ...``) resolve relative to the worktree.
    Otherwise return None (inherit the process's cwd).
    """
    state = session.load_session()
    if state and state.worktree:
        return str((project_root() / state.worktree).resolve())
    return None


async def _run_shell(cmd: str) -> tuple[int, str]:
    """Run a shell command and return (returncode, combined output)."""
    proc = await asyncio.create_subprocess_shell(
        cmd,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.STDOUT,
        cwd=_working_dir(),
    )
    stdout, _ = await proc.communicate()
    return proc.returncode or 0, stdout.decode().strip()


async def _git_diff_staged_hash() -> str:
    """Return a SHA-256 hash of the current staged diff."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "diff",
        "--staged",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=_working_dir(),
    )
    stdout, _ = await proc.communicate()
    return hashlib.sha256(stdout).hexdigest()


async def git_stage(paths: list[str] | None = None, all: bool = False) -> dict:
    """Stage files for commit with safety checks for sensitive files.

    Args:
        paths: Specific file paths to stage. Mutually exclusive with ``all``.
        all: Stage all changed files (equivalent to ``git add -A``, minus
             dangerous files). Mutually exclusive with ``paths``.

    Returns:
        dict with ``status``, ``staged`` list, and ``warnings`` list.
    """
    if bool(paths) == all:
        return {
            "status": "error",
            "reason": "Provide either 'paths' or 'all=True', not both (or neither).",
        }

    cwd = _working_dir()
    warnings: list[str] = []

    if all:
        # Discover changed files via git status
        proc = await asyncio.create_subprocess_exec(
            "git", "status", "--porcelain",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=cwd,
        )
        stdout, _ = await proc.communicate()
        lines = stdout.decode().strip().splitlines()
        # Parse porcelain output: first 3 chars are status, rest is path
        candidates = [line[3:].strip().strip('"') for line in lines if line.strip()]
    else:
        candidates = list(paths)  # type: ignore[arg-type]

    safe: list[str] = []
    for p in candidates:
        if _is_dangerous(p):
            warnings.append(f"Blocked sensitive file: {p}")
        else:
            safe.append(p)

    if not safe:
        return {"status": "nothing_to_stage", "staged": [], "warnings": warnings}

    proc = await asyncio.create_subprocess_exec(
        "git", "add", "--", *safe,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=cwd,
    )
    _, stderr = await proc.communicate()

    if proc.returncode != 0:
        return {
            "status": "error",
            "reason": stderr.decode().strip(),
            "staged": [],
            "warnings": warnings,
        }

    result = {"status": "staged", "staged": safe, "warnings": warnings}
    session.audit_log("git_stage", {"paths": paths, "all": all}, "ok", result)
    return result


async def run_tests(component: str | None = None, scope: str | None = None) -> dict:
    """Run tests with circuit breaker. Escalates after max attempts."""
    state = session.load_session() or session.SessionState()
    task_id = state.active_task or "unknown"

    attempt = session.increment_test_attempts(task_id)
    max_attempts = config.quality_gate.max_test_attempts

    if attempt > max_attempts:
        await br_client.br_update(task_id, status="blocked")
        result = {
            "status": "ESCALATED",
            "output": f"Max test attempts ({max_attempts}) exceeded. Task blocked.",
            "attempt": attempt,
        }
        session.audit_log("run_tests", {"component": component, "scope": scope}, "escalated", result)
        return result

    cmd = f"just test-{component}" if component else "just test"
    if scope:
        cmd = f"{cmd} {scope}"

    rc, output = await _run_shell(cmd)
    status = "pass" if rc == 0 else "fail"

    result = {"status": status, "output": output, "attempt": attempt}
    session.audit_log("run_tests", {"component": component, "scope": scope}, status, result)
    return result


async def run_lint(component: str | None = None) -> dict:
    """Run linter. No circuit breaker."""
    cmd = f"just lint-{component}" if component else "just lint"

    rc, output = await _run_shell(cmd)
    status = "pass" if rc == 0 else "fail"

    issues = []
    if rc != 0:
        issues.extend(line.strip() for line in output.splitlines() if line.strip())

    result = {"status": status, "issues": issues}
    session.audit_log("run_lint", {"component": component}, status, result)
    return result


async def run_format(component: str | None = None) -> dict:
    """Run formatter. No circuit breaker."""
    cmd = f"just format-{component}" if component else "just format"

    rc, output = await _run_shell(cmd)
    status = "formatted" if rc == 0 else "error"

    result = {"status": status, "output": output}
    session.audit_log("run_format", {"component": component}, status, result)
    return result


async def attempt_commit(message: str) -> dict:
    """Commit staged changes after verifying review approval and quality gate."""
    state = session.load_session()

    # Precondition 1: Review must be approved
    if state is None or state.last_review_status != "APPROVED":
        result = {"status": "rejected", "reason": "No approved review in session"}
        session.audit_log("attempt_commit", {"message": message}, "rejected", result)
        return result

    # Precondition 2: Staged diff must match what was reviewed
    current_hash = await _git_diff_staged_hash()
    if current_hash != state.last_review_diff_hash:
        result = {"status": "rejected", "reason": "Staged diff changed since review"}
        session.audit_log("attempt_commit", {"message": message}, "rejected", result)
        return result

    # Precondition 3: Quality gate must pass
    rc, check_output = await _run_shell("just check")
    if rc != 0:
        result = {"status": "rejected", "reason": f"Quality gate failed: {check_output}"}
        session.audit_log("attempt_commit", {"message": message}, "rejected", result)
        return result

    # All checks passed — commit
    proc = await asyncio.create_subprocess_exec(
        "git",
        "commit",
        "-m",
        message,
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=_working_dir(),
    )
    _stdout, stderr = await proc.communicate()

    if proc.returncode != 0:
        result = {"status": "rejected", "reason": f"git commit failed: {stderr.decode().strip()}"}
        session.audit_log("attempt_commit", {"message": message}, "rejected", result)
        return result

    task_id = state.active_task or "unknown"
    result = {
        "status": "committed",
        "next_steps": [
            f"1. If more deliverables remain in the plan, repeat the TDD loop",
            f"2. When all deliverables are done, run merge_to_main(task_id='{task_id}')",
            f"3. Then complete_task(task_id='{task_id}') to close the task",
            f"4. Call next_ready to pick up the next task",
        ],
    }
    session.audit_log("attempt_commit", {"message": message}, "committed", result)
    return result
