"""Structured code review tools — LLM-powered review with JSON output.

Tools: request_code_review
"""

from __future__ import annotations

import asyncio
import hashlib
import json
import logging
import os
import re
from pathlib import Path, PurePosixPath

from openai import AsyncOpenAI

from config import config, project_root
from utils import br_client, session

logger = logging.getLogger(__name__)

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
    logger.debug("_stage_files: cwd=%s", cwd or "(inherit)")
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
        raw_output = stdout.decode().rstrip()  # Only strip trailing whitespace, preserve leading XY
        logger.debug("_stage_files: git status output=%r", raw_output)
        lines = raw_output.splitlines()
        candidates = _parse_porcelain(lines)
    else:
        candidates = list(paths)  # type: ignore[arg-type]

    logger.debug("_stage_files: candidates=%r", candidates)

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
    previous_issues: list[dict] | None = None,
) -> dict:
    """Core review logic shared by MCP tool and standalone CLI.

    Returns dict with keys: status, issues, raw_response.
    """
    client = AsyncOpenAI(
        api_key=api_key,
        base_url="https://openrouter.ai/api/v1",
    )

    user_message = f"# RULES\n{constitution}\n\n# DIFF\n{diff}\n\n# FILES\n{files_content}"

    if previous_issues:
        prev_json = json.dumps(previous_issues, indent=2)
        user_message += (
            f"\n\n# PREVIOUS REVIEW ISSUES\n"
            f"The following issues were raised in a prior review of this code. "
            f"The author has attempted to fix them. For each previous issue, "
            f"check whether it has been addressed in the current DIFF and FILES. "
            f"Only re-report an issue if it still exists in the current code. "
            f"Do NOT echo resolved issues.\n\n{prev_json}"
        )

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


# ---------------------------------------------------------------------------
# Result builders and validators (reduce C901 complexity)
# ---------------------------------------------------------------------------


def _error_result(
    reason: str,
    staged: list[str] | None = None,
    warnings: list[str] | None = None,
    attempt: int | None = None,
) -> dict:
    """Build a standardized error response."""
    result: dict = {
        "status": "error",
        "issues": [],
        "reason": reason,
        "staged": staged or [],
        "warnings": warnings or [],
    }
    if attempt is not None:
        result["attempt"] = attempt
    return result


def _check_circuit_breaker(state: session.SessionState, task_id: str) -> dict | None:
    """Check if circuit breaker has been tripped for review attempts.

    Returns an escalation result if max attempts exceeded, else None.
    """
    max_attempts = config.quality_gate.max_review_attempts
    current_attempts = (state.review_attempts or {}).get(task_id, 0)

    if current_attempts >= max_attempts:
        return {
            "status": "ESCALATED",
            "issues": [],
            "attempt": current_attempts,
            "reason": f"Max review attempts ({max_attempts}) exceeded. Task blocked.",
            "next_steps": [
                "1. Stop working on this task — it is now blocked",
                "2. Call next_ready() to get a different task",
            ],
        }
    return None


def _validate_api_key() -> str | None:
    """Return API key if available, or None if missing."""
    return os.environ.get("REVIEWER_API_KEY")


def _load_review_assets() -> tuple[str, str] | dict:
    """Load constitution and reviewer prompt files.

    Returns (constitution, prompt) on success, or error dict on failure.
    """
    constitution, prompt = _load_review_files()

    if constitution is None:
        return _error_result(
            f"Constitution file not found: {config.review.constitution_file}"
        )
    if prompt is None:
        return _error_result(f"Reviewer prompt not found: {config.review.prompt_file}")

    return constitution, prompt


