"""Structured code review tools — LLM-powered review with JSON output.

Tools: request_code_review
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import os
import re
from pathlib import Path, PurePosixPath

from openai import AsyncOpenAI

from config import config, project_root
from utils import br_client, session

# ---------------------------------------------------------------------------
# Dangerous-file detection for staging
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
    return bool(_DANGEROUS_PATH_PARTS & set(p.parts))


def _working_dir() -> str | None:
    """Resolve the working directory for git commands.

    If a worktree is active in the session, return its absolute path.
    Otherwise return None (inherit the process's cwd).
    """
    state = session.load_session()
    if state and state.worktree:
        return str((project_root() / state.worktree).resolve())
    return None


async def _git_diff_staged() -> str:
    """Return the text of the current staged diff."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "diff",
        "--staged",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=_working_dir(),
    )
    stdout, _ = await proc.communicate()
    return stdout.decode()


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


async def _get_changed_files() -> list[str]:
    """Return list of staged file paths."""
    proc = await asyncio.create_subprocess_exec(
        "git",
        "diff",
        "--staged",
        "--name-only",
        stdout=asyncio.subprocess.PIPE,
        stderr=asyncio.subprocess.PIPE,
        cwd=_working_dir(),
    )
    stdout, _ = await proc.communicate()
    return [f for f in stdout.decode().strip().splitlines() if f]


def _read_file_contents(paths: list[str]) -> str:
    """Read and format the contents of the given files for review context."""
    wdir = _working_dir()
    base = Path(wdir) if wdir else Path.cwd()
    sections = []
    for path in paths:
        p = base / path
        if p.exists() and p.is_file():
            try:
                content = p.read_text()
                sections.append(f"## {path}\n```\n{content}\n```")
            except (OSError, UnicodeDecodeError):
                sections.append(f"## {path}\n(binary or unreadable)")
    return "\n\n".join(sections)


def _parse_porcelain(lines: list[str]) -> list[str]:
    """Extract file paths from git status --porcelain output lines."""
    paths_out: list[str] = []
    for line in lines:
        if not line.strip():
            continue
        m = re.match(r"^..\s+(.+)$", line)
        if m:
            path = m.group(1).strip('"')
            # For renames (R/C), take the destination path after " -> "
            if " -> " in path:
                path = path.split(" -> ", 1)[1]
            paths_out.append(path)
    return paths_out


async def _stage_files(paths: list[str] | None, *, stage_all: bool) -> dict:
    """Stage files with safety checks for sensitive files.

    Args:
        paths: Specific file paths to stage. Mutually exclusive with stage_all.
        stage_all: Stage all changed files (equivalent to git add -A, minus
             dangerous files). Mutually exclusive with paths.

    Returns:
        dict with status, staged list, and warnings list.
    """
    if paths and stage_all:
        return {
            "status": "error",
            "reason": "Provide either 'paths' or 'stage_all=True', not both.",
        }
    if not paths and not stage_all:
        return {
            "status": "error",
            "reason": "Provide either 'paths' or 'stage_all=True'.",
        }

    cwd = _working_dir()
    warnings: list[str] = []

    if stage_all:
        # Discover changed files via git status
        proc = await asyncio.create_subprocess_exec(
            "git",
            "status",
            "--porcelain",
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=cwd,
        )
        stdout, _ = await proc.communicate()
        lines = stdout.decode().strip().splitlines()
        candidates = _parse_porcelain(lines)
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
        "git",
        "add",
        "--",
        *safe,
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

    return {"status": "staged", "staged": safe, "warnings": warnings}


async def _perform_review(
    diff: str,
    files_content: str,
    constitution: str,
    prompt: str,
    model: str,
    api_key: str,
) -> dict:
    """Core review logic shared by MCP tool and standalone CLI.

    Returns dict with keys: status, issues, raw_response.
    """
    client = AsyncOpenAI(
        api_key=api_key,
        base_url="https://openrouter.ai/api/v1",
    )

    user_message = f"# RULES\n{constitution}\n\n# DIFF\n{diff}\n\n# FILES\n{files_content}"

    response = await client.chat.completions.create(
        model=model,
        max_tokens=4096,
        messages=[
            {"role": "system", "content": prompt},
            {"role": "user", "content": user_message},
        ],
    )

    raw_text = (response.choices[0].message.content or "").strip()

    # Parse JSON response — handle markdown code fences
    json_text = raw_text
    if json_text.startswith("```"):
        lines = json_text.splitlines()
        # Remove opening fence (```json or ```)
        lines = lines[1:]
        # Remove closing fence
        if lines and lines[-1].strip() == "```":
            lines = lines[:-1]
        json_text = "\n".join(lines)

    try:
        issues = json.loads(json_text)
        if not isinstance(issues, list):
            msg = "Response is not a JSON array"
            raise TypeError(msg)  # noqa: TRY301
    except (json.JSONDecodeError, TypeError):
        return {
            "status": "ESCALATED",
            "issues": [],
            "raw_response": raw_text,
            "parse_error": "Failed to parse reviewer response as JSON array",
        }

    # Determine status based on severity threshold
    threshold = config.quality_gate.review_severity_threshold
    dominated_severities = {"error"}
    if threshold == "warning":
        dominated_severities.add("warning")

    has_blocking = any(issue.get("severity") in dominated_severities for issue in issues)
    status = "REJECTED" if has_blocking else "APPROVED"

    return {"status": status, "issues": issues, "raw_response": raw_text}


def _load_review_files() -> tuple[str | None, str | None]:
    """Load constitution and prompt files.

    Returns tuple of (constitution, prompt), either of which may be None if not found.
    """
    root = project_root()
    constitution_path = root / config.review.constitution_file
    prompt_path = root / config.review.prompt_file

    constitution = constitution_path.read_text() if constitution_path.exists() else None
    prompt = prompt_path.read_text() if prompt_path.exists() else None

    return constitution, prompt


def _merge_issues(prev_issues: list[dict], new_issues: list[dict]) -> list[dict]:
    """Merge issues from previous attempts, deduplicating by rule+file+line."""
    seen: set[tuple] = set()
    merged: list[dict] = []
    for issue in [*prev_issues, *new_issues]:
        key = (
            issue.get("rule", ""),
            issue.get("file", ""),
            issue.get("line", ""),
        )
        if key not in seen:
            seen.add(key)
            merged.append(issue)
    return merged


async def request_code_review(
    paths: list[str] | None = None,
    *,
    stage_all: bool = False,
) -> dict:
    """Stage files and review them against the project constitution.

    First stages the specified files (with safety checks for sensitive files),
    then uses an LLM to check the staged diff against CONSTITUTION.md rules.

    Args:
        paths: Specific file paths to stage and review. Mutually exclusive with stage_all.
        stage_all: Stage and review all changed files. Mutually exclusive with paths.

    Returns:
        Structured JSON with status (APPROVED/REJECTED/ESCALATED), issues,
        staged files, and any warnings about blocked sensitive files.
    """
    state = session.load_session() or session.SessionState()
    task_id = state.active_task or "unknown"
    max_attempts = config.quality_gate.max_review_attempts

    # Check circuit breaker (without incrementing yet)
    current_attempts = (state.review_attempts or {}).get(task_id, 0)
    if current_attempts >= max_attempts:
        await br_client.br_update(task_id, status="blocked")
        result = {
            "status": "ESCALATED",
            "issues": [],
            "attempt": current_attempts,
            "reason": f"Max review attempts ({max_attempts}) exceeded. Task blocked.",
            "next_steps": [
                "1. Stop working on this task — it is now blocked",
                "2. Call next_ready() to get a different task",
            ],
        }
        session.audit_log("request_code_review", {}, "escalated", result)
        return result

    # Validate API key (don't burn an attempt for missing config)
    api_key = os.environ.get("REVIEWER_API_KEY")
    if not api_key:
        result = {
            "status": "error",
            "issues": [],
            "reason": "REVIEWER_API_KEY environment variable not set",
        }
        session.audit_log("request_code_review", {}, "error", result)
        return result

    # Stage files first (don't burn an attempt for staging failures)
    stage_result = await _stage_files(paths, stage_all=stage_all)
    if stage_result["status"] != "staged":
        result = {
            "status": "error",
            "issues": [],
            "reason": stage_result.get("reason", "No files to stage"),
            "staged": stage_result.get("staged", []),
            "warnings": stage_result.get("warnings", []),
        }
        session.audit_log("request_code_review", {}, "error", result)
        return result

    staged_files = stage_result["staged"]
    warnings = stage_result.get("warnings", [])

    # Get staged diff
    diff = await _git_diff_staged()
    if not diff.strip():
        result = {
            "status": "error",
            "issues": [],
            "reason": "No staged changes to review",
            "staged": staged_files,
            "warnings": warnings,
        }
        session.audit_log("request_code_review", {}, "error", result)
        return result

    # Get changed file contents
    changed_files = await _get_changed_files()
    files_content = _read_file_contents(changed_files)

    # Load constitution and reviewer prompt
    constitution, prompt = _load_review_files()
    if constitution is None:
        result = {
            "status": "error",
            "issues": [],
            "reason": f"Constitution file not found: {config.review.constitution_file}",
            "staged": staged_files,
            "warnings": warnings,
        }
        session.audit_log("request_code_review", {}, "error", result)
        return result

    if prompt is None:
        result = {
            "status": "error",
            "issues": [],
            "reason": f"Reviewer prompt not found: {config.review.prompt_file}",
            "staged": staged_files,
            "warnings": warnings,
        }
        session.audit_log("request_code_review", {}, "error", result)
        return result

    # All preconditions met — now increment the attempt counter
    attempt = session.increment_review_attempts(task_id)

    # Perform the review
    review_result = await _perform_review(
        diff=diff,
        files_content=files_content,
        constitution=constitution,
        prompt=prompt,
        model=config.review.model,
        api_key=api_key,
    )

    status = review_result["status"]
    issues = review_result["issues"]

    # Aggregate issues from previous attempts (dedup by rule+file+line)
    if attempt >= 2:
        prev_state = session.load_session() or session.SessionState()
        prev_issues = prev_state.last_review_issues or []
        issues = _merge_issues(prev_issues, issues)

    # Update session state
    state = session.load_session() or session.SessionState()
    state.last_review_status = status
    state.last_review_diff_hash = await _git_diff_staged_hash()
    state.last_review_issues = issues
    session.save_session(state)

    result = {
        "status": status,
        "issues": issues,
        "attempt": attempt,
        "staged": staged_files,
        "warnings": warnings,
    }
    if "parse_error" in review_result:
        result["parse_error"] = review_result["parse_error"]

    # Add next_steps based on review status
    if status == "APPROVED":
        result["next_steps"] = [
            "1. attempt_commit(message='your commit message') to commit",
        ]
    elif status == "REJECTED":
        result["next_steps"] = [
            "1. Fix the issues listed above",
            "2. request_code_review(stage_all=True) to re-submit",
        ]
    elif status == "ESCALATED":
        result["next_steps"] = [
            "1. Stop working on this task — it is now escalated",
            "2. Call next_ready() to get a different task",
        ]

    session.audit_log("request_code_review", {}, status.lower(), result)
    return result