def _build_unchanged_result(
    state: session.SessionState,
    current_attempts: int,
    staged_files: list[str],
    warnings: list[str],
) -> dict:
    """Build result for unchanged diff (short-circuit return)."""
    result: dict = {
        "status": state.last_review_status or "REJECTED",
        "issues": state.last_review_issues or [],
        "attempt": current_attempts,
        "staged": staged_files,
        "warnings": warnings,
        "unchanged": True,
        "reason": "Diff unchanged since last review — fix the issues before resubmitting.",
    }

    if state.last_review_status == "APPROVED":
        result["next_steps"] = ["1. attempt_commit(message='your commit message') to commit"]
    else:
        result["next_steps"] = [
            "1. Fix the issues listed above",
            "2. request_code_review(stage_all=True) to re-submit",
        ]

    return result


def _build_review_result(
    status: str,
    issues: list[dict],
    attempt: int,
    staged_files: list[str],
    warnings: list[str],
    parse_error: str | None = None,
) -> dict:
    """Build the final review result with next_steps."""
    result: dict = {
        "status": status,
        "issues": issues,
        "attempt": attempt,
        "staged": staged_files,
        "warnings": warnings,
    }

    if parse_error:
        result["parse_error"] = parse_error

    if status == "APPROVED":
        result["next_steps"] = ["1. attempt_commit(message='your commit message') to commit"]
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

    return result


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
    current_attempts = (state.review_attempts or {}).get(task_id, 0)

    # 1. Check circuit breaker
    if escalation := _check_circuit_breaker(state, task_id):
        await br_client.br_update(task_id, status="blocked")
        session.audit_log("request_code_review", {}, "escalated", escalation)
        return escalation

    # 2. Validate API key
    api_key = _validate_api_key()
    if not api_key:
        result = _error_result("REVIEWER_API_KEY environment variable not set")
        session.audit_log("request_code_review", {}, "error", result)
        return result

    # 3. Stage files
    stage_result = await _stage_files(paths, stage_all=stage_all)
    if stage_result["status"] != "staged":
        result = _error_result(
            stage_result.get("reason", "No files to stage"),
            staged=stage_result.get("staged", []),
            warnings=stage_result.get("warnings", []),
        )
        session.audit_log("request_code_review", {}, "error", result)
        return result

    staged_files = stage_result["staged"]
    warnings = stage_result.get("warnings", [])

    # 4. Get staged diff
    diff = await _git_diff_staged()
    if not diff.strip():
        result = _error_result(
            "No staged changes to review",
            staged=staged_files,
            warnings=warnings,
        )
        session.audit_log("request_code_review", {}, "error", result)
        return result

    # 5. Load review assets (constitution + prompt)
    assets = _load_review_assets()
    if isinstance(assets, dict):
        result = {**assets, "staged": staged_files, "warnings": warnings}
        session.audit_log("request_code_review", {}, "error", result)
        return result
    constitution, prompt = assets

    # 6. Check for unchanged diff (short-circuit)
    current_diff_hash = await _git_diff_staged_hash()
    if (
        state.last_review_diff_hash
        and state.last_review_diff_hash == current_diff_hash
        and state.last_review_issues is not None
    ):
        result = _build_unchanged_result(state, current_attempts, staged_files, warnings)
        session.audit_log("request_code_review", {}, "unchanged", result)
        return result

    # 7. Execute review
    attempt = session.increment_review_attempts(task_id)
    previous_issues = state.last_review_issues if attempt >= 2 else None

    changed_files = await _get_changed_files()
    files_content = _read_file_contents(changed_files)

    review_result = await _perform_review(
        diff=diff,
        files_content=files_content,
        constitution=constitution,
        prompt=prompt,
        model=config.review.model,
        api_key=api_key,
        previous_issues=previous_issues,
    )

    status = review_result["status"]
    issues = review_result["issues"]

    # 8. Update session state
    state = session.load_session() or session.SessionState()
    state.last_review_status = status
    state.last_review_diff_hash = await _git_diff_staged_hash()
    state.last_review_issues = issues
    session.save_session(state)

    # 9. Build and return result
    result = _build_review_result(
        status=status,
        issues=issues,
        attempt=attempt,
        staged_files=staged_files,
        warnings=warnings,
        parse_error=review_result.get("parse_error"),
    )

    session.audit_log("request_code_review", {}, status.lower(), result)
    return result
